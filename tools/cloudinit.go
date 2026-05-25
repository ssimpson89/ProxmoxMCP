package tools

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerCloudInitTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("cloudinit_set",
		mcp.WithDescription("Set cloud-init configuration on a VM or template. Cloud-init parameters are applied on first boot of a cloned VM, providing unique identity (hostname, SSH keys, network config). A cloud-init drive must be attached to the VM (use cloudinit_drive parameter or add one with template_config_set: {\"ide2\": \"local-lvm:cloudinit\"})."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM or template ID")),
		mcp.WithString("ciuser", mcp.Description("Default user name")),
		mcp.WithString("cipassword", mcp.Description("Password for the default user (prefer SSH keys instead)")),
		mcp.WithString("sshkeys", mcp.Description("Newline-separated SSH public keys (will be URL-encoded automatically)")),
		mcp.WithString("ipconfig0", mcp.Description("IP config for first interface: ip=dhcp or ip=CIDR,gw=IP (e.g. ip=10.0.0.100/24,gw=10.0.0.1)")),
		mcp.WithString("ipconfig1", mcp.Description("IP config for second interface (same format as ipconfig0)")),
		mcp.WithString("ipconfig2", mcp.Description("IP config for third interface (same format as ipconfig0)")),
		mcp.WithString("nameserver", mcp.Description("DNS server IP(s), space-separated")),
		mcp.WithString("searchdomain", mcp.Description("DNS search domain(s)")),
		mcp.WithString("citype", mcp.Description("Cloud-init type: nocloud (Linux default), configdrive2 (older), opennebula")),
		mcp.WithBoolean("ciupgrade", mcp.Description("Auto-upgrade packages on first boot (default: false — first boot is faster and avoids surprise package changes; pass true to force apt/dnf upgrade)")),
		mcp.WithString("cicustom", mcp.Description("Custom cloud-init config files: user=STORAGE:snippets/file.yaml,network=...,meta=...")),
		mcp.WithString("cloudinit_drive", mcp.Description("Attach a cloud-init drive if not already present. Specify as DEVICE=STORAGE (e.g. ide2=local-lvm). Only needed if the VM doesn't already have one.")),
	), cloudinitSetHandler(client))

	s.AddTool(mcp.NewTool("cloudinit_get",
		mcp.WithDescription("Read cloud-init configuration from a VM or template. Returns current cloud-init parameters (user, IP config, nameserver, etc.). SSH keys and passwords are redacted."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node name")),
		mcp.WithNumber("vmid", mcp.Required(), mcp.Description("VM or template ID")),
	), cloudinitGetHandler(client))
}

func cloudinitSetHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		args := req.GetArguments()
		var opts []px.VirtualMachineOption

		if v, ok := args["cloudinit_drive"].(string); ok && v != "" {
			parts := strings.SplitN(v, "=", 2)
			if len(parts) != 2 {
				return mcp.NewToolResultError("cloudinit_drive must be in DEVICE=STORAGE format (e.g. ide2=local-lvm)"), nil
			}
			if !ciDriveRe.MatchString(parts[0]) {
				return mcp.NewToolResultError("cloudinit_drive device must be ideN, sataN, or scsiN (e.g. ide2)"), nil
			}
			opts = append(opts, px.VirtualMachineOption{
				Name:  parts[0],
				Value: parts[1] + ":cloudinit",
			})
		}

		stringParams := []string{"ciuser", "cipassword", "nameserver", "searchdomain", "citype", "cicustom",
			"ipconfig0", "ipconfig1", "ipconfig2"}
		for _, key := range stringParams {
			if v, ok := args[key].(string); ok && v != "" {
				opts = append(opts, px.VirtualMachineOption{Name: key, Value: v})
			}
		}

		if v, ok := args["sshkeys"].(string); ok && v != "" {
			opts = append(opts, px.VirtualMachineOption{
				Name:  "sshkeys",
				Value: url.QueryEscape(v),
			})
		}

		if len(opts) == 0 {
			return mcp.NewToolResultError("at least one cloud-init parameter must be provided"), nil
		}

		// Default ciupgrade to 0 (don't auto-upgrade on first boot) unless the
		// caller explicitly sets it. Proxmox's own default is 1, so we always
		// send the option to make our default the effective one.
		ciupgrade := 0
		if v, ok := args["ciupgrade"].(bool); ok && v {
			ciupgrade = 1
		}
		opts = append(opts, px.VirtualMachineOption{Name: "ciupgrade", Value: ciupgrade})

		task, err := vm.Config(ctx, opts...)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to set cloud-init config on VM %d: %v", vmid, err)), nil
		}
		if err := waitForTask(ctx, task); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Cloud-init config update failed: %v", err)), nil
		}

		keys := make([]string, 0, len(opts))
		for _, o := range opts {
			keys = append(keys, o.Name)
		}
		return mcp.NewToolResultText(fmt.Sprintf("Successfully set cloud-init config on VM %d: %s", vmid, strings.Join(keys, ", "))), nil
	}
}

func cloudinitGetHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		vm, _, vmid, err := withVM(client, ctx, req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		cfg := vm.VirtualMachineConfig
		if cfg == nil {
			return mcp.NewToolResultError(fmt.Sprintf("VM %d has no configuration", vmid)), nil
		}

		type cloudinitInfo struct {
			VMID         int    `json:"vmid"`
			CIUser       string `json:"ciuser,omitempty"`
			CIType       string `json:"citype,omitempty"`
			CIUpgrade    int    `json:"ciupgrade,omitempty"`
			CICustom     string `json:"cicustom,omitempty"`
			Nameserver   string `json:"nameserver,omitempty"`
			Searchdomain string `json:"searchdomain,omitempty"`
			IPConfig0    string `json:"ipconfig0,omitempty"`
			IPConfig1    string `json:"ipconfig1,omitempty"`
			IPConfig2    string `json:"ipconfig2,omitempty"`
			IPConfig3    string `json:"ipconfig3,omitempty"`
			IPConfig4    string `json:"ipconfig4,omitempty"`
			IPConfig5    string `json:"ipconfig5,omitempty"`
			HasSSHKeys   bool   `json:"has_sshkeys"`
			HasPassword  bool   `json:"has_cipassword"`
		}

		info := cloudinitInfo{
			VMID:         vmid,
			CIUser:       cfg.CIUser,
			CIType:       cfg.CIType,
			CIUpgrade:    cfg.CIUpgrade,
			CICustom:     cfg.CICustom,
			Nameserver:   cfg.Nameserver,
			Searchdomain: cfg.Searchdomain,
			IPConfig0:    cfg.IPConfig0,
			IPConfig1:    cfg.IPConfig1,
			IPConfig2:    cfg.IPConfig2,
			IPConfig3:    cfg.IPConfig3,
			IPConfig4:    cfg.IPConfig4,
			IPConfig5:    cfg.IPConfig5,
			HasSSHKeys:   cfg.SSHKeys != "",
			HasPassword:  cfg.CIPassword != "",
		}

		return marshalResult(info)
	}
}
