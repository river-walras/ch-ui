// SPDX-License-Identifier: BUSL-1.1
// Copyright (C) 2024-2026 Caio Ricciuti.
// Part of CH-UI Pro. Licensed under the Business Source License 1.1 (see
// LICENSE.BSL), NOT the Apache-2.0 LICENSE that governs the rest of the repo.

// Package oidc wraps an OpenID Connect identity provider for CH-UI SSO. It
// authenticates the person; ClickHouse access is handled separately by the
// per-connection service account.
package oidc

import (
	"context"
	"fmt"
	"strings"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/caioricciuti/ch-ui/internal/config"
)

// Provider holds an initialized OIDC verifier and OAuth2 config.
type Provider struct {
	verifier    *gooidc.IDTokenVerifier
	oauth2      oauth2.Config
	groupsClaim string
}

// Claims is the subset of ID-token claims CH-UI uses.
type Claims struct {
	Subject       string
	Email         string
	EmailVerified bool
	Name          string
	Groups        []string
}

// New initializes an OIDC provider from config by discovering the issuer.
func New(ctx context.Context, cfg *config.Config) (*Provider, error) {
	provider, err := gooidc.NewProvider(ctx, strings.TrimSpace(cfg.OIDCIssuerURL))
	if err != nil {
		return nil, fmt.Errorf("oidc discovery for %q: %w", cfg.OIDCIssuerURL, err)
	}

	groupsClaim := strings.TrimSpace(cfg.OIDCGroupsClaim)
	if groupsClaim == "" {
		groupsClaim = "groups"
	}

	scopes := []string{gooidc.ScopeOpenID, "email", "profile"}
	if len(cfg.OIDCAdminGroups) > 0 || len(cfg.OIDCAnalystGroups) > 0 {
		scopes = append(scopes, groupsClaim)
	}

	return &Provider{
		verifier: provider.Verifier(&gooidc.Config{ClientID: cfg.OIDCClientID}),
		oauth2: oauth2.Config{
			ClientID:     cfg.OIDCClientID,
			ClientSecret: cfg.OIDCClientSecret,
			RedirectURL:  cfg.OIDCRedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       scopes,
		},
		groupsClaim: groupsClaim,
	}, nil
}

// AuthCodeURL builds the IdP authorization URL with state and nonce.
func (p *Provider) AuthCodeURL(state, nonce string) string {
	return p.oauth2.AuthCodeURL(state, gooidc.Nonce(nonce))
}

// Exchange swaps the authorization code for tokens and verifies the ID token,
// checking that its nonce matches the one issued at login.
func (p *Provider) Exchange(ctx context.Context, code, expectedNonce string) (*Claims, error) {
	tok, err := p.oauth2.Exchange(ctx, code)
	if err != nil {
		return nil, fmt.Errorf("token exchange: %w", err)
	}
	rawID, ok := tok.Extra("id_token").(string)
	if !ok || rawID == "" {
		return nil, fmt.Errorf("no id_token in token response")
	}
	idToken, err := p.verifier.Verify(ctx, rawID)
	if err != nil {
		return nil, fmt.Errorf("verify id_token: %w", err)
	}
	if idToken.Nonce != expectedNonce {
		return nil, fmt.Errorf("id_token nonce mismatch")
	}

	var raw map[string]interface{}
	if err := idToken.Claims(&raw); err != nil {
		return nil, fmt.Errorf("parse claims: %w", err)
	}

	c := &Claims{
		Subject: idToken.Subject,
		Email:   asString(raw["email"]),
		Name:    asString(raw["name"]),
	}
	if v, ok := raw["email_verified"].(bool); ok {
		c.EmailVerified = v
	}
	c.Groups = asStringSlice(raw[p.groupsClaim])
	return c, nil
}

func asString(v interface{}) string {
	s, _ := v.(string)
	return s
}

func asStringSlice(v interface{}) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []interface{}:
		out := make([]string, 0, len(t))
		for _, e := range t {
			if s, ok := e.(string); ok {
				out = append(out, s)
			}
		}
		return out
	case string:
		if t == "" {
			return nil
		}
		return []string{t}
	}
	return nil
}
