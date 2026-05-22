package tools

import (
	"context"
	"fmt"
	"path"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerTemplateTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("template_list",
		mcp.WithDescription("List all VM templates across the cluster or on a specific node"),
		mcp.WithString("node", mcp.Description("Filter by node name (optional)")),
	), templateListHandler(client))

	s.AddTool(mcp.NewTool("template_create",
		mcp.WithDescription("Create a new VM template by downloading a disk image (qcow2/raw) from a URL and importing it. The image is downloaded directly by the Proxmox node. Safe for both full and linked clone workflows."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node to create the template on")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Template name (e.g. rocky-9-template)")),
		mcp.WithString("url", mcp.Required(), mcp.Description("URL to a qcow2 or raw disk image")),
		mcp.WithString("storage", mcp.Required(), mcp.Description("Storage pool for the disk (e.g. local-lvm, ceph-pool)")),
		mcp.WithNumber("vmid", mcp.Description("VM ID for the template (optional, auto-assigned if omitted)")),
		mcp.WithNumber("memory", mcp.Description("Memory in MB (default: 2048)")),
		mcp.WithNumber("cores", mcp.Description("CPU cores (default: 2)")),
		mcp.WithString("net", mcp.Description("Network config (default: virtio,bridge=vmbr0)")),
		mcp.WithString("bios", mcp.Description("Firmware type: seabios (default) or ovmf (UEFI)")),
		mcp.WithString("ostype", mcp.Description("Guest OS type: l26 (Linux 2.6+), win11, win10, etc.")),
		mcp.WithString("vga", mcp.Description("VGA type: std, virtio, qxl, serial0, none")),
		mcp.WithString("machine", mcp.Description("Machine type: q35 or i440fx (default: q35 for UEFI)")),
		mcp.WithString("tags", mcp.Description("Semicolon-separated tags (e.g. linux;template;production)")),
		mcp.WithString("description", mcp.Description("Template description")),
		mcp.WithString("agent", mcp.Description("QEMU Guest Agent config (default: enabled=1)")),
		mcp.WithString("scsihw", mcp.Description("SCSI controller type (default: virtio-scsi-pci)")),
	), templateCreateHandler(client))

	s.AddTool(mcp.NewTool("template_update_disk",
		mcp.WithDescription("Replace an existing VM template's disk with a new image downloaded from a URL. Safe for templates used with full clones (existing full clones are independent copies). DESTRUCTIVE for linked clones: linked clones reference the template's base disk, so replacing it will break them. If you use linked clones, create a new template instead (template_create) and migrate clones to the new template."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node the template is on")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Template VM ID")),
		mcp.WithString("url", mcp.Required(), mcp.Description("URL to a qcow2 or raw disk image")),
		mcp.WithString("storage", mcp.Required(), mcp.Description("Storage pool for the new disk")),
	), templateUpdateDiskHandler(client))

	s.AddTool(mcp.NewTool("template_delete",
		mcp.WithDescription("Delete a VM template. DESTRUCTIVE: the template and its disks are permanently removed."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node the template is on")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Template VM ID to delete")),
	), templateDeleteHandler(client))

	s.AddTool(mcp.NewTool("template_clone",
		mcp.WithDescription("Clone a new VM from a template. Supports full clones (independent copy) and linked clones (thin-provisioned, shares base disk). Linked clones are faster and use less storage but depend on the template — do not delete the template while linked clones exist."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node the template is on")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Template VM ID to clone from")),
		mcp.WithString("name", mcp.Required(), mcp.Description("Name for the new VM")),
		mcp.WithBoolean("full", mcp.Description("true for full clone (independent copy), false for linked clone (default: true)")),
		mcp.WithNumber("newid", mcp.Description("VM ID for the clone (optional, auto-assigned if omitted)")),
		mcp.WithString("storage", mcp.Description("Target storage for full clone disks (optional, uses template's storage if omitted)")),
		mcp.WithString("target", mcp.Description("Target node for the clone (optional, same node if omitted)")),
		mcp.WithString("pool", mcp.Description("Resource pool to add the clone to (optional)")),
		mcp.WithString("format", mcp.Description("Disk format for full clone: qcow2, raw, vmdk (optional)")),
		mcp.WithString("snapname", mcp.Description("Clone from this snapshot instead of current state (optional)")),
	), templateCloneHandler(client))

	s.AddTool(mcp.NewTool("template_config_set",
		mcp.WithDescription("Update configuration on an existing VM template. Accepts any valid QEMU VM configuration key-value pairs. Common keys: bios (seabios|ovmf), ostype (l26|win11|...), vga (std|virtio|qxl|none), machine (q35|i440fx), tags, description, onboot (0|1), agent (enabled=1), protection (0|1), scsihw, boot, cpu, balloon."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node the template is on")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("Template VM ID")),
		mcp.WithObject("config", mcp.Required(), mcp.Description("Key-value pairs of configuration to set (e.g. {\"bios\": \"ovmf\", \"tags\": \"linux;production\", \"ostype\": \"l26\"})")),
	), templateConfigSetHandler(client))
}

func templateListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := optionalNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		type tmplSummary struct {
			VMID   int    `json:"vmid"`
			Name   string `json:"name"`
			Node   string `json:"node"`
			CPUs   int    `json:"cpus"`
			MaxMem uint64 `json:"maxmem"`
			Status string `json:"status"`
		}

		items := make([]tmplSummary, 0)

		collectTemplates := func(node *px.Node) error {
			vms, err := node.VirtualMachines(ctx)
			if err != nil {
				return err
			}
			for _, vm := range vms {
				if vm.Template {
					items = append(items, tmplSummary{
						VMID:   int(vm.VMID),
						Name:   vm.Name,
						Node:   vm.Node,
						CPUs:   vm.CPUs,
						MaxMem: vm.MaxMem,
						Status: vm.Status,
					})
				}
			}
			return nil
		}

		if nodeName != "" {
			node, err := client.Node(ctx, nodeName)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
			}
			if err := collectTemplates(node); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to list VMs on %q: %v", nodeName, err)), nil
			}
			return marshalResult(items)
		}

		nodeErrs, err := forEachNode(ctx, client, collectTemplates)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		if len(nodeErrs) > 0 {
			return marshalResult(listResult[tmplSummary]{Items: items, Errors: nodeErrs})
		}
		return marshalResult(items)
	}
}

func templateCreateHandler(client *px.Client) server.ToolHandlerFunc {
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

		filename := filenameFromURL(imageURL)
		storage, err := node.Storage(ctx, storageName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get storage %q: %v", storageName, err)), nil
		}

		dlTask, err := storage.DownloadURL(ctx, "import", filename, imageURL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to start image download: %v", err)), nil
		}
		if err := waitForTask(ctx, dlTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Image download failed: %v", err)), nil
		}

		importVolume := fmt.Sprintf("%s:import/%s", storageName, filename)
		vmOpts := []px.VirtualMachineOption{
			{Name: "name", Value: name},
			{Name: "memory", Value: memory},
			{Name: "cores", Value: cores},
			{Name: "net0", Value: netConfig},
			{Name: "scsihw", Value: scsihwConfig},
			{Name: "scsi0", Value: importVolume},
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

		// Step 3: Convert to template
		vm, err := node.VirtualMachine(ctx, vmid)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get VM %d after creation: %v", vmid, err)), nil
		}

		tmplTask, err := vm.ConvertToTemplate(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to convert to template: %v", err)), nil
		}
		if err := waitForTask(ctx, tmplTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Template conversion failed: %v", err)), nil
		}

		result := struct {
			VMID    int    `json:"vmid"`
			Name    string `json:"name"`
			Node    string `json:"node"`
			Storage string `json:"storage"`
			Message string `json:"message"`
		}{
			VMID:    vmid,
			Name:    name,
			Node:    nodeName,
			Storage: storageName,
			Message: fmt.Sprintf("Template %q created successfully as VM %d", name, vmid),
		}

		return marshalResult(result)
	}
}

func templateUpdateDiskHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, nodeName, vmid, err := withVM(client, ctx, req)
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

		if !vm.Template {
			return mcp.NewToolResultError(fmt.Sprintf("VM %d is not a template", vmid)), nil
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		// Step 1: Download the new image
		filename := filenameFromURL(imageURL)
		storage, err := node.Storage(ctx, storageName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get storage %q: %v", storageName, err)), nil
		}

		dlTask, err := storage.DownloadURL(ctx, "import", filename, imageURL)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to start image download: %v", err)), nil
		}
		if err := waitForTask(ctx, dlTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Image download failed: %v", err)), nil
		}

		// Step 2: Detach old disk
		if vm.VirtualMachineConfig != nil && vm.VirtualMachineConfig.SCSI0 != "" {
			unlinkTask, err := vm.UnlinkDisk(ctx, "scsi0", true)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Failed to detach old disk: %v", err)), nil
			}
			if err := waitForTask(ctx, unlinkTask); err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("Disk detach failed: %v", err)), nil
			}
		}

		// Step 3: Attach new disk
		importVolume := fmt.Sprintf("%s:import/%s", storageName, filename)
		configTask, err := vm.Config(ctx, px.VirtualMachineOption{
			Name:  "scsi0",
			Value: importVolume,
		})
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to attach new disk: %v", err)), nil
		}
		if err := waitForTask(ctx, configTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Disk attach failed: %v", err)), nil
		}

		// Step 4: Re-convert to template
		tmplTask, err := vm.ConvertToTemplate(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to re-template: %v", err)), nil
		}
		if err := waitForTask(ctx, tmplTask); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Template conversion failed: %v", err)), nil
		}

		result := struct {
			VMID    int    `json:"vmid"`
			Name    string `json:"name"`
			Node    string `json:"node"`
			Message string `json:"message"`
		}{
			VMID:    vmid,
			Name:    vm.Name,
			Node:    nodeName,
			Message: fmt.Sprintf("Template %d disk updated from %s", vmid, imageURL),
		}

		return marshalResult(result)
	}
}

func templateDeleteHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if !vm.Template {
			return mcp.NewToolResultError(fmt.Sprintf("VM %d is not a template", vmid)), nil
		}

		task, err := vm.Delete(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to delete template %d: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Template deletion failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully deleted template %d (%s)", vmid, vm.Name)), nil
	}
}

func templateCloneHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, nodeName, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		name, err := req.RequireString("name")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if !vm.Template {
			return mcp.NewToolResultError(fmt.Sprintf("VM %d is not a template", vmid)), nil
		}

		storageOpt := optionalStr(req, "storage", "")
		if storageOpt != "" && !storageNameRe.MatchString(storageOpt) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid storage name %q", storageOpt)), nil
		}
		targetOpt := optionalStr(req, "target", "")
		if targetOpt != "" && !nodeNameRe.MatchString(targetOpt) {
			return mcp.NewToolResultError(fmt.Sprintf("invalid target node name %q", targetOpt)), nil
		}

		opts := &px.VirtualMachineCloneOptions{
			Name:     name,
			Full:     1,
			Storage:  storageOpt,
			Target:   targetOpt,
			Pool:     optionalStr(req, "pool", ""),
			Format:   optionalStr(req, "format", ""),
			SnapName: optionalStr(req, "snapname", ""),
		}

		if v, ok := req.GetArguments()["full"].(bool); ok && !v {
			opts.Full = 0
		}

		newid, _ := optionalInt(req, "newid")
		if newid > 0 {
			opts.NewID = newid
		}

		clonedID, task, err := vm.Clone(ctx, opts)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to clone template %d: %v", vmid, err)), nil
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
			Name       string `json:"name"`
			SourceVMID int    `json:"source_vmid"`
			Node       string `json:"node"`
			CloneType  string `json:"clone_type"`
			Message    string `json:"message"`
		}{
			VMID:       clonedID,
			Name:       name,
			SourceVMID: vmid,
			Node:       targetNode,
			CloneType:  cloneType,
			Message:    fmt.Sprintf("Successfully cloned template %d to VM %d (%s clone)", vmid, clonedID, cloneType),
		}

		return marshalResult(result)
	}
}

var allowedConfigKeys = map[string]bool{
	"acpi": true, "agent": true, "balloon": true, "bios": true,
	"boot": true, "cores": true, "cpu": true, "cpulimit": true,
	"cpuunits": true, "description": true, "hotplug": true,
	"hugepages": true, "kvm": true, "machine": true, "memory": true,
	"name": true, "numa": true, "onboot": true, "ostype": true,
	"protection": true, "scsihw": true, "sockets": true, "startup": true,
	"tablet": true, "tags": true, "vcpus": true, "vga": true,
}

func templateConfigSetHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		if !vm.Template {
			return mcp.NewToolResultError(fmt.Sprintf("VM %d is not a template", vmid)), nil
		}

		args := req.GetArguments()
		configMap, ok := args["config"].(map[string]interface{})
		if !ok {
			return mcp.NewToolResultError("parameter \"config\" must be an object"), nil
		}
		if len(configMap) == 0 {
			return mcp.NewToolResultError("config must contain at least one key-value pair"), nil
		}

		opts := make([]px.VirtualMachineOption, 0, len(configMap))
		for k, v := range configMap {
			if !allowedConfigKeys[k] {
				return mcp.NewToolResultError(fmt.Sprintf("config key %q is not allowed; allowed keys: acpi, agent, balloon, bios, boot, cores, cpu, cpulimit, cpuunits, description, hotplug, hugepages, kvm, machine, memory, name, numa, onboot, ostype, protection, scsihw, sockets, startup, tablet, tags, vcpus, vga", k)), nil
			}
			opts = append(opts, px.VirtualMachineOption{Name: k, Value: v})
		}

		task, err := vm.Config(ctx, opts...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to update template %d config: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Config update failed: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf("Successfully updated %d config key(s) on template %d", len(configMap), vmid)), nil
	}
}

func filenameFromURL(u string) string {
	p := path.Base(u)
	if idx := strings.IndexByte(p, '?'); idx != -1 {
		p = p[:idx]
	}
	if idx := strings.IndexByte(p, '#'); idx != -1 {
		p = p[:idx]
	}
	if p == "" || p == "." || p == "/" || !safeFilenameRe.MatchString(p) {
		return "disk-image.qcow2"
	}
	return p
}
