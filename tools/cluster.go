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

	s.AddTool(mcp.NewTool("cluster_resources",
		mcp.WithDescription("List all resources across the cluster (VMs, containers, storage, nodes). Provides a unified view of resource usage."),
		mcp.WithString("type", mcp.Description("Filter by resource type: vm, storage, node, sdn (optional, lists all if omitted)")),
	), clusterResourcesHandler(client))

	s.AddTool(mcp.NewTool("cluster_next_id",
		mcp.WithDescription("Get the next available VM/container ID in the cluster. Useful for planning new deployments."),
	), clusterNextIDHandler(client))
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

func clusterResourcesHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cluster, err := client.Cluster(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster: %v", err)), nil
		}

		validResourceTypes := map[string]bool{"vm": true, "storage": true, "node": true, "sdn": true}
		var filters []string
		if t := optionalStr(req, "type", ""); t != "" {
			if !validResourceTypes[t] {
				return mcp.NewToolResultError(fmt.Sprintf("invalid resource type %q: must be vm, storage, node, or sdn", t)), nil
			}
			filters = append(filters, t)
		}

		resources, err := cluster.Resources(ctx, filters...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster resources: %v", err)), nil
		}

		type resourceSummary struct {
			ID       string  `json:"id"`
			Type     string  `json:"type"`
			Name     string  `json:"name,omitempty"`
			Node     string  `json:"node,omitempty"`
			Status   string  `json:"status,omitempty"`
			VMID     uint64  `json:"vmid,omitempty"`
			CPU      float64 `json:"cpu,omitempty"`
			MaxCPU   uint64  `json:"maxcpu,omitempty"`
			Mem      uint64  `json:"mem,omitempty"`
			MaxMem   uint64  `json:"maxmem,omitempty"`
			Disk     uint64  `json:"disk,omitempty"`
			MaxDisk  uint64  `json:"maxdisk,omitempty"`
			Uptime   uint64  `json:"uptime,omitempty"`
			Tags     string  `json:"tags,omitempty"`
			Pool     string  `json:"pool,omitempty"`
			Template uint64  `json:"template,omitempty"`
		}

		items := make([]resourceSummary, 0, len(resources))
		for _, r := range resources {
			items = append(items, resourceSummary{
				ID:       r.ID,
				Type:     r.Type,
				Name:     r.Name,
				Node:     r.Node,
				Status:   r.Status,
				VMID:     r.VMID,
				CPU:      r.CPU,
				MaxCPU:   r.MaxCPU,
				Mem:      r.Mem,
				MaxMem:   r.MaxMem,
				Disk:     r.Disk,
				MaxDisk:  r.MaxDisk,
				Uptime:   r.Uptime,
				Tags:     r.Tags,
				Pool:     r.Pool,
				Template: r.Template,
			})
		}

		return marshalResult(items)
	}
}

func clusterNextIDHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		cluster, err := client.Cluster(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster: %v", err)), nil
		}

		nextID, err := cluster.NextID(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get next ID: %v", err)), nil
		}

		result := struct {
			NextID int `json:"next_id"`
		}{NextID: nextID}

		return marshalResult(result)
	}
}
