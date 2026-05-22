package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerStorageTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("storage_list",
		mcp.WithDescription("List storage pools, optionally filtered by node. Shows capacity, usage, type, and content types."),
		mcp.WithString("node", mcp.Description("Filter by node name (optional, lists cluster-wide storage if omitted)")),
	), storageListHandler(client))

	s.AddTool(mcp.NewTool("storage_content",
		mcp.WithDescription("List the content (ISOs, disk images, backups, templates) of a storage pool on a node"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithString("storage", mcp.Required(), mcp.Description("Storage pool name")),
	), storageContentHandler(client))
}

func storageListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := optionalNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type storageSummary struct {
			Name         string  `json:"name"`
			Node         string  `json:"node,omitempty"`
			Type         string  `json:"type"`
			Content      string  `json:"content"`
			Active       int     `json:"active"`
			Enabled      int     `json:"enabled"`
			Shared       int     `json:"shared"`
			Total        uint64  `json:"total"`
			Used         uint64  `json:"used"`
			Avail        uint64  `json:"avail"`
			UsedFraction float64 `json:"used_fraction"`
		}

		items := make([]storageSummary, 0)

		if nodeName != "" {
			node, err := client.Node(ctx, nodeName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
			}
			storages, err := node.Storages(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list storage on %q: %v", nodeName, err)), nil
			}
			for _, s := range storages {
				items = append(items, storageSummary{
					Name:         s.Name,
					Node:         s.Node,
					Type:         s.Type,
					Content:      s.Content,
					Active:       s.Active,
					Enabled:      s.Enabled,
					Shared:       s.Shared,
					Total:        s.Total,
					Used:         s.Used,
					Avail:        s.Avail,
					UsedFraction: s.UsedFraction,
				})
			}
			return marshalResult(items)
		}

		nodeErrs, err := forEachNode(ctx, client, func(node *px.Node) error {
			storages, err := node.Storages(ctx)
			if err != nil {
				return err
			}
			for _, s := range storages {
				items = append(items, storageSummary{
					Name:         s.Name,
					Node:         s.Node,
					Type:         s.Type,
					Content:      s.Content,
					Active:       s.Active,
					Enabled:      s.Enabled,
					Shared:       s.Shared,
					Total:        s.Total,
					Used:         s.Used,
					Avail:        s.Avail,
					UsedFraction: s.UsedFraction,
				})
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(nodeErrs) > 0 {
			return marshalResult(listResult[storageSummary]{Items: items, Errors: nodeErrs})
		}
		return marshalResult(items)
	}
}

func storageContentHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := requiredNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		storageName, err := requiredStorageName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		storage, err := node.Storage(ctx, storageName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get storage %q: %v", storageName, err)), nil
		}

		content, err := storage.GetContent(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list storage content: %v", err)), nil
		}

		type contentItem struct {
			Volid  string `json:"volid"`
			Format string `json:"format"`
			Size   uint64 `json:"size"`
			VMID   uint64 `json:"vmid,omitempty"`
			Notes  string `json:"notes,omitempty"`
		}

		result := make([]contentItem, 0, len(content))
		for _, c := range content {
			result = append(result, contentItem{
				Volid:  c.Volid,
				Format: c.Format,
				Size:   c.Size,
				VMID:   c.VMID,
				Notes:  c.Notes,
			})
		}

		return marshalResult(result)
	}
}
