package config

import (
	"bytes"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Port              string
	HTTPPort          string
	HTTPSPort         string
	TZ                string
	JWTSecret         string
	DatabasePath      string
	PlaywrightPath    string
	MaxFpsLimit       int
	OIDCProvider      string
	OIDCClientID      string
	OIDCClientSecret  string
	OIDCRedirectURL   string
	OIDCAllowedEmails []string
	OIDCScopes        []string
	TLSDomain         string
	TLSEmail          string
	TLSDataDir        string
}

func Load() *Config {
	jwtSecret := getEnvOrFile("JWT_SECRET", "")
	if jwtSecret == "" {
		// CRITICAL SECURITY REQUIREMENT: Fail fast if no secret
		panic("CRITICAL ERROR: JWT_SECRET or JWT_SECRET_FILE environment variable is not set. Refusing to start.")
	}

	return &Config{
		Port:              getEnv("PORT", "8080"), // Legacy fallback
		HTTPPort:          getEnv("HTTP_PORT", "8080"),
		HTTPSPort:         getEnv("HTTPS_PORT", "8443"),
		TZ:                getEnv("TZ", "UTC"),
		JWTSecret:         jwtSecret,
		DatabasePath:      getEnv("DATABASE_PATH", "./data/app.db"),
		PlaywrightPath:    getEnv("PLAYWRIGHT_PATH", ""),
		MaxFpsLimit:       getEnvInt("APP_MAX_FPS_LIMIT", 60),
		OIDCProvider:      getEnv("OIDC_PROVIDER", ""),
		OIDCClientID:      getEnv("OIDC_CLIENT_ID", ""),
		OIDCClientSecret:  getEnvOrFile("OIDC_CLIENT_SECRET", ""),
		OIDCRedirectURL:   getEnv("OIDC_REDIRECT_URL", ""),
		OIDCAllowedEmails: normalizeEmailList(getEnv("OIDC_ALLOWED_EMAILS", "")),
		OIDCScopes:        normalizeScopes(getEnv("OIDC_SCOPES", "openid profile email")),
		TLSDomain:         getEnv("TLS_DOMAIN", ""),
		TLSEmail:          getEnv("TLS_EMAIL", ""),
		TLSDataDir:        getEnv("TLS_DATA_DIR", "/app/data/certs"),
	}
}

// Validate checks critical configuration and permissions
func (c *Config) Validate() error {
	// Check TLS Data Dir writability if TLS is enabled or just in general for data persistence
	// We check it if we are going to use it, or generally if it's the data dir.
	// Implementation: Check if we can write to c.TLSDataDir
	if c.TLSDomain != "" {
		// Ensure directory exists or can be created
		if err := os.MkdirAll(c.TLSDataDir, 0755); err != nil {
			return fmt.Errorf("failed to create TLS data directory %s: %w", c.TLSDataDir, err)
		}
		// Test write
		testFile := c.TLSDataDir + "/.write_test"
		if err := os.WriteFile(testFile, []byte("test"), 0600); err != nil {
			return fmt.Errorf("TLS data directory %s is not writable: %w. UID: %d", c.TLSDataDir, err, os.Getuid())
		}
		os.Remove(testFile)
	}
	return nil
}

func getEnv(key, defaultVal string) string {
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

// getEnvOrFile tries to read from Key_FILE first, then Key environment variable.
func getEnvOrFile(key, defaultVal string) string {
	// 1. Try _FILE variant first (Docker Secrets preferred)
	fileKey := key + "_FILE"
	if filePath, ok := os.LookupEnv(fileKey); ok && filePath != "" {
		content, err := os.ReadFile(filePath)
		if err == nil {
			// Trim whitespace/newlines from file content
			return string(bytes.TrimSpace(content))
		}
		// If file specified but fails to read, we should probably warn or fail,
		// but for now we fallback or return empty to let validation fail later.
		// Detailed logging would be good here but we are in config package.
	}

	// 2. Fallback to direct env var
	if v, ok := os.LookupEnv(key); ok {
		return v
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultVal
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return defaultVal
	}
	return i
}

func normalizeEmailList(input string) []string {
	if input == "" {
		return nil
	}
	parts := strings.Split(input, ",")
	var result []string
	for _, p := range parts {
		email := strings.ToLower(strings.TrimSpace(p))
		if email != "" {
			// Basic sanitization/validation could go here
			result = append(result, email)
		}
	}
	return result
}

func normalizeScopes(input string) []string {
	parts := strings.Fields(input) // Handles spaces better than Split
	if len(parts) == 0 {
		return []string{"openid", "profile", "email"}
	}
	return parts
}
