package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

const lxcShutdownTimeout = 60

func registerLxcTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("lxc_list",
		mcp.WithDescription("List all LXC containers across the cluster, or on a specific node"),
		mcp.WithString("node", mcp.Description("Filter by node name (optional, lists all nodes if omitted)")),
	), lxcListHandler(client))

	s.AddTool(mcp.NewTool("lxc_status",
		mcp.WithDescription("Get detailed LXC container configuration and runtime status"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcStatusHandler(client))

	s.AddTool(mcp.NewTool("lxc_start",
		mcp.WithDescription("Start a stopped LXC container"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcActionHandler(client, "start", func(ctx context.Context, ct *px.Container) (*px.Task, error) {
		return ct.Start(ctx)
	}))

	s.AddTool(mcp.NewTool("lxc_stop",
		mcp.WithDescription("Force stop an LXC container. WARNING: may cause data loss. Prefer lxc_shutdown for graceful stop."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcActionHandler(client, "stop", func(ctx context.Context, ct *px.Container) (*px.Task, error) {
		return ct.Stop(ctx)
	}))

	s.AddTool(mcp.NewTool("lxc_shutdown",
		mcp.WithDescription("Gracefully shut down an LXC container"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcActionHandler(client, "shutdown", func(ctx context.Context, ct *px.Container) (*px.Task, error) {
		return ct.Shutdown(ctx, false, lxcShutdownTimeout)
	}))

	s.AddTool(mcp.NewTool("lxc_reboot",
		mcp.WithDescription("Reboot a running LXC container"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcActionHandler(client, "reboot", func(ctx context.Context, ct *px.Container) (*px.Task, error) {
		return ct.Reboot(ctx)
	}))

	s.AddTool(mcp.NewTool("lxc_snapshot_list",
		mcp.WithDescription("List all snapshots for an LXC container"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
	), lxcSnapshotListHandler(client))

	s.AddTool(mcp.NewTool("lxc_snapshot_create",
		mcp.WithDescription("Create a new snapshot of an LXC container"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name")),
	), lxcSnapshotCreateHandler(client))
}

func lxcListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := optionalNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type ctSummary struct {
			VMID   int    `json:"vmid"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Node   string `json:"node"`
			CPUs   int    `json:"cpus"`
			MaxMem uint64 `json:"maxmem"`
			Uptime uint64 `json:"uptime"`
		}

		items := make([]ctSummary, 0)

		if nodeName != "" {
			node, err := client.Node(ctx, nodeName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
			}
			cts, err := node.Containers(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list containers on %q: %v", nodeName, err)), nil
			}
			for _, ct := range cts {
				items = append(items, ctSummary{
					VMID:   int(ct.VMID),
					Name:   ct.Name,
					Status: ct.Status,
					Node:   ct.Node,
					CPUs:   ct.CPUs,
					MaxMem: ct.MaxMem,
					Uptime: ct.Uptime,
				})
			}
			return marshalResult(items)
		}

		nodeErrs, err := forEachNode(ctx, client, func(node *px.Node) error {
			cts, err := node.Containers(ctx)
			if err != nil {
				return err
			}
			for _, ct := range cts {
				items = append(items, ctSummary{
					VMID:   int(ct.VMID),
					Name:   ct.Name,
					Status: ct.Status,
					Node:   ct.Node,
					CPUs:   ct.CPUs,
					MaxMem: ct.MaxMem,
					Uptime: ct.Uptime,
				})
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(nodeErrs) > 0 {
			return marshalResult(listResult[ctSummary]{Items: items, Errors: nodeErrs})
		}
		return marshalResult(items)
	}
}

func lxcStatusHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type ctDetail struct {
			VMID    int    `json:"vmid"`
			Name    string `json:"name"`
			Status  string `json:"status"`
			Node    string `json:"node"`
			CPUs    int    `json:"cpus"`
			MaxMem  uint64 `json:"maxmem"`
			MaxDisk uint64 `json:"maxdisk"`
			MaxSwap uint64 `json:"maxswap"`
			Uptime  uint64 `json:"uptime"`
			Tags    string `json:"tags,omitempty"`
			Config  any    `json:"config,omitempty"`
		}

		detail := ctDetail{
			VMID:    vmid,
			Name:    ct.Name,
			Status:  ct.Status,
			Node:    ct.Node,
			CPUs:    ct.CPUs,
			MaxMem:  ct.MaxMem,
			MaxDisk: ct.MaxDisk,
			MaxSwap: ct.MaxSwap,
			Uptime:  ct.Uptime,
			Tags:    ct.Tags,
			Config:  sanitizeConfig(ct.ContainerConfig),
		}

		return marshalResult(detail)
	}
}

func lxcActionHandler(client *px.Client, action string, fn func(ctx context.Context, ct *px.Container) (*px.Task, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, nodeName, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := fn(ctx, ct)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to %s container %d: %v", action, vmid, err)), nil
		}

		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Task failed for %s container %d: %v", action, vmid, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully sent %s to container %d on node %s", action, vmid, nodeName)), nil
	}
}

func lxcSnapshotListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, _, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		snapshots, err := ct.Snapshots(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list snapshots: %v", err)), nil
		}

		type snapInfo struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Snaptime    int64  `json:"snaptime,omitempty"`
			Parent      string `json:"parent,omitempty"`
		}

		result := make([]snapInfo, 0, len(snapshots))
		for _, s := range snapshots {
			result = append(result, snapInfo{
				Name:        s.Name,
				Description: s.Description,
				Snaptime:    s.SnapshotCreationTime,
				Parent:      s.Parent,
			})
		}

		return marshalResult(result)
	}
}

func lxcSnapshotCreateHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := ct.NewSnapshot(ctx, snapName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create snapshot: %v", err)), nil
		}

		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot creation failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully created snapshot %q for container %d", snapName, vmid)), nil
	}
}
