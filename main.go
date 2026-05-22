package main

import (
	"context"
	"fmt"
	"os"

	"github.com/mark3labs/mcp-go/server"

	"github.com/ssimpson/ProxmoxMCP/config"
	"github.com/ssimpson/ProxmoxMCP/proxmox"
	"github.com/ssimpson/ProxmoxMCP/tools"
)

var version = "0.1.0"

func main() {
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Configuration error: %v\n", err)
		os.Exit(1)
	}

	client, err := proxmox.NewClient(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create Proxmox client: %v\n", err)
		os.Exit(1)
	}

	if err := proxmox.ValidateConnection(context.Background(), client); err != nil {
		fmt.Fprintf(os.Stderr, "Startup check failed: %v\n", err)
		os.Exit(1)
	}

	s := server.NewMCPServer(
		"proxmox-mcp",
		version,
		server.WithToolCapabilities(true),
	)

	tools.Register(s, client, cfg)

	if err := server.ServeStdio(s); err != nil {
		fmt.Fprintf(os.Stderr, "Server error: %v\n", err)
		os.Exit(1)
	}
}
