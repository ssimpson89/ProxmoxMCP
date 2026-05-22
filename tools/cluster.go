package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerClusterTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("cluster_status",
		mcp.WithDescription("Get cluster health, quorum status, and node membership"),
	), clusterStatusHandler(client))
}

func clusterStatusHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cluster, err := client.Cluster(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster status: %v", err)), nil
		}

		type nodeInfo struct {
			Name   string `json:"name"`
			ID     string `json:"id"`
			Status string `json:"status"`
			Online int    `json:"online"`
			Local  int    `json:"local"`
		}

		type clusterInfo struct {
			Name    string     `json:"name"`
			ID      string     `json:"id"`
			Version int        `json:"version"`
			Quorate int        `json:"quorate"`
			Nodes   []nodeInfo `json:"nodes"`
		}

		nodes := make([]nodeInfo, 0, len(cluster.Nodes))
		for _, n := range cluster.Nodes {
			nodes = append(nodes, nodeInfo{
				Name:   n.Name,
				ID:     n.ID,
				Status: n.Status,
				Online: n.Online,
				Local:  n.Local,
			})
		}

		info := clusterInfo{
			Name:    cluster.Name,
			ID:      cluster.ID,
			Version: cluster.Version,
			Quorate: cluster.Quorate,
			Nodes:   nodes,
		}

		return marshalResult(info)
	}
}
