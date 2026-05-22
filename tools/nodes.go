package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerNodeTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("node_list",
		mcp.WithDescription("List all nodes in the cluster with status summary (CPU, memory, uptime)"),
	), nodeListHandler(client))

	s.AddTool(mcp.NewTool("node_status",
		mcp.WithDescription("Get detailed status for a specific node including CPU, memory, kernel version, and PVE version"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
	), nodeStatusHandler(client))
}

func nodeListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodes, err := client.Nodes(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list nodes: %v", err)), nil
		}

		type nodeSummary struct {
			Node    string  `json:"node"`
			Status  string  `json:"status"`
			CPU     float64 `json:"cpu"`
			MaxCPU  int     `json:"maxcpu"`
			Mem     uint64  `json:"mem"`
			MaxMem  uint64  `json:"maxmem"`
			Disk    uint64  `json:"disk"`
			MaxDisk uint64  `json:"maxdisk"`
			Uptime  uint64  `json:"uptime"`
		}

		result := make([]nodeSummary, 0, len(nodes))
		for _, n := range nodes {
			result = append(result, nodeSummary{
				Node:    n.Node,
				Status:  n.Status,
				CPU:     n.CPU,
				MaxCPU:  n.MaxCPU,
				Mem:     n.Mem,
				MaxMem:  n.MaxMem,
				Disk:    n.Disk,
				MaxDisk: n.MaxDisk,
				Uptime:  n.Uptime,
			})
		}

		return marshalResult(result)
	}
}

func nodeStatusHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := requiredNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		type nodeDetail struct {
			Name        string   `json:"name"`
			Kversion    string   `json:"kernel_version"`
			PVEVersion  string   `json:"pve_version"`
			CPUModel    string   `json:"cpu_model"`
			CPUCores    int      `json:"cpu_cores"`
			CPUSockets  int      `json:"cpu_sockets"`
			CPUUsage    float64  `json:"cpu_usage"`
			LoadAvg     []string `json:"load_average"`
			MemTotal    uint64   `json:"memory_total"`
			MemUsed     uint64   `json:"memory_used"`
			MemFree     uint64   `json:"memory_free"`
			SwapTotal   uint64   `json:"swap_total"`
			SwapUsed    uint64   `json:"swap_used"`
			SwapFree    uint64   `json:"swap_free"`
			RootFSTotal uint64   `json:"rootfs_total"`
			RootFSUsed  uint64   `json:"rootfs_used"`
			RootFSFree  uint64   `json:"rootfs_free"`
			Uptime      uint64   `json:"uptime"`
		}

		detail := nodeDetail{
			Name:        node.Name,
			Kversion:    node.Kversion,
			PVEVersion:  node.PVEVersion,
			CPUModel:    node.CPUInfo.Model,
			CPUCores:    node.CPUInfo.Cores,
			CPUSockets:  node.CPUInfo.Sockets,
			CPUUsage:    node.CPU,
			LoadAvg:     node.LoadAvg,
			MemTotal:    node.Memory.Total,
			MemUsed:     node.Memory.Used,
			MemFree:     node.Memory.Free,
			SwapTotal:   node.Swap.Total,
			SwapUsed:    node.Swap.Used,
			SwapFree:    node.Swap.Free,
			RootFSTotal: node.RootFS.Total,
			RootFSUsed:  node.RootFS.Used,
			RootFSFree:  node.RootFS.Free,
			Uptime:      node.Uptime,
		}

		return marshalResult(detail)
	}
}
