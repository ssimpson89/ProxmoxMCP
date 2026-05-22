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

		args := req.GetArguments()
		netConfig := "virtio,bridge=vmbr0"
		if v, ok := args["net"].(string); ok && v != "" {
			netConfig = v
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

		// Step 1: Download the image to Proxmox storage
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

		// Step 2: Create the VM
		importVolume := fmt.Sprintf("%s:import/%s", storageName, filename)
		createTask, err := node.NewVirtualMachine(ctx, vmid,
			px.VirtualMachineOption{Name: "name", Value: name},
			px.VirtualMachineOption{Name: "memory", Value: memory},
			px.VirtualMachineOption{Name: "cores", Value: cores},
			px.VirtualMachineOption{Name: "net0", Value: netConfig},
			px.VirtualMachineOption{Name: "scsihw", Value: "virtio-scsi-pci"},
			px.VirtualMachineOption{Name: "scsi0", Value: importVolume},
			px.VirtualMachineOption{Name: "boot", Value: "order=scsi0"},
			px.VirtualMachineOption{Name: "serial0", Value: "socket"},
			px.VirtualMachineOption{Name: "agent", Value: "enabled=1"},
		)
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

func optionalInt(req mcp.CallToolRequest, name string) (int, error) {
	args := req.GetArguments()
	val, ok := args[name]
	if !ok {
		return 0, nil
	}
	switch v := val.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	default:
		return 0, fmt.Errorf("parameter %q must be a number", name)
	}
}

func optionalIntDefault(req mcp.CallToolRequest, name string, defaultVal int) int {
	v, _ := optionalInt(req, name)
	if v == 0 {
		return defaultVal
	}
	return v
}

func filenameFromURL(u string) string {
	p := path.Base(u)
	if idx := strings.IndexByte(p, '?'); idx != -1 {
		p = p[:idx]
	}
	if p == "" || p == "." || p == "/" {
		return "disk-image.qcow2"
	}
	return p
}
