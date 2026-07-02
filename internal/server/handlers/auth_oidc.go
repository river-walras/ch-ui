// SPDX-License-Identifier: BUSL-1.1
// Copyright (C) 2024-2026 Caio Ricciuti.
// Part of CH-UI Pro. Licensed under the Business Source License 1.1 (see
// LICENSE.BSL), NOT the Apache-2.0 LICENSE that governs the rest of the repo.

package handlers

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/caioricciuti/ch-ui/internal/database"
)

const (
	oidcStateCookie = "chui_oidc_state"
	oidcNonceCookie = "chui_oidc_nonce"
	oidcFlowMaxAge  = 10 * time.Minute
)

// OIDCLogin starts the SSO flow: generate state + nonce, stash them in
// short-lived cookies, and redirect the browser to the identity provider.
//
// GET /api/auth/oidc/login
func (h *AuthHandler) OIDCLogin(w http.ResponseWriter, r *http.Request) {
	state, err := randomToken()
	if err != nil {
		h.oidcFail(w, r, "could not start SSO", err)
		return
	}
	nonce, err := randomToken()
	if err != nil {
		h.oidcFail(w, r, "could not start SSO", err)
		return
	}

	secure := shouldUseSecureCookie(r, h.Config)
	h.setFlowCookie(w, oidcStateCookie, state, secure)
	h.setFlowCookie(w, oidcNonceCookie, nonce, secure)

	http.Redirect(w, r, h.OIDC.AuthCodeURL(state, nonce), http.StatusFound)
}

// OIDCCallback completes the SSO flow: validate state, exchange the code, verify
// the ID token, map the person to a role and to the connection's service
// account, and create a CH-UI session.
//
// GET /api/auth/oidc/callback
func (h *AuthHandler) OIDCCallback(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if errParam := q.Get("error"); errParam != "" {
		h.oidcFail(w, r, "identity provider returned an error", fmt.Errorf("%s: %s", errParam, q.Get("error_description")))
		return
	}

	// CSRF: the state query param must match the state cookie.
	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil || stateCookie.Value == "" || stateCookie.Value != q.Get("state") {
		h.oidcFail(w, r, "invalid SSO state", fmt.Errorf("state mismatch"))
		return
	}
	nonceCookie, err := r.Cookie(oidcNonceCookie)
	if err != nil || nonceCookie.Value == "" {
		h.oidcFail(w, r, "invalid SSO session", fmt.Errorf("missing nonce"))
		return
	}
	// One-shot: clear the flow cookies regardless of outcome.
	secure := shouldUseSecureCookie(r, h.Config)
	h.clearFlowCookie(w, oidcStateCookie, secure)
	h.clearFlowCookie(w, oidcNonceCookie, secure)

	claims, err := h.OIDC.Exchange(r.Context(), q.Get("code"), nonceCookie.Value)
	if err != nil {
		h.oidcFail(w, r, "SSO verification failed", err)
		return
	}

	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if email == "" {
		h.oidcFail(w, r, "your identity provider did not return an email", fmt.Errorf("empty email claim"))
		return
	}
	if !h.oidcDomainAllowed(email) {
		h.oidcFail(w, r, "your email domain is not allowed", fmt.Errorf("domain not allowed: %s", email))
		return
	}

	// Resolve the target connection and its ClickHouse service account.
	connID, err := h.oidcConnectionID()
	if err != nil {
		h.oidcFail(w, r, "no connection configured for SSO", err)
		return
	}
	saUser, saEncryptedPw, err := h.DB.GetConnectionSSOAccount(connID)
	if err != nil || saUser == "" || saEncryptedPw == "" {
		h.oidcFail(w, r, "SSO is not finished being set up (no ClickHouse service account on the connection)", fmt.Errorf("missing sso service account for %s", connID))
		return
	}

	role := h.oidcRole(claims.Groups)

	token := uuid.NewString()
	expiresAt := time.Now().UTC().Add(SessionDuration).Format(time.RFC3339)
	if _, err := h.DB.CreateSession(database.CreateSessionParams{
		ConnectionID:      connID,
		ClickhouseUser:    saUser,
		EncryptedPassword: saEncryptedPw,
		Token:             token,
		ExpiresAt:         expiresAt,
		UserRole:          role,
		AuthSubject:       email,
	}); err != nil {
		h.oidcFail(w, r, "could not create session", err)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(SessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})

	h.DB.CreateAuditLog(database.AuditLogParams{
		Action:       "user.login",
		Username:     strPtr(email),
		ConnectionID: strPtr(connID),
		Details:      strPtr(fmt.Sprintf("SSO login via OIDC (role: %s, ch_account: %s)", role, saUser)),
		IPAddress:    strPtr(getClientIP(r)),
	})
	slog.Info("SSO login", "user", email, "role", role, "connection", connID)

	http.Redirect(w, r, "/", http.StatusFound)
}

// oidcRole maps the person's IdP groups to a CH-UI role.
func (h *AuthHandler) oidcRole(groups []string) string {
	inAny := func(set []string) bool {
		for _, g := range groups {
			for _, want := range set {
				if strings.EqualFold(strings.TrimSpace(g), strings.TrimSpace(want)) {
					return true
				}
			}
		}
		return false
	}
	if inAny(h.Config.OIDCAdminGroups) {
		return "admin"
	}
	if inAny(h.Config.OIDCAnalystGroups) {
		return "analyst"
	}
	return "viewer"
}

func (h *AuthHandler) oidcDomainAllowed(email string) bool {
	if len(h.Config.OIDCAllowedDomains) == 0 {
		return true
	}
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return false
	}
	domain := email[at+1:]
	for _, d := range h.Config.OIDCAllowedDomains {
		if strings.EqualFold(domain, strings.TrimSpace(d)) {
			return true
		}
	}
	return false
}

// oidcConnectionID returns the connection SSO sessions should use: the
// configured one, or the embedded connection, or the first connection.
func (h *AuthHandler) oidcConnectionID() (string, error) {
	if id := strings.TrimSpace(h.Config.OIDCConnectionID); id != "" {
		return id, nil
	}
	conns, err := h.DB.GetConnections()
	if err != nil {
		return "", err
	}
	for _, c := range conns {
		if c.IsEmbedded {
			return c.ID, nil
		}
	}
	if len(conns) > 0 {
		return conns[0].ID, nil
	}
	return "", fmt.Errorf("no connections exist")
}

func (h *AuthHandler) setFlowCookie(w http.ResponseWriter, name, value string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    value,
		Path:     "/api/auth/oidc",
		MaxAge:   int(oidcFlowMaxAge.Seconds()),
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (h *AuthHandler) clearFlowCookie(w http.ResponseWriter, name string, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    "",
		Path:     "/api/auth/oidc",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
	})
}

// oidcFail logs the underlying error and redirects to the login page with a
// short, non-sensitive reason.
func (h *AuthHandler) oidcFail(w http.ResponseWriter, r *http.Request, userMsg string, err error) {
	slog.Warn("OIDC SSO failed", "reason", userMsg, "error", err)
	http.Redirect(w, r, "/login?sso_error="+url.QueryEscape(userMsg), http.StatusFound)
}

func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
