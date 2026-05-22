package config

import (
	"fmt"
	"net/url"
	"os"
	"strings"
)

type Config struct {
	Host        string
	TokenID     string
	TokenSecret string
	Username    string
	Password    string
	VerifyTLS   bool
	AllowExec   bool
}

func (c *Config) UseTokenAuth() bool {
	return c.TokenID != ""
}

func Load() (*Config, error) {
	host := os.Getenv("PROXMOX_HOST")
	if host == "" {
		return nil, fmt.Errorf("PROXMOX_HOST is required")
	}
	host = strings.TrimRight(host, "/")

	u, err := url.ParseRequestURI(host)
	if err != nil {
		return nil, fmt.Errorf("PROXMOX_HOST is not a valid URL: %v", err)
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return nil, fmt.Errorf("PROXMOX_HOST must use https:// (or http:// for testing)")
	}

	tokenID, err := loadSecret("PROXMOX_TOKEN_ID")
	if err != nil {
		return nil, err
	}
	tokenSecret, err := loadSecret("PROXMOX_TOKEN_SECRET")
	if err != nil {
		return nil, err
	}

	username, err := loadSecret("PROXMOX_USER")
	if err != nil {
		return nil, err
	}
	password, err := loadSecret("PROXMOX_PASSWORD")
	if err != nil {
		return nil, err
	}

	hasToken := tokenID != "" || tokenSecret != ""
	hasPassword := username != "" || password != ""

	if !hasToken && !hasPassword {
		return nil, fmt.Errorf("authentication required: set PROXMOX_TOKEN_ID + PROXMOX_TOKEN_SECRET, or PROXMOX_USER + PROXMOX_PASSWORD")
	}
	if hasToken && hasPassword {
		return nil, fmt.Errorf("set either token auth (PROXMOX_TOKEN_ID) or password auth (PROXMOX_USER), not both")
	}
	if hasToken {
		if tokenID == "" {
			return nil, fmt.Errorf("PROXMOX_TOKEN_ID is required (format: user@realm!token-name)")
		}
		if tokenSecret == "" {
			return nil, fmt.Errorf("PROXMOX_TOKEN_SECRET is required")
		}
	}
	if hasPassword {
		if username == "" {
			return nil, fmt.Errorf("PROXMOX_USER is required (format: user@realm)")
		}
		if password == "" {
			return nil, fmt.Errorf("PROXMOX_PASSWORD is required")
		}
	}

	verifyTLS, err := parseBoolEnv("PROXMOX_VERIFY_TLS", true)
	if err != nil {
		return nil, err
	}

	allowExec, err := parseBoolEnv("PROXMOX_ALLOW_EXEC", false)
	if err != nil {
		return nil, err
	}

	return &Config{
		Host:        host,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Username:    username,
		Password:    password,
		VerifyTLS:   verifyTLS,
		AllowExec:   allowExec,
	}, nil
}

// loadSecret reads a value from env var NAME, or if NAME_FILE is set,
// reads the value from the file at that path. The _FILE variant takes
// precedence, enabling integration with secret managers, Docker secrets,
// and Kubernetes secret volumes without exposing values in config files.
func loadSecret(name string) (string, error) {
	filePath := os.Getenv(name + "_FILE")
	if filePath != "" {
		data, err := os.ReadFile(filePath)
		if err != nil {
			return "", fmt.Errorf("%s_FILE: cannot read %q: %v", name, filePath, err)
		}
		val := strings.TrimSpace(string(data))
		if val == "" {
			return "", fmt.Errorf("%s_FILE: file %q is empty", name, filePath)
		}
		return val, nil
	}
	return os.Getenv(name), nil
}

func parseBoolEnv(name string, defaultVal bool) (bool, error) {
	v := os.Getenv(name)
	if v == "" {
		return defaultVal, nil
	}
	switch strings.ToLower(v) {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be true/false/1/0, got %q", name, v)
	}
}
