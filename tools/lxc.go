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

	s.AddTool(mcp.NewTool("lxc_snapshot_rollback",
		mcp.WithDescription("Rollback an LXC container to a named snapshot. DESTRUCTIVE: current state will be lost."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name to rollback to")),
	), lxcSnapshotRollbackHandler(client))

	s.AddTool(mcp.NewTool("lxc_snapshot_delete",
		mcp.WithDescription("Delete a snapshot from an LXC container. DESTRUCTIVE: the snapshot data is permanently removed."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name to delete")),
	), lxcSnapshotDeleteHandler(client))

	s.AddTool(mcp.NewTool("lxc_resize",
		mcp.WithDescription("Resize an LXC container disk/filesystem. Can only increase size, not shrink."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
		mcp.WithString("disk", mcp.Required(), mcp.Description("Disk/volume name (e.g. rootfs, mp0)")),
		mcp.WithString("size", mcp.Required(), mcp.Description("New size or size increment (e.g. +10G, 50G)")),
	), lxcResizeHandler(client))

	s.AddTool(mcp.NewTool("lxc_migrate",
		mcp.WithDescription("Migrate an LXC container to a different node."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Current node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target node name")),
		mcp.WithBoolean("online", mcp.Description("Online/live migration (default: false)")),
		mcp.WithBoolean("restart", mcp.Description("Restart container after migration on target node (default: false)")),
	), lxcMigrateHandler(client))

	s.AddTool(mcp.NewTool("lxc_template_create",
		mcp.WithDescription("Convert an existing LXC container into a template. The container must be stopped. Once converted, the container cannot be started — it can only be cloned."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Container ID to convert")),
	), lxcTemplateCreateHandler(client))

	s.AddTool(mcp.NewTool("lxc_clone",
		mcp.WithDescription("Clone an LXC container or container template. Supports full clones (independent copy) and linked clones (shares base image). Linked clones are faster but depend on the source."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node the source container is on")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Source container ID to clone from")),
		mcp.WithString("hostname", mcp.Required(), mcp.Description("Hostname for the new container")),
		mcp.WithBoolean("full", mcp.Description("true for full clone, false for linked clone (default: true)")),
		mcp.WithNumber("newid", mcp.Description("Container ID for the clone (optional, auto-assigned if omitted)")),
		mcp.WithString("storage", mcp.Description("Target storage for full clone (optional)")),
		mcp.WithString("target", mcp.Description("Target node (optional, same node if omitted)")),
		mcp.WithString("pool", mcp.Description("Resource pool to add the clone to (optional)")),
	), lxcCloneHandler(client))
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

		collectContainers := func(node *px.Node) error {
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
		}

		if nodeName != "" {
			node, err := client.Node(ctx, nodeName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
			}
			if err := collectContainers(node); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list containers on %q: %v", nodeName, err)), nil
			}
			return marshalResult(items)
		}

		nodeErrs, err := forEachNode(ctx, client, collectContainers)
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

func lxcTemplateCreateHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, nodeName, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if ct.Status != "stopped" {
			return mcp.NewToolResultError(fmt.Sprintf("Container %d must be stopped before converting to template (current status: %s)", vmid, ct.Status)), nil
		}

		if err := ct.Template(ctx); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to convert container %d to template: %v", vmid, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully converted container %d (%s) to template on node %s", vmid, ct.Name, nodeName)), nil
	}
}

func lxcCloneHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, nodeName, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		hostname, err := req.RequireString("hostname")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		storageOpt := optionalStr(req, "storage", "")
		if storageOpt != "" && !storageNameRe.MatchString(storageOpt) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid storage name %q", storageOpt)), nil
		}
		targetOpt := optionalStr(req, "target", "")
		if targetOpt != "" && !nodeNameRe.MatchString(targetOpt) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid target node name %q", targetOpt)), nil
		}

		opts := &px.ContainerCloneOptions{
			Hostname: hostname,
			Full:     1,
			Storage:  storageOpt,
			Target:   targetOpt,
			Pool:     optionalStr(req, "pool", ""),
		}

		if v, ok := req.GetArguments()["full"].(bool); ok && !v {
			opts.Full = 0
		}

		newid, _ := optionalInt(req, "newid")
		if newid > 0 {
			opts.NewID = newid
		}

		clonedID, task, err := ct.Clone(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to clone container %d: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Clone task failed: %v", err)), nil
		}

		cloneType := "full"
		if opts.Full == 0 {
			cloneType = "linked"
		}
		targetNode := nodeName
		if opts.Target != "" {
			targetNode = opts.Target
		}

		result := struct {
			VMID       int    `json:"vmid"`
			Hostname   string `json:"hostname"`
			SourceVMID int    `json:"source_vmid"`
			Node       string `json:"node"`
			CloneType  string `json:"clone_type"`
			Message    string `json:"message"`
		}{
			VMID:       clonedID,
			Hostname:   hostname,
			SourceVMID: vmid,
			Node:       targetNode,
			CloneType:  cloneType,
			Message:    fmt.Sprintf("Successfully cloned container %d to %d (%s clone)", vmid, clonedID, cloneType),
		}

		return marshalResult(result)
	}
}

func lxcSnapshotRollbackHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := ct.RollbackSnapshot(ctx, snapName, false)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to rollback snapshot: %v", err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot rollback failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully rolled back container %d to snapshot %q", vmid, snapName)), nil
	}
}

func lxcSnapshotDeleteHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := ct.DeleteSnapshot(ctx, snapName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete snapshot: %v", err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot deletion failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted snapshot %q from container %d", snapName, vmid)), nil
	}
}

func lxcResizeHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		disk, err := req.RequireString("disk")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !lxcDiskRe.MatchString(disk) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid disk name %q: must be rootfs or mpN", disk)), nil
		}
		size, err := req.RequireString("size")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !diskSizeRe.MatchString(size) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid size %q: must be like +10G, 50G, 100M", size)), nil
		}

		task, err := ct.Resize(ctx, disk, size)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resize disk %s on container %d: %v", disk, vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Disk resize failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully resized disk %s to %s on container %d", disk, size, vmid)), nil
	}
}

func lxcMigrateHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		ct, _, vmid, err := withContainer(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		target, err := req.RequireString("target")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !nodeNameRe.MatchString(target) {
			return mcp.NewToolResultError(fmt.Sprintf("Invalid target node name %q", target)), nil
		}

		opts := &px.ContainerMigrateOptions{
			Target: target,
		}

		// IntOrBool(false) is dropped by omitempty in the library's JSON tags,
		// so we can only explicitly send true.
		args := req.GetArguments()
		if v, ok := args["online"].(bool); ok && v {
			opts.Online = px.IntOrBool(true)
		}
		if v, ok := args["restart"].(bool); ok && v {
			opts.Restart = px.IntOrBool(true)
		}

		task, err := ct.Migrate(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to migrate container %d: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Migration failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully migrated container %d to node %s", vmid, target)), nil
	}
}
