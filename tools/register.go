package tools

import (
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"

	"github.com/ssimpson/ProxmoxMCP/config"
)

func Register(s *server.MCPServer, client *px.Client, cfg *config.Config) {
	registerClusterTools(s, client)
	registerNodeTools(s, client)
	registerQemuTools(s, client, cfg)
	registerLxcTools(s, client)
	registerStorageTools(s, client)
	registerTemplateTools(s, client)
}
