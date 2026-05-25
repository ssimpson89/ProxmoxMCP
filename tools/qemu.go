package tools

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"

	"github.com/ssimpson/ProxmoxMCP/config"
)

func registerQemuTools(s *server.MCPServer, client *px.Client, cfg *config.Config) {
	s.AddTool(mcp.NewTool("qemu_list",
		mcp.WithDescription("List all QEMU virtual machines across the cluster, or on a specific node"),
		mcp.WithString("node", mcp.Description("Filter by node name (optional, lists all nodes if omitted)")),
	), qemuListHandler(client))

	s.AddTool(mcp.NewTool("qemu_create_from_url",
		mcp.WithDescription("Create a new VM (not a template) by downloading a disk image (qcow2/raw) from a URL and importing it. The image is downloaded directly by the Proxmox node. Use this when you want a regular VM from an image URL; use template_create instead if you want a reusable template."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node to create the VM on")),
		mcp.WithString("name", mcp.Required(), mcp.Description("VM name (e.g. webserver-01)")),
		mcp.WithString("url", mcp.Required(), mcp.Description("URL to a qcow2 or raw disk image")),
		mcp.WithString("storage", mcp.Required(), mcp.Description("Storage pool for the disk (e.g. local-lvm, ceph-pool)")),
		mcp.WithNumber("vmid", mcp.Description("VM ID (optional, auto-assigned if omitted)")),
		mcp.WithNumber("memory", mcp.Description("Memory in MB (default: 2048)")),
		mcp.WithNumber("cores", mcp.Description("CPU cores (default: 2)")),
		mcp.WithString("net", mcp.Description("Network config (default: virtio,bridge=vmbr0)")),
		mcp.WithString("bios", mcp.Description("Firmware type: seabios (default) or ovmf (UEFI)")),
		mcp.WithString("ostype", mcp.Description("Guest OS type: l26 (Linux 2.6+), win11, win10, etc.")),
		mcp.WithString("vga", mcp.Description("VGA type: std, virtio, qxl, serial0, none")),
		mcp.WithString("machine", mcp.Description("Machine type: q35 or i440fx")),
		mcp.WithString("tags", mcp.Description("Semicolon-separated tags (e.g. linux;production)")),
		mcp.WithString("description", mcp.Description("VM description")),
		mcp.WithString("agent", mcp.Description("QEMU Guest Agent config (default: enabled=1)")),
		mcp.WithString("scsihw", mcp.Description("SCSI controller type (default: virtio-scsi-pci)")),
		mcp.WithString("cpu", mcp.Description("CPU type (default: host — gives the guest full host CPU feature visibility for best performance; set to e.g. kvm64 or x86-64-v2-AES if you need cross-CPU live migration)")),
		mcp.WithString("disk_size", mcp.Description("Resize the imported disk after import (e.g. +10G to grow by 10GB, 50G to set to 50GB)")),
		mcp.WithBoolean("start", mcp.Description("Start the VM after creation (default: false)")),
	), qemuCreateFromURLHandler(client))

	s.AddTool(mcp.NewTool("qemu_status",
		mcp.WithDescription("Get detailed VM configuration and runtime status"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuStatusHandler(client))

	s.AddTool(mcp.NewTool("qemu_start",
		mcp.WithDescription("Start a stopped virtual machine"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "start", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Start(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_stop",
		mcp.WithDescription("Force stop a virtual machine. WARNING: This immediately stops the VM and may cause data loss. Prefer qemu_shutdown for graceful shutdown."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "stop", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Stop(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_shutdown",
		mcp.WithDescription("Send ACPI shutdown signal to a virtual machine for graceful shutdown"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "shutdown", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Shutdown(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_reboot",
		mcp.WithDescription("Reboot a running virtual machine"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "reboot", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Reboot(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_reset",
		mcp.WithDescription("Hard reset a virtual machine (like pressing the reset button). DESTRUCTIVE: may cause data loss in the guest OS."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "reset", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Reset(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_suspend",
		mcp.WithDescription("Suspend a virtual machine to RAM (pause CPU execution)"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "suspend", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Pause(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_resume",
		mcp.WithDescription("Resume a suspended virtual machine"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuActionHandler(client, "resume", func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error) {
		return vm.Resume(ctx)
	}))

	s.AddTool(mcp.NewTool("qemu_snapshot_list",
		mcp.WithDescription("List all snapshots for a virtual machine"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuSnapshotListHandler(client))

	s.AddTool(mcp.NewTool("qemu_snapshot_create",
		mcp.WithDescription("Create a new snapshot of a virtual machine"),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name")),
	), qemuSnapshotCreateHandler(client))

	s.AddTool(mcp.NewTool("qemu_snapshot_rollback",
		mcp.WithDescription("Rollback a virtual machine to a named snapshot. DESTRUCTIVE: current state will be lost."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name to rollback to")),
	), qemuSnapshotRollbackHandler(client))

	s.AddTool(mcp.NewTool("qemu_snapshot_delete",
		mcp.WithDescription("Delete a snapshot from a virtual machine. DESTRUCTIVE: the snapshot data is permanently removed."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Snapshot name to delete")),
	), qemuSnapshotDeleteHandler(client))

	s.AddTool(mcp.NewTool("qemu_resize_disk",
		mcp.WithDescription("Resize a VM disk. Can only increase size, not shrink. Size format: +10G (add 10GB), 50G (set to 50GB)."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("disk", mcp.Required(), mcp.Description("Disk name (e.g. scsi0, virtio0, ide0)")),
		mcp.WithString("size", mcp.Required(), mcp.Description("New size or size increment (e.g. +10G, 50G)")),
	), qemuResizeDiskHandler(client))

	s.AddTool(mcp.NewTool("qemu_migrate",
		mcp.WithDescription("Migrate a VM to a different node. Supports online (live) migration and offline migration."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Current node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("target", mcp.Required(), mcp.Description("Target node name")),
		mcp.WithBoolean("online", mcp.Description("Live migration (default: true if VM is running)")),
		mcp.WithString("targetstorage", mcp.Description("Target storage for local disks (optional, required if disks are on local storage)")),
		mcp.WithBoolean("with_local_disks", mcp.Description("Migrate local disks to target storage (default: false)")),
	), qemuMigrateHandler(client))

	s.AddTool(mcp.NewTool("qemu_agent_info",
		mcp.WithDescription("Get guest OS information, hostname, and network interfaces from a running VM via the QEMU Guest Agent. Requires the guest agent to be installed and running."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
	), qemuAgentInfoHandler(client))

	execHandler := toolError("qemu_exec is disabled. Set PROXMOX_ALLOW_EXEC=true to enable command execution.")
	if cfg.AllowExec {
		execHandler = qemuExecHandler(client)
	}
	s.AddTool(mcp.NewTool("qemu_exec",
		mcp.WithDescription("Execute a command inside a VM via the QEMU Guest Agent. Requires the guest agent to be installed and running. Disabled by default; set PROXMOX_ALLOW_EXEC=true to enable."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM ID")),
		mcp.WithString("command", mcp.Required(), mcp.Description("Command to execute (passed to /bin/sh -c)")),
	), execHandler)
}

func qemuListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := optionalNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type vmSummary struct {
			VMID   int    `json:"vmid"`
			Name   string `json:"name"`
			Status string `json:"status"`
			Node   string `json:"node"`
			CPUs   int    `json:"cpus"`
			MaxMem uint64 `json:"maxmem"`
			Uptime uint64 `json:"uptime"`
		}

		items := make([]vmSummary, 0)

		if nodeName != "" {
			node, err := client.Node(ctx, nodeName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
			}
			vms, err := node.VirtualMachines(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list VMs on %q: %v", nodeName, err)), nil
			}
			for _, vm := range vms {
				items = append(items, vmSummary{
					VMID:   int(vm.VMID),
					Name:   vm.Name,
					Status: vm.Status,
					Node:   vm.Node,
					CPUs:   vm.CPUs,
					MaxMem: vm.MaxMem,
					Uptime: vm.Uptime,
				})
			}
			return marshalResult(items)
		}

		nodeErrs, err := forEachNode(ctx, client, func(node *px.Node) error {
			vms, err := node.VirtualMachines(ctx)
			if err != nil {
				return err
			}
			for _, vm := range vms {
				items = append(items, vmSummary{
					VMID:   int(vm.VMID),
					Name:   vm.Name,
					Status: vm.Status,
					Node:   vm.Node,
					CPUs:   vm.CPUs,
					MaxMem: vm.MaxMem,
					Uptime: vm.Uptime,
				})
			}
			return nil
		})
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if len(nodeErrs) > 0 {
			return marshalResult(listResult[vmSummary]{Items: items, Errors: nodeErrs})
		}
		return marshalResult(items)
	}
}

func qemuStatusHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type vmDetail struct {
			VMID      int     `json:"vmid"`
			Name      string  `json:"name"`
			Status    string  `json:"status"`
			QMPStatus string  `json:"qmpstatus,omitempty"`
			Node      string  `json:"node"`
			CPUs      int     `json:"cpus"`
			MaxMem    uint64  `json:"maxmem"`
			MaxDisk   uint64  `json:"maxdisk"`
			Mem       uint64  `json:"mem"`
			CPU       float64 `json:"cpu"`
			Uptime    uint64  `json:"uptime"`
			Tags      string  `json:"tags,omitempty"`
			Config    any     `json:"config,omitempty"`
		}

		detail := vmDetail{
			VMID:      vmid,
			Name:      vm.Name,
			Status:    vm.Status,
			QMPStatus: vm.QMPStatus,
			Node:      vm.Node,
			CPUs:      vm.CPUs,
			MaxMem:    vm.MaxMem,
			MaxDisk:   vm.MaxDisk,
			Mem:       vm.Mem,
			CPU:       vm.CPU,
			Uptime:    vm.Uptime,
			Tags:      vm.Tags,
			Config:    sanitizeConfig(vm.VirtualMachineConfig),
		}

		return marshalResult(detail)
	}
}

func qemuActionHandler(client *px.Client, action string, fn func(ctx context.Context, vm *px.VirtualMachine) (*px.Task, error)) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, nodeName, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := fn(ctx, vm)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to %s VM %d: %v", action, vmid, err)), nil
		}

		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Task failed for %s VM %d: %v", action, vmid, err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully sent %s to VM %d on node %s", action, vmid, nodeName)), nil
	}
}

func qemuSnapshotListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, _, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		snapshots, err := vm.Snapshots(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list snapshots: %v", err)), nil
		}

		type snapInfo struct {
			Name        string `json:"name"`
			Description string `json:"description,omitempty"`
			Snaptime    int64  `json:"snaptime,omitempty"`
			VMState     int    `json:"vmstate"`
			Parent      string `json:"parent,omitempty"`
		}

		result := make([]snapInfo, 0, len(snapshots))
		for _, s := range snapshots {
			result = append(result, snapInfo{
				Name:        s.Name,
				Description: s.Description,
				Snaptime:    s.Snaptime,
				VMState:     s.Vmstate,
				Parent:      s.Parent,
			})
		}

		return marshalResult(result)
	}
}

func qemuSnapshotCreateHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := vm.NewSnapshot(ctx, snapName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create snapshot: %v", err)), nil
		}

		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot creation failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully created snapshot %q for VM %d", snapName, vmid)), nil
	}
}

func qemuSnapshotRollbackHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := vm.SnapshotRollback(ctx, snapName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to rollback snapshot: %v", err)), nil
		}

		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot rollback failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully rolled back VM %d to snapshot %q", vmid, snapName)), nil
	}
}

func qemuExecHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		command, err := req.RequireString("command")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		pid, err := vm.AgentExec(ctx, []string{"/bin/sh", "-c", command}, "")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to execute command in VM %d: %v", vmid, err)), nil
		}

		status, err := vm.WaitForAgentExecExit(ctx, pid, 30)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Command execution timed out or failed in VM %d: %v", vmid, err)), nil
		}

		result := struct {
			ExitCode int    `json:"exit_code"`
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
		}{
			ExitCode: status.ExitCode,
			Stdout:   status.OutData,
			Stderr:   status.ErrData,
		}

		data, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal exec result: %v", err)), nil
		}

		return mcp.NewToolResultText(string(data)), nil
	}
}

func qemuSnapshotDeleteHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		snapName, err := requiredSnapName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		task, err := vm.DeleteSnapshot(ctx, snapName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete snapshot: %v", err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Snapshot deletion failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted snapshot %q from VM %d", snapName, vmid)), nil
	}
}

func qemuResizeDiskHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		disk, err := req.RequireString("disk")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !qemuDiskRe.MatchString(disk) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid disk name %q: must be scsiN, virtioN, ideN, sataN, or efidiskN", disk)), nil
		}
		size, err := req.RequireString("size")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if !diskSizeRe.MatchString(size) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid size %q: must be like +10G, 50G, 100M", size)), nil
		}

		task, err := vm.ResizeDisk(ctx, disk, size)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to resize disk %s on VM %d: %v", disk, vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Disk resize failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully resized disk %s to %s on VM %d", disk, size, vmid)), nil
	}
}

func qemuMigrateHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
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

		opts := &px.VirtualMachineMigrateOptions{
			Target:        target,
			TargetStorage: optionalStr(req, "targetstorage", ""),
		}

		// IntOrBool(false) is dropped by omitempty in the library's JSON tags,
		// so we can only explicitly send true. When omitted, Proxmox defaults
		// to online migration for running VMs and offline for stopped VMs.
		args := req.GetArguments()
		if v, ok := args["online"].(bool); ok && v {
			opts.Online = px.IntOrBool(true)
		}
		if v, ok := args["with_local_disks"].(bool); ok && v {
			opts.WithLocalDisks = px.IntOrBool(true)
		}

		task, err := vm.Migrate(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to migrate VM %d: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Migration failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully migrated VM %d to node %s", vmid, target)), nil
	}
}

func qemuCreateFromURLHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := requiredNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		imageURL, err := req.RequireString("url")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		storageName, err := requiredStorageName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		vmid, _ := optionalInt(req, "vmid")
		memory := optionalIntDefault(req, "memory", 2048)
		cores := optionalIntDefault(req, "cores", 2)

		netConfig := optionalStr(req, "net", "virtio,bridge=vmbr0")
		agentConfig := optionalStr(req, "agent", "enabled=1")
		scsihwConfig := optionalStr(req, "scsihw", "virtio-scsi-pci")
		cpuConfig := optionalStr(req, "cpu", "host")
		diskSize := optionalStr(req, "disk_size", "")
		if diskSize != "" && !diskSizeRe.MatchString(diskSize) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid disk_size %q: must be like +10G, 50G, 100M", diskSize)), nil
		}

		startVM := false
		if v, ok := req.GetArguments()["start"].(bool); ok {
			startVM = v
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		if vmid == 0 {
			cluster, err := client.Cluster(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get cluster: %v", err)), nil
			}
			vmid, err = cluster.NextID(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get next VM ID: %v", err)), nil
			}
		}

		importFrom, _, err := downloadImageForImport(ctx, client, node, imageURL, storageName)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		// Create the disk on the user-specified storage by importing from the
		// staged file (size auto-detected via the leading "0").
		scsi0 := fmt.Sprintf("%s:0,import-from=%s", storageName, importFrom)
		vmOpts := []px.VirtualMachineOption{
			{Name: "name", Value: name},
			{Name: "memory", Value: memory},
			{Name: "cores", Value: cores},
			{Name: "cpu", Value: cpuConfig},
			{Name: "net0", Value: netConfig},
			{Name: "scsihw", Value: scsihwConfig},
			{Name: "scsi0", Value: scsi0},
			{Name: "boot", Value: "order=scsi0"},
			{Name: "serial0", Value: "socket"},
			{Name: "agent", Value: agentConfig},
		}

		for _, key := range []string{"bios", "ostype", "vga", "machine", "tags", "description"} {
			if v := optionalStr(req, key, ""); v != "" {
				vmOpts = append(vmOpts, px.VirtualMachineOption{Name: key, Value: v})
			}
		}

		createTask, err := node.NewVirtualMachine(ctx, vmid, vmOpts...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to create VM: %v", err)), nil
		}
		if err := waitForTask(ctx, createTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("VM creation failed: %v", err)), nil
		}

		vm, err := node.VirtualMachine(ctx, vmid)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get VM %d after creation: %v", vmid, err)), nil
		}

		if diskSize != "" {
			resizeTask, err := vm.ResizeDisk(ctx, "scsi0", diskSize)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to resize disk on VM %d: %v", vmid, err)), nil
			}
			if err := waitForTask(ctx, resizeTask); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Disk resize failed: %v", err)), nil
			}
		}

		started := false
		if startVM {
			startTask, err := vm.Start(ctx)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("VM %d created but failed to start: %v", vmid, err)), nil
			}
			if err := waitForTask(ctx, startTask); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("VM %d created but start task failed: %v", vmid, err)), nil
			}
			started = true
		}

		result := struct {
			VMID    int    `json:"vmid"`
			Name    string `json:"name"`
			Node    string `json:"node"`
			Storage string `json:"storage"`
			Started bool   `json:"started"`
			Message string `json:"message"`
		}{
			VMID:    vmid,
			Name:    name,
			Node:    nodeName,
			Storage: storageName,
			Started: started,
			Message: fmt.Sprintf("VM %q created successfully as VM %d", name, vmid),
		}

		return marshalResult(result)
	}
}

func qemuAgentInfoHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type ifaceInfo struct {
			Name            string `json:"name"`
			HardwareAddress string `json:"hardware_address"`
			IPAddresses     []struct {
				Type    string `json:"type"`
				Address string `json:"address"`
				Prefix  int    `json:"prefix"`
			} `json:"ip_addresses,omitempty"`
		}

		type agentInfo struct {
			VMID       int         `json:"vmid"`
			Hostname   string      `json:"hostname,omitempty"`
			OSName     string      `json:"os_name,omitempty"`
			OSVersion  string      `json:"os_version,omitempty"`
			OSPretty   string      `json:"os_pretty_name,omitempty"`
			Kernel     string      `json:"kernel,omitempty"`
			Machine    string      `json:"machine,omitempty"`
			Interfaces []ifaceInfo `json:"interfaces,omitempty"`
		}

		info := agentInfo{VMID: vmid}

		hostname, err := vm.AgentGetHostName(ctx)
		if err == nil {
			info.Hostname = hostname
		}

		osInfo, err := vm.AgentOsInfo(ctx)
		if err == nil && osInfo != nil {
			info.OSName = osInfo.Name
			info.OSVersion = osInfo.Version
			info.OSPretty = osInfo.PrettyName
			info.Kernel = osInfo.KernelRelease
			info.Machine = osInfo.Machine
		}

		ifaces, err := vm.AgentGetNetworkIFaces(ctx)
		if err == nil {
			for _, iface := range ifaces {
				fi := ifaceInfo{
					Name:            iface.Name,
					HardwareAddress: iface.HardwareAddress,
				}
				for _, ip := range iface.IPAddresses {
					fi.IPAddresses = append(fi.IPAddresses, struct {
						Type    string `json:"type"`
						Address string `json:"address"`
						Prefix  int    `json:"prefix"`
					}{
						Type:    ip.IPAddressType,
						Address: ip.IPAddress,
						Prefix:  ip.Prefix,
					})
				}
				info.Interfaces = append(info.Interfaces, fi)
			}
		}

		if info.Hostname == "" && info.OSName == "" && info.Interfaces == nil {
			return mcp.NewToolResultError(fmt.Sprintf("Guest agent on VM %d returned no data; ensure the agent is installed and running", vmid)), nil
		}

		return marshalResult(info)
	}
}
