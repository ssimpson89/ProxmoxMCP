package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/ssimpson/ProxmoxMCP/config"
	"github.com/ssimpson/ProxmoxMCP/proxmox"
)

type mcpServerEntry struct {
	Command string            `json:"command"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
}

func main() {
	fmt.Println("╔══════════════════════════════════════════╗")
	fmt.Println("║     Proxmox MCP Server Setup Wizard      ║")
	fmt.Println("╚══════════════════════════════════════════╝")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)

	host := promptRequired(reader, "Proxmox host URL (e.g. https://pve.example.com:8006)")
	host = strings.TrimRight(host, "/")
	if err := validateHost(host); err != nil {
		fatal("Invalid host: %v", err)
	}

	verifyTLS := promptYesNo(reader, "Verify TLS certificates?", true)

	authMethod := promptChoice(reader, "Authentication method", []string{
		"API Token (recommended for MCP servers)",
		"Username & Password",
	})

	var tokenID, tokenSecret, username, password string
	var secretLabel, secretEnvKey, secretFileEnvKey string
	var credFileNames [2]struct{ name, value string }

	if strings.HasPrefix(authMethod, "API") {
		tokenID = promptRequired(reader, "API Token ID (e.g. user@pam!token-name)")
		if !strings.Contains(tokenID, "!") {
			fatal("Token ID must contain '!' (format: user@realm!token-name)")
		}
		tokenSecret = promptRequired(reader, "API Token Secret (UUID)")
		secretLabel = "Token Secret"
		secretEnvKey = "PROXMOX_TOKEN_SECRET"
		secretFileEnvKey = "PROXMOX_TOKEN_SECRET_FILE"
		credFileNames = [2]struct{ name, value string }{
			{"token-id", tokenID},
			{"token-secret", tokenSecret},
		}
	} else {
		username = promptRequired(reader, "Username (e.g. root@pam)")
		if !strings.Contains(username, "@") {
			fatal("Username must include realm (format: user@realm, e.g. root@pam)")
		}
		password = promptRequired(reader, "Password")
		secretLabel = "Password"
		secretEnvKey = "PROXMOX_PASSWORD"
		secretFileEnvKey = "PROXMOX_PASSWORD_FILE"
		credFileNames = [2]struct{ name, value string }{
			{"username", username},
			{"password", password},
		}
	}

	allowExec := promptYesNo(reader, "Enable qemu_exec (command execution inside VMs)?", false)

	binaryPath := findBinary()

	fmt.Println()
	fmt.Println("─── Testing Connection ───")
	fmt.Printf("  Connecting to %s ...", host)

	cfg := &config.Config{
		Host:        host,
		TokenID:     tokenID,
		TokenSecret: tokenSecret,
		Username:    username,
		Password:    password,
		VerifyTLS:   verifyTLS,
	}
	client, err := proxmox.NewClient(cfg)
	if err != nil {
		fmt.Println(" FAILED")
		fatal("Could not create client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := proxmox.ValidateConnection(ctx, client); err != nil {
		fmt.Println(" FAILED")
		fatal("Connection failed: %v\n\n  Check your host URL and credentials.", err)
	}
	fmt.Println(" OK")

	fmt.Println()
	fmt.Println("─── Configuration Summary ───")
	fmt.Printf("  Host:       %s\n", host)
	if tokenID != "" {
		fmt.Printf("  Token ID:   %s\n", tokenID)
		fmt.Printf("  Secret:     %s\n", maskSecret(tokenSecret))
	} else {
		fmt.Printf("  Username:   %s\n", username)
		fmt.Printf("  Password:   %s\n", maskSecret(password))
	}
	fmt.Printf("  Verify TLS: %v\n", verifyTLS)
	fmt.Printf("  Allow Exec: %v\n", allowExec)
	fmt.Printf("  Binary:     %s\n", binaryPath)
	fmt.Println()

	fmt.Println("─── Credential Storage ───")
	fmt.Println("  MCP clients pass credentials as environment variables.")
	fmt.Println("  Choose how to store them:")
	fmt.Println()
	mode := promptChoice(reader, "Storage method", []string{
		"Credential file (recommended, secrets in a separate file Claude cannot see)",
		"Wrapper script (secrets in a shell script, not in JSON config)",
		"Inline env vars (standard, credentials stored in config JSON)",
	})

	target := promptChoice(reader, "Generate config for", []string{"Claude Code", "Claude Desktop", "Both", "Print JSON only"})

	env := map[string]string{
		"PROXMOX_HOST": host,
	}
	if !verifyTLS {
		env["PROXMOX_VERIFY_TLS"] = "false"
	}
	if allowExec {
		env["PROXMOX_ALLOW_EXEC"] = "true"
	}

	_ = secretLabel
	var entry mcpServerEntry
	var chosenMethod string

	switch {
	case strings.HasPrefix(mode, "Credential file"):
		credDir, err := writeCredentialFiles(credFileNames[0].name, credFileNames[0].value, credFileNames[1].name, credFileNames[1].value)
		if err != nil {
			fmt.Printf("\n  Warning: failed to write credential files: %v\n", err)
			fmt.Println("  Falling back to inline env vars.")
			chosenMethod = "inline"
		} else {
			if tokenID != "" {
				env["PROXMOX_TOKEN_ID_FILE"] = filepath.Join(credDir, credFileNames[0].name)
				env[secretFileEnvKey] = filepath.Join(credDir, credFileNames[1].name)
			} else {
				env["PROXMOX_USER_FILE"] = filepath.Join(credDir, credFileNames[0].name)
				env[secretFileEnvKey] = filepath.Join(credDir, credFileNames[1].name)
			}
			entry = mcpServerEntry{
				Command: binaryPath,
				Env:     env,
			}
			chosenMethod = "file"
			fmt.Printf("\n  ✓ Credential files written to %s/ (mode 0600)\n", credDir)
			fmt.Println("    Config JSON contains file paths, not secrets.")
		}

	case strings.HasPrefix(mode, "Wrapper"):
		allEnv := map[string]string{"PROXMOX_HOST": host}
		if tokenID != "" {
			allEnv["PROXMOX_TOKEN_ID"] = tokenID
			allEnv[secretEnvKey] = tokenSecret
		} else {
			allEnv["PROXMOX_USER"] = username
			allEnv[secretEnvKey] = password
		}
		if !verifyTLS {
			allEnv["PROXMOX_VERIFY_TLS"] = "false"
		}
		if allowExec {
			allEnv["PROXMOX_ALLOW_EXEC"] = "true"
		}
		wrapperPath, err := writeWrapperScript(binaryPath, allEnv)
		if err != nil {
			fmt.Printf("\n  Warning: failed to create wrapper script: %v\n", err)
			fmt.Println("  Falling back to inline env vars.")
			chosenMethod = "inline"
		} else {
			entry = mcpServerEntry{
				Command: wrapperPath,
			}
			chosenMethod = "wrapper"
			fmt.Printf("\n  ✓ Wrapper script written to %s (mode 0700)\n", wrapperPath)
			fmt.Println("    Credentials are in the script, NOT in the MCP config JSON.")
		}
	}

	if chosenMethod == "" || chosenMethod == "inline" {
		if tokenID != "" {
			env["PROXMOX_TOKEN_ID"] = tokenID
			env[secretEnvKey] = tokenSecret
		} else {
			env["PROXMOX_USER"] = username
			env[secretEnvKey] = password
		}
		entry = mcpServerEntry{
			Command: binaryPath,
			Env:     env,
		}
		chosenMethod = "inline"
	}

	switch target {
	case "Claude Code":
		writeConfig(entry, claudeCodePath())
	case "Claude Desktop":
		writeConfig(entry, claudeDesktopPath())
	case "Both":
		writeConfig(entry, claudeCodePath())
		writeConfig(entry, claudeDesktopPath())
	case "Print JSON only":
		printJSON(entry)
	}

	fmt.Println()
	fmt.Println("─── Security Notes ───")
	switch chosenMethod {
	case "file":
		fmt.Println("  Credentials are in separate files, not the MCP config JSON.")
		fmt.Println("  Claude sees only file paths in the config, not the actual secrets.")
	case "wrapper":
		fmt.Println("  Credentials are in the wrapper script, not the MCP config JSON.")
		fmt.Println("  Claude sees only the script path in the config, not the actual secrets.")
	default:
		fmt.Println("  Credentials are stored as plaintext in the MCP config file.")
		fmt.Println("  Claude can read this file. Consider using credential file or wrapper method.")
	}
	fmt.Println("  Use a dedicated Proxmox user with minimal permissions.")
	fmt.Println("  Rotate your API token periodically.")
	fmt.Println()
}

func maskSecret(s string) string {
	if len(s) < 8 {
		return "****"
	}
	return s[:2] + strings.Repeat("*", len(s)-4) + s[len(s)-2:]
}

func promptRequired(reader *bufio.Reader, prompt string) string {
	for {
		fmt.Printf("  %s: ", prompt)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input != "" {
			return input
		}
		fmt.Println("  (required)")
	}
}

func promptYesNo(reader *bufio.Reader, prompt string, defaultVal bool) bool {
	hint := "Y/n"
	if !defaultVal {
		hint = "y/N"
	}
	fmt.Printf("  %s [%s]: ", prompt, hint)
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(strings.ToLower(input))
	if input == "" {
		return defaultVal
	}
	return input == "y" || input == "yes"
}

func promptChoice(reader *bufio.Reader, prompt string, choices []string) string {
	fmt.Printf("  %s:\n", prompt)
	for i, c := range choices {
		fmt.Printf("    %d) %s\n", i+1, c)
	}
	for {
		fmt.Printf("  Choice [1]: ")
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)
		if input == "" {
			return choices[0]
		}
		var idx int
		if _, err := fmt.Sscanf(input, "%d", &idx); err == nil && idx >= 1 && idx <= len(choices) {
			return choices[idx-1]
		}
		fmt.Println("  (invalid choice)")
	}
}

func validateHost(host string) error {
	u, err := url.ParseRequestURI(host)
	if err != nil {
		return err
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("must use https:// (or http:// for testing)")
	}
	return nil
}

func findBinary() string {
	paths := []string{
		"proxmox-mcp",
	}

	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		home, _ := os.UserHomeDir()
		gopath = filepath.Join(home, "go")
	}
	paths = append(paths, filepath.Join(gopath, "bin", "proxmox-mcp"))

	for _, p := range paths {
		if abs, err := filepath.Abs(p); err == nil {
			if _, err := os.Stat(abs); err == nil {
				return abs
			}
		}
	}

	if p, err := findInPath("proxmox-mcp"); err == nil {
		return p
	}

	return "proxmox-mcp"
}

func findInPath(name string) (string, error) {
	pathEnv := os.Getenv("PATH")
	for _, dir := range filepath.SplitList(pathEnv) {
		full := filepath.Join(dir, name)
		if runtime.GOOS == "windows" {
			full += ".exe"
		}
		if _, err := os.Stat(full); err == nil {
			return full, nil
		}
	}
	return "", fmt.Errorf("not found in PATH")
}

func writeCredentialFiles(name1, value1, name2, value2 string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	credDir := filepath.Join(home, ".config", "proxmox-mcp")
	if err := os.MkdirAll(credDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create credential dir: %v", err)
	}

	for _, f := range []struct {
		name, value string
	}{
		{name1, value1},
		{name2, value2},
	} {
		path := filepath.Join(credDir, f.name)
		if err := os.WriteFile(path, []byte(f.value+"\n"), 0o600); err != nil {
			return "", fmt.Errorf("failed to write %s: %v", path, err)
		}
		if err := os.Chmod(path, 0o600); err != nil {
			return "", fmt.Errorf("failed to set permissions on %s: %v", path, err)
		}
	}

	return credDir, nil
}

func writeWrapperScript(binaryPath string, env map[string]string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}

	scriptDir := filepath.Join(home, ".config", "proxmox-mcp")
	if err := os.MkdirAll(scriptDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create script dir: %v", err)
	}

	var scriptPath, scriptContent string

	if runtime.GOOS == "windows" {
		scriptPath = filepath.Join(scriptDir, "run.bat")
		var lines []string
		lines = append(lines, "@echo off")
		for k, v := range env {
			lines = append(lines, fmt.Sprintf("set %s=%s", k, v))
		}
		lines = append(lines, fmt.Sprintf("\"%s\" %%*", binaryPath))
		scriptContent = strings.Join(lines, "\r\n") + "\r\n"
	} else {
		scriptPath = filepath.Join(scriptDir, "run.sh")
		var lines []string
		lines = append(lines, "#!/bin/sh")
		for k, v := range env {
			lines = append(lines, fmt.Sprintf("export %s='%s'", k, shellEscape(v)))
		}
		lines = append(lines, fmt.Sprintf("exec '%s' \"$@\"", shellEscape(binaryPath)))
		scriptContent = strings.Join(lines, "\n") + "\n"
	}

	if err := os.WriteFile(scriptPath, []byte(scriptContent), 0o700); err != nil {
		return "", err
	}
	if err := os.Chmod(scriptPath, 0o700); err != nil {
		return "", err
	}

	return scriptPath, nil
}

func shellEscape(s string) string {
	return strings.ReplaceAll(s, "'", "'\\''")
}

func claudeCodePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "settings.json")
}

func claudeDesktopPath() string {
	home, _ := os.UserHomeDir()
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json")
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Claude", "claude_desktop_config.json")
	default:
		return filepath.Join(home, ".config", "claude", "claude_desktop_config.json")
	}
}

func writeConfig(entry mcpServerEntry, configPath string) {
	var config map[string]any

	data, err := os.ReadFile(configPath)
	if err == nil {
		if err := json.Unmarshal(data, &config); err != nil {
			fmt.Printf("  Warning: existing %s has invalid JSON, will create new\n", configPath)
			config = nil
		}
	}
	if config == nil {
		config = make(map[string]any)
	}

	servers, ok := config["mcpServers"].(map[string]any)
	if !ok {
		servers = make(map[string]any)
	}
	servers["proxmox"] = entry
	config["mcpServers"] = servers

	out, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		fatal("Failed to marshal config: %v", err)
	}

	configDir := filepath.Dir(configPath)
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		fatal("Failed to create config dir: %v", err)
	}
	if err := os.WriteFile(configPath, append(out, '\n'), 0o600); err != nil {
		fatal("Failed to write config: %v", err)
	}
	// Enforce permissions even on existing files
	if err := os.Chmod(configPath, 0o600); err != nil {
		fmt.Printf("  Warning: could not set permissions on %s: %v\n", configPath, err)
	}

	fmt.Printf("\n  ✓ Config written to %s (mode 0600)\n", configPath)
}

func printJSON(entry mcpServerEntry) {
	wrapper := map[string]any{
		"mcpServers": map[string]any{
			"proxmox": entry,
		},
	}
	out, _ := json.MarshalIndent(wrapper, "", "  ")
	fmt.Println()
	fmt.Println(string(out))
	fmt.Println()
	fmt.Println("  Add the \"proxmox\" block to your MCP client config.")
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "\n  Error: "+format+"\n", args...)
	os.Exit(1)
}
