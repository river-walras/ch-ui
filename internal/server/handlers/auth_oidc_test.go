package handlers

import (
	"testing"

	"github.com/caioricciuti/ch-ui/internal/config"
)

func TestOIDCRoleMapping(t *testing.T) {
	h := &AuthHandler{Config: &config.Config{
		OIDCAdminGroups:   []string{"ch-ui-admins", "platform"},
		OIDCAnalystGroups: []string{"data-analysts"},
	}}

	cases := []struct {
		name   string
		groups []string
		want   string
	}{
		{"admin wins", []string{"data-analysts", "platform"}, "admin"},
		{"analyst", []string{"data-analysts"}, "analyst"},
		{"case-insensitive admin", []string{"CH-UI-Admins"}, "admin"},
		{"unknown group -> viewer", []string{"random"}, "viewer"},
		{"no groups -> viewer", nil, "viewer"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := h.oidcRole(tc.groups); got != tc.want {
				t.Fatalf("oidcRole(%v) = %q, want %q", tc.groups, got, tc.want)
			}
		})
	}
}

func TestOIDCDomainAllowed(t *testing.T) {
	t.Run("no restriction allows all", func(t *testing.T) {
		h := &AuthHandler{Config: &config.Config{}}
		if !h.oidcDomainAllowed("anyone@example.com") {
			t.Fatal("expected all domains allowed when none configured")
		}
	})

	h := &AuthHandler{Config: &config.Config{
		OIDCAllowedDomains: []string{"caioricciuti.com", "acme.io"},
	}}
	cases := map[string]bool{
		"caio@caioricciuti.com": true,
		"x@ACME.IO":             true, // case-insensitive
		"evil@attacker.com":     false,
		"no-at-sign":            false,
	}
	for email, want := range cases {
		if got := h.oidcDomainAllowed(email); got != want {
			t.Errorf("oidcDomainAllowed(%q) = %v, want %v", email, got, want)
		}
	}
}
