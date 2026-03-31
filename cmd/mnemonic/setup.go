package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// setupCommand configures mnemonic for a specific AI agent.
// Usage: mnemonic setup claude-code
func setupCommand(args []string) {
	if len(args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: mnemonic setup <agent>\n\nSupported agents:\n  claude-code    Configure Claude Code MCP settings\n  cursor         Configure Cursor MCP settings\n")
		os.Exit(exitUsage)
	}

	switch args[1] {
	case "claude-code":
		setupClaudeCode()
	case "cursor":
		setupCursor()
	default:
		fmt.Fprintf(os.Stderr, "Unknown agent: %s\nSupported: claude-code, cursor\n", args[1])
		os.Exit(exitUsage)
	}
}

func setupClaudeCode() {
	binaryPath := findBinary()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		die(exitGeneral, "cannot determine home directory", "")
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.local.json")
	mcpEntry := map[string]interface{}{
		"command": binaryPath,
		"args":    []string{"mcp"},
	}

	if err := upsertMCPConfig(settingsPath, "mnemonic", mcpEntry); err != nil {
		die(exitGeneral, fmt.Sprintf("failed to configure Claude Code: %v", err), "")
	}

	fmt.Printf("Claude Code configured.\n")
	fmt.Printf("  Settings: %s\n", settingsPath)
	fmt.Printf("  Binary:   %s\n", binaryPath)
	fmt.Printf("\nRestart Claude Code to pick up the changes.\n")
}

func setupCursor() {
	binaryPath := findBinary()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		die(exitGeneral, "cannot determine home directory", "")
	}

	// Cursor uses ~/.cursor/mcp.json
	settingsPath := filepath.Join(homeDir, ".cursor", "mcp.json")
	mcpEntry := map[string]interface{}{
		"command": binaryPath,
		"args":    []string{"mcp"},
	}

	if err := upsertMCPConfig(settingsPath, "mnemonic", mcpEntry); err != nil {
		die(exitGeneral, fmt.Sprintf("failed to configure Cursor: %v", err), "")
	}

	fmt.Printf("Cursor configured.\n")
	fmt.Printf("  Settings: %s\n", settingsPath)
	fmt.Printf("  Binary:   %s\n", binaryPath)
	fmt.Printf("\nRestart Cursor to pick up the changes.\n")
}

// findBinary returns the absolute path to the mnemonic binary.
func findBinary() string {
	// Try the running binary first
	exe, err := os.Executable()
	if err == nil {
		abs, err := filepath.EvalSymlinks(exe)
		if err == nil {
			return abs
		}
		return exe
	}

	// Fall back to PATH lookup
	path, err := exec.LookPath("mnemonic")
	if err == nil {
		abs, _ := filepath.Abs(path)
		return abs
	}

	die(exitGeneral, "cannot find mnemonic binary", "ensure mnemonic is in your PATH")
	return ""
}

// upsertMCPConfig reads a JSON settings file, adds/updates the mcpServers entry,
// and writes it back. Creates the file and parent directory if needed.
func upsertMCPConfig(path string, serverName string, entry map[string]interface{}) error {
	// Ensure parent directory exists
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}

	// Read existing settings or start fresh
	settings := make(map[string]interface{})
	data, err := os.ReadFile(path)
	if err == nil {
		// Trim BOM and whitespace
		content := strings.TrimSpace(string(data))
		if content != "" {
			if err := json.Unmarshal([]byte(content), &settings); err != nil {
				return fmt.Errorf("parsing %s: %w", path, err)
			}
		}
	}

	// Ensure mcpServers key exists
	servers, ok := settings["mcpServers"].(map[string]interface{})
	if !ok {
		servers = make(map[string]interface{})
	}

	// Check if already configured
	if existing, ok := servers[serverName]; ok {
		fmt.Printf("mnemonic is already configured in %s\n", path)
		existingJSON, _ := json.MarshalIndent(existing, "  ", "  ")
		fmt.Printf("  Current config: %s\n", existingJSON)
		fmt.Printf("  Updating...\n")
	}

	servers[serverName] = entry
	settings["mcpServers"] = servers

	// Write back with nice formatting
	output, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}

	if err := os.WriteFile(path, append(output, '\n'), 0o644); err != nil {
		return fmt.Errorf("writing %s: %w", path, err)
	}

	return nil
}
