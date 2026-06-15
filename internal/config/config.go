package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/caioricciuti/ch-ui/internal/license"
	"gopkg.in/yaml.v3"
)

type Config struct {
	// Server
	Port    int
	DevMode bool
	AppURL  string

	// Database
	DatabasePath string

	// Security
	AppSecretKey   string
	SessionMaxAge  int // seconds, default 7 days
	AllowedOrigins []string

	// TLS — when both are set the server terminates TLS itself; otherwise it
	// serves plaintext HTTP and expects a reverse proxy to terminate TLS.
	TLSCertFile string
	TLSKeyFile  string

	// Audit forwarding (SIEM). Any combination may be enabled; all are optional.
	AuditWebhookURL    string // POST each audit event as JSON to this URL
	AuditLogFile       string // append audit events as JSON lines to this file
	AuditForwardStdout bool   // emit audit events as structured stdout logs

	// Tunnel
	TunnelURL string

	// Embedded agent
	ClickHouseURL  string // default http://localhost:8123
	ConnectionName string // default Local ClickHouse

	// License
	LicenseJSON string // Stored signed license JSON (loaded from DB at startup)

	// OIDC SSO. When configured, users can log in via an external IdP
	// (Okta/Entra/Google/Keycloak). OIDC authenticates the person; queries run
	// against ClickHouse using the per-connection service account credentials
	// stored on the target connection. Password login keeps working alongside.
	OIDCIssuerURL      string
	OIDCClientID       string
	OIDCClientSecret   string
	OIDCRedirectURL    string   // e.g. https://ch-ui.example.com/api/auth/oidc/callback
	OIDCConnectionID   string   // connection SSO sessions use (default: the embedded connection)
	OIDCAllowedDomains []string // if set, only these email domains may log in
	OIDCGroupsClaim    string   // ID-token claim holding group memberships (default "groups")
	OIDCAdminGroups    []string // group values mapped to the admin role
	OIDCAnalystGroups  []string // group values mapped to the analyst role
}

// serverConfigFile is the YAML structure for the server config file.
type serverConfigFile struct {
	Port               int      `yaml:"port"`
	AppURL             string   `yaml:"app_url"`
	DatabasePath       string   `yaml:"database_path"`
	ClickHouseURL      string   `yaml:"clickhouse_url"`
	ConnectionName     string   `yaml:"connection_name"`
	AppSecretKey       string   `yaml:"app_secret_key"`
	AllowedOrigins     []string `yaml:"allowed_origins"`
	TunnelURL          string   `yaml:"tunnel_url"`
	TLSCertFile        string   `yaml:"tls_cert_file"`
	TLSKeyFile         string   `yaml:"tls_key_file"`
	AuditWebhookURL    string   `yaml:"audit_webhook_url"`
	AuditLogFile       string   `yaml:"audit_log_file"`
	AuditForwardStdout bool     `yaml:"audit_forward_stdout"`
	OIDCIssuerURL      string   `yaml:"oidc_issuer_url"`
	OIDCClientID       string   `yaml:"oidc_client_id"`
	OIDCClientSecret   string   `yaml:"oidc_client_secret"`
	OIDCRedirectURL    string   `yaml:"oidc_redirect_url"`
	OIDCConnectionID   string   `yaml:"oidc_connection_id"`
	OIDCAllowedDomains []string `yaml:"oidc_allowed_domains"`
	OIDCGroupsClaim    string   `yaml:"oidc_groups_claim"`
	OIDCAdminGroups    []string `yaml:"oidc_admin_groups"`
	OIDCAnalystGroups  []string `yaml:"oidc_analyst_groups"`
}

// DefaultServerConfigPath returns the platform-specific default config path.
func DefaultServerConfigPath() string {
	switch runtime.GOOS {
	case "darwin":
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "ch-ui", "server.yaml")
	default:
		return "/etc/ch-ui/server.yaml"
	}
}

// Load creates a Config by merging: config file -> env vars -> defaults.
// Priority: env vars > config file > defaults.
func Load(configPath string) *Config {
	cfg := &Config{
		Port:           3488,
		DatabasePath:   "./data/ch-ui.db",
		AppSecretKey:   DefaultAppSecretKey,
		SessionMaxAge:  7 * 24 * 60 * 60,
		ClickHouseURL:  "http://localhost:8123",
		ConnectionName: "Local ClickHouse",
	}

	// 1. Load from config file (overrides defaults)
	if configPath != "" {
		if err := loadServerConfigFile(configPath, cfg); err != nil {
			if !os.IsNotExist(err) {
				slog.Warn("Failed to load config file", "path", configPath, "error", err)
			} else {
				slog.Warn("Config file not found", "path", configPath)
			}
		} else {
			slog.Info("Loaded config file", "path", configPath)
		}
	} else {
		// Try default path, silently ignore if not found
		defaultPath := DefaultServerConfigPath()
		if err := loadServerConfigFile(defaultPath, cfg); err == nil {
			slog.Info("Loaded config file", "path", defaultPath)
		}
	}

	// 2. Override with environment variables (highest priority)
	if v := os.Getenv("PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			cfg.Port = p
		}
	}
	if v := os.Getenv("APP_URL"); v != "" {
		cfg.AppURL = trimQuotes(v)
	}
	if v := os.Getenv("DATABASE_PATH"); v != "" {
		cfg.DatabasePath = v
	}
	if v := os.Getenv("CLICKHOUSE_URL"); v != "" {
		cfg.ClickHouseURL = v
	}
	if v := os.Getenv("CONNECTION_NAME"); v != "" {
		cfg.ConnectionName = trimQuotes(v)
	}
	// Backward-compatible typo alias
	if v := os.Getenv("CONNECITION_NAME"); v != "" {
		cfg.ConnectionName = trimQuotes(v)
	}
	if v := os.Getenv("APP_SECRET_KEY"); v != "" {
		cfg.AppSecretKey = trimQuotes(v)
	}
	if v := os.Getenv("ALLOWED_ORIGINS"); v != "" {
		cfg.AllowedOrigins = nil
		for _, o := range strings.Split(v, ",") {
			if trimmed := strings.TrimSpace(o); trimmed != "" {
				cfg.AllowedOrigins = append(cfg.AllowedOrigins, trimmed)
			}
		}
	}
	if v := os.Getenv("TUNNEL_URL"); v != "" {
		cfg.TunnelURL = v
	}
	if v := os.Getenv("TLS_CERT_FILE"); v != "" {
		cfg.TLSCertFile = trimQuotes(v)
	}
	if v := os.Getenv("TLS_KEY_FILE"); v != "" {
		cfg.TLSKeyFile = trimQuotes(v)
	}
	if v := os.Getenv("AUDIT_WEBHOOK_URL"); v != "" {
		cfg.AuditWebhookURL = trimQuotes(v)
	}
	if v := os.Getenv("AUDIT_LOG_FILE"); v != "" {
		cfg.AuditLogFile = trimQuotes(v)
	}
	if v := os.Getenv("AUDIT_FORWARD_STDOUT"); v != "" {
		cfg.AuditForwardStdout = v == "1" || strings.EqualFold(v, "true")
	}
	if v := os.Getenv("OIDC_ISSUER_URL"); v != "" {
		cfg.OIDCIssuerURL = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_CLIENT_ID"); v != "" {
		cfg.OIDCClientID = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_CLIENT_SECRET"); v != "" {
		cfg.OIDCClientSecret = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_REDIRECT_URL"); v != "" {
		cfg.OIDCRedirectURL = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_CONNECTION_ID"); v != "" {
		cfg.OIDCConnectionID = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_GROUPS_CLAIM"); v != "" {
		cfg.OIDCGroupsClaim = trimQuotes(v)
	}
	if v := os.Getenv("OIDC_ALLOWED_DOMAINS"); v != "" {
		cfg.OIDCAllowedDomains = splitList(v)
	}
	if v := os.Getenv("OIDC_ADMIN_GROUPS"); v != "" {
		cfg.OIDCAdminGroups = splitList(v)
	}
	if v := os.Getenv("OIDC_ANALYST_GROUPS"); v != "" {
		cfg.OIDCAnalystGroups = splitList(v)
	}

	// Derive defaults for computed fields
	if cfg.AppURL == "" {
		cfg.AppURL = "http://localhost:" + strconv.Itoa(cfg.Port)
	}
	if cfg.TunnelURL == "" {
		cfg.TunnelURL = "ws://127.0.0.1:" + strconv.Itoa(cfg.Port) + "/connect"
	}

	cfg.DevMode = os.Getenv("NODE_ENV") != "production"

	return cfg
}

func loadServerConfigFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var fc serverConfigFile
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return err
	}

	if fc.Port != 0 {
		cfg.Port = fc.Port
	}
	if fc.AppURL != "" {
		cfg.AppURL = fc.AppURL
	}
	if fc.DatabasePath != "" {
		cfg.DatabasePath = fc.DatabasePath
	}
	if fc.ClickHouseURL != "" {
		cfg.ClickHouseURL = fc.ClickHouseURL
	}
	if fc.ConnectionName != "" {
		cfg.ConnectionName = fc.ConnectionName
	}
	if fc.AppSecretKey != "" {
		cfg.AppSecretKey = fc.AppSecretKey
	}
	if len(fc.AllowedOrigins) > 0 {
		cfg.AllowedOrigins = fc.AllowedOrigins
	}
	if fc.TunnelURL != "" {
		cfg.TunnelURL = fc.TunnelURL
	}
	if fc.TLSCertFile != "" {
		cfg.TLSCertFile = fc.TLSCertFile
	}
	if fc.TLSKeyFile != "" {
		cfg.TLSKeyFile = fc.TLSKeyFile
	}
	if fc.AuditWebhookURL != "" {
		cfg.AuditWebhookURL = fc.AuditWebhookURL
	}
	if fc.AuditLogFile != "" {
		cfg.AuditLogFile = fc.AuditLogFile
	}
	if fc.AuditForwardStdout {
		cfg.AuditForwardStdout = true
	}
	if fc.OIDCIssuerURL != "" {
		cfg.OIDCIssuerURL = fc.OIDCIssuerURL
	}
	if fc.OIDCClientID != "" {
		cfg.OIDCClientID = fc.OIDCClientID
	}
	if fc.OIDCClientSecret != "" {
		cfg.OIDCClientSecret = fc.OIDCClientSecret
	}
	if fc.OIDCRedirectURL != "" {
		cfg.OIDCRedirectURL = fc.OIDCRedirectURL
	}
	if fc.OIDCConnectionID != "" {
		cfg.OIDCConnectionID = fc.OIDCConnectionID
	}
	if fc.OIDCGroupsClaim != "" {
		cfg.OIDCGroupsClaim = fc.OIDCGroupsClaim
	}
	if len(fc.OIDCAllowedDomains) > 0 {
		cfg.OIDCAllowedDomains = fc.OIDCAllowedDomains
	}
	if len(fc.OIDCAdminGroups) > 0 {
		cfg.OIDCAdminGroups = fc.OIDCAdminGroups
	}
	if len(fc.OIDCAnalystGroups) > 0 {
		cfg.OIDCAnalystGroups = fc.OIDCAnalystGroups
	}

	return nil
}

// OIDCEnabled reports whether OIDC SSO is fully configured.
func (c *Config) OIDCEnabled() bool {
	return strings.TrimSpace(c.OIDCIssuerURL) != "" &&
		strings.TrimSpace(c.OIDCClientID) != "" &&
		strings.TrimSpace(c.OIDCClientSecret) != "" &&
		strings.TrimSpace(c.OIDCRedirectURL) != ""
}

// TLSEnabled reports whether native TLS termination is configured.
func (c *Config) TLSEnabled() bool {
	return strings.TrimSpace(c.TLSCertFile) != "" && strings.TrimSpace(c.TLSKeyFile) != ""
}

// GenerateServerTemplate returns a YAML config template for the server.
func GenerateServerTemplate() string {
	return `# CH-UI Server Configuration
#
# Place this file at:
#   macOS: ~/.config/ch-ui/server.yaml
#   Linux: /etc/ch-ui/server.yaml
#
# All settings can also be set via environment variables.
# Priority: env vars > config file > defaults

# HTTP port (default: 3488)
port: 3488

# Public URL of the server
# app_url: https://ch-ui.yourcompany.com

# SQLite database path (default: ./data/ch-ui.db)
# database_path: /var/lib/ch-ui/ch-ui.db

# ClickHouse HTTP endpoint (default: http://localhost:8123)
# clickhouse_url: http://localhost:8123

# Embedded connection display name (default: Local ClickHouse)
# connection_name: Local ClickHouse

# Secret key for session encryption (CHANGE THIS in production)
# app_secret_key: your-random-secret-here

# Allowed CORS origins
# allowed_origins:
#   - https://ch-ui.yourcompany.com

# Native TLS termination. Set both to serve HTTPS directly (PEM files).
# If unset, CH-UI serves plaintext HTTP and expects a reverse proxy to
# terminate TLS. Do not expose plaintext HTTP directly on a network.
# tls_cert_file: /etc/ch-ui/tls/server.crt
# tls_key_file: /etc/ch-ui/tls/server.key

# Audit forwarding (SIEM). The authoritative audit copy is always kept in the
# database; these forward a best-effort stream to your tooling.
# audit_forward_stdout: true                       # structured stdout logs
# audit_log_file: /var/log/ch-ui/audit.jsonl       # append JSON lines
# audit_webhook_url: https://siem.example.com/hook # POST each event as JSON

# OIDC SSO (Okta/Entra/Google/Keycloak). When set, users can sign in via your
# IdP. Queries run via the per-connection ClickHouse service account (set it in
# the admin UI or PUT /api/connections/{id}/sso-account). Password login still works.
# oidc_issuer_url: https://accounts.google.com
# oidc_client_id: your-client-id
# oidc_client_secret: your-client-secret
# oidc_redirect_url: https://ch-ui.yourcompany.com/api/auth/oidc/callback
# oidc_connection_id: ""                  # default: the embedded connection
# oidc_allowed_domains: [yourcompany.com] # restrict by email domain (optional)
# oidc_groups_claim: groups               # ID-token claim with group memberships
# oidc_admin_groups: [ch-ui-admins]       # IdP groups mapped to the admin role
# oidc_analyst_groups: [data-analysts]    # IdP groups mapped to the analyst role
`
}

func (c *Config) IsProduction() bool {
	return !c.DevMode
}

// ProAccess describes the installation's current entitlement to Pro features.
type ProAccess int

const (
	// ProNone means no valid Pro license and no grace window — Pro features 402.
	ProNone ProAccess = iota
	// ProActive means a valid, unexpired Pro license — full Pro access.
	ProActive
	// ProGrace means the Pro license expired but is within the read-only grace
	// window — reads are allowed, writes are blocked.
	ProGrace
)

// ProAccess validates the stored license once and returns the current Pro
// entitlement state.
func (c *Config) ProAccess() ProAccess {
	info := license.ValidateLicense(c.LicenseJSON)
	if !strings.EqualFold(strings.TrimSpace(info.Edition), "pro") {
		return ProNone
	}
	switch {
	case info.Valid:
		return ProActive
	case info.InGrace:
		return ProGrace
	default:
		return ProNone
	}
}

// IsPro reports whether the installation has a fully active Pro license.
func (c *Config) IsPro() bool {
	return c.ProAccess() == ProActive
}

// splitList parses a comma-separated env value into a trimmed, non-empty slice.
func splitList(v string) []string {
	var out []string
	for _, part := range strings.Split(v, ",") {
		if t := strings.TrimSpace(part); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func trimQuotes(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}
