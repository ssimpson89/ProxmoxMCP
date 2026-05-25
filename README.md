# ProxmoxMCP

An open-source MCP (Model Context Protocol) server for Proxmox VE, written in Go. Ships as a single binary with zero runtime dependencies.

## Features

- 30 MCP tools covering cluster, node, VM (QEMU), container (LXC), storage, and template management
- Start, stop, reboot, snapshot, and execute commands in VMs via QEMU Guest Agent
- LXC container lifecycle management with snapshot support
- Single binary, no Python/Node.js runtime required
- stdio transport for Claude Code / Claude Desktop

## Quick Start

### Install

```bash
go install github.com/ssimpson/ProxmoxMCP@latest
```

Or download a prebuilt binary from [Releases](https://github.com/ssimpson/ProxmoxMCP/releases).

### Setup Wizard

The fastest way to configure ProxmoxMCP is the interactive setup wizard:

```bash
# If installed via go install:
go run github.com/ssimpson/ProxmoxMCP/cmd/setup@latest

# Or from source:
go run ./cmd/setup

# Or use the prebuilt binary:
proxmox-mcp-setup
```

The wizard walks you through creating a Proxmox API token, entering your credentials, and writes the config directly to your Claude Code or Claude Desktop settings file.

### Manual Configuration

If you prefer to configure manually:

1. **Create a Proxmox API Token** in the Proxmox web UI: Datacenter > Permissions > API Tokens > Add. Recommended: create a dedicated user with limited permissions rather than using root.

2. **Add to your MCP client configuration:**

**Claude Code** (`~/.claude/settings.json`):

```json
{
  "mcpServers": {
    "proxmox": {
      "command": "proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "https://your-proxmox-host:8006",
        "PROXMOX_TOKEN_ID": "user@pam!token-name",
        "PROXMOX_TOKEN_SECRET": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      }
    }
  }
}
```

**Claude Desktop** (`claude_desktop_config.json`):

```json
{
  "mcpServers": {
    "proxmox": {
      "command": "/path/to/proxmox-mcp",
      "env": {
        "PROXMOX_HOST": "https://your-proxmox-host:8006",
        "PROXMOX_TOKEN_ID": "user@pam!token-name",
        "PROXMOX_TOKEN_SECRET": "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"
      }
    }
  }
}
```

For self-signed certificates, add `"PROXMOX_VERIFY_TLS": "false"` to the env block.

## Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `PROXMOX_HOST` | Yes | | Proxmox API URL (e.g., `https://pve.example.com:8006`) |
| `PROXMOX_TOKEN_ID` | * | | API token ID (e.g., `user@pam!token-name`) |
| `PROXMOX_TOKEN_SECRET` | * | | API token secret UUID |
| `PROXMOX_USER` | * | | Username for password auth (e.g., `root@pam`) |
| `PROXMOX_PASSWORD` | * | | Password for password auth |
| `PROXMOX_VERIFY_TLS` | No | `true` | Verify TLS certificates (set to `false` for self-signed certs) |
| `PROXMOX_ALLOW_EXEC` | No | `false` | Enable `qemu_exec` tool for running commands inside VMs via Guest Agent |

\* Provide either `PROXMOX_TOKEN_ID` + `PROXMOX_TOKEN_SECRET` (recommended) or `PROXMOX_USER` + `PROXMOX_PASSWORD`. API tokens are preferred because they can be scoped to specific permissions and don't expire with password changes.

All credential variables support a `_FILE` suffix (e.g., `PROXMOX_TOKEN_SECRET_FILE=/path/to/secret`). The `_FILE` variant reads the credential from a file at that path, which keeps secrets out of the MCP config JSON. This also works with Docker secrets and Kubernetes secret volumes.

## Available Tools

### Cluster
| Tool | Description |
|---|---|
| `cluster_status` | Get cluster health, quorum status, and node membership |
| `cluster_resources` | List all resources across the cluster (VMs, containers, storage, nodes). Provides a unified view of resource usage. |
| `cluster_next_id` | Get the next available VM/container ID in the cluster. Useful for planning new deployments. |

### Nodes
| Tool | Description |
|---|---|
| `node_list` | List all nodes in the cluster with status summary (CPU, memory, uptime) |
| `node_status` | Get detailed status for a specific node including CPU, memory, kernel version, and PVE version |
| `node_backup` | Create a backup (vzdump) of a VM or container. The backup is stored on the specified storage pool. |

### QEMU Virtual Machines
| Tool | Description |
|---|---|
| `qemu_list` | List all QEMU virtual machines across the cluster, or on a specific node |
| `qemu_create_from_url` | Create a new VM (not a template) by downloading a disk image (qcow2/raw) from a URL and importing it. The image is downloaded directly by the Proxmox node. Use this when you want a regular VM from an image URL; use template_create instead if you want a reusable template. |
| `qemu_status` | Get detailed VM configuration and runtime status |
| `qemu_start` | Start a stopped virtual machine |
| `qemu_stop` | Force stop a virtual machine. WARNING: This immediately stops the VM and may cause data loss. Prefer qemu_shutdown for graceful shutdown. |
| `qemu_shutdown` | Send ACPI shutdown signal to a virtual machine for graceful shutdown |
| `qemu_reboot` | Reboot a running virtual machine |
| `qemu_reset` | Hard reset a virtual machine (like pressing the reset button). DESTRUCTIVE: may cause data loss in the guest OS. |
| `qemu_suspend` | Suspend a virtual machine to RAM (pause CPU execution) |
| `qemu_resume` | Resume a suspended virtual machine |
| `qemu_snapshot_list` | List all snapshots for a virtual machine |
| `qemu_snapshot_create` | Create a new snapshot of a virtual machine |
| `qemu_snapshot_rollback` | Rollback a virtual machine to a named snapshot. DESTRUCTIVE: current state will be lost. |
| `qemu_snapshot_delete` | Delete a snapshot from a virtual machine. DESTRUCTIVE: the snapshot data is permanently removed. |
| `qemu_resize_disk` | Resize a VM disk. Can only increase size, not shrink. Size format: +10G (add 10GB), 50G (set to 50GB). |
| `qemu_migrate` | Migrate a VM to a different node. Supports online (live) migration and offline migration. |
| `qemu_agent_info` | Get guest OS information, hostname, and network interfaces from a running VM via the QEMU Guest Agent. Requires the guest agent to be installed and running. |
| `qemu_exec` | Execute a command inside a VM via the QEMU Guest Agent. Requires the guest agent to be installed and running. Disabled by default; set PROXMOX_ALLOW_EXEC=true to enable. |

### LXC Containers
| Tool | Description |
|---|---|
| `lxc_list` | List all LXC containers across the cluster, or on a specific node |
| `lxc_status` | Get detailed LXC container configuration and runtime status |
| `lxc_start` | Start a stopped LXC container |
| `lxc_stop` | Force stop an LXC container. WARNING: may cause data loss. Prefer lxc_shutdown for graceful stop. |
| `lxc_shutdown` | Gracefully shut down an LXC container |
| `lxc_reboot` | Reboot a running LXC container |
| `lxc_snapshot_list` | List all snapshots for an LXC container |
| `lxc_snapshot_create` | Create a new snapshot of an LXC container |
| `lxc_snapshot_rollback` | Rollback an LXC container to a named snapshot. DESTRUCTIVE: current state will be lost. |
| `lxc_snapshot_delete` | Delete a snapshot from an LXC container. DESTRUCTIVE: the snapshot data is permanently removed. |
| `lxc_resize` | Resize an LXC container disk/filesystem. Can only increase size, not shrink. |
| `lxc_migrate` | Migrate an LXC container to a different node. |
| `lxc_template_create` | Convert an existing LXC container into a template. The container must be stopped. Once converted, the container cannot be started — it can only be cloned. |
| `lxc_clone` | Clone an LXC container or container template. Supports full clones (independent copy) and linked clones (shares base image). Linked clones are faster but depend on the source. |

### Templates
| Tool | Description |
|---|---|
| `template_list` | List all VM templates across the cluster or on a specific node |
| `template_create` | Create a new VM template by downloading a disk image (qcow2/raw) from a URL and importing it. The image is downloaded directly by the Proxmox node. Safe for both full and linked clone workflows. |
| `template_update_disk` | Replace an existing VM template's disk with a new image downloaded from a URL. Safe for templates used with full clones (existing full clones are independent copies). DESTRUCTIVE for linked clones: linked clones reference the template's base disk, so replacing it will break them. If you use linked clones, create a new template instead (template_create) and migrate clones to the new template. |
| `template_delete` | Delete a VM template. DESTRUCTIVE: the template and its disks are permanently removed. |
| `template_clone` | Clone a new VM from a template. Supports full clones (independent copy) and linked clones (thin-provisioned, shares base disk). Linked clones are faster and use less storage but depend on the template — do not delete the template while linked clones exist. |
| `template_config_set` | Update configuration on an existing VM template. Accepts any valid QEMU VM configuration key-value pairs. Common keys: bios (seabios|ovmf), ostype (l26|win11|...), vga (std|virtio|qxl|none), machine (q35|i440fx), tags, description, onboot (0|1), agent (enabled=1), protection (0|1), scsihw, boot, cpu, balloon. |

### Storage
| Tool | Description |
|---|---|
| `storage_list` | List storage pools, optionally filtered by node. Shows capacity, usage, type, and content types. |
| `storage_content` | List the content (ISOs, disk images, backups, templates) of a storage pool on a node |

### Cloud-Init
| Tool | Description |
|---|---|
| `cloudinit_set` | Set cloud-init configuration on a VM or template. Cloud-init parameters are applied on first boot of a cloned VM, providing unique identity (hostname, SSH keys, network config). A cloud-init drive must be attached to the VM (use cloudinit_drive parameter or add one with template_config_set: {\"ide2\": \"local-lvm:cloudinit\"}). |
| `cloudinit_get` | Read cloud-init configuration from a VM or template. Returns current cloud-init parameters (user, IP config, nameserver, etc.). SSH keys and passwords are redacted. |

### Appliances
| Tool | Description |
|---|---|
| `appliance_list` | List available appliance templates from the Proxmox catalog. These are pre-built LXC container templates (TurnKey Linux, system images, etc.) that can be downloaded to storage and used to create containers. |
| `appliance_download` | Download an appliance template from the Proxmox catalog to local storage. Use appliance_list to find available templates. The template name is the 'template' field from appliance_list (e.g. 'ubuntu-24.04-standard_24.04-2_amd64.tar.zst'). |

## Building from Source

```bash
git clone https://github.com/ssimpson/ProxmoxMCP.git
cd ProxmoxMCP
go build -o proxmox-mcp .
go build -o proxmox-mcp-setup ./cmd/setup
```

## Security

The MCP protocol uses environment variables for STDIO server credentials. Claude can read its own config files, so credentials stored as inline env vars in `settings.json` or `claude_desktop_config.json` are visible in conversation. The setup wizard offers three storage methods to address this:

**Credential files (recommended):** Secrets are written to `~/.config/proxmox-mcp/token-id` and `token-secret` (mode 0600). The MCP config JSON only contains `PROXMOX_TOKEN_ID_FILE` and `PROXMOX_TOKEN_SECRET_FILE` paths. Claude sees file paths, not secrets.

```json
{
  "env": {
    "PROXMOX_HOST": "https://pve.example.com:8006",
    "PROXMOX_TOKEN_ID_FILE": "/home/user/.config/proxmox-mcp/token-id",
    "PROXMOX_TOKEN_SECRET_FILE": "/home/user/.config/proxmox-mcp/token-secret"
  }
}
```

**Wrapper script:** Credentials are stored in `~/.config/proxmox-mcp/run.sh` (mode 0700), and the MCP config JSON only references the script path.

**Inline env vars:** Credentials stored directly in the MCP config JSON. Standard MCP pattern, but Claude can read them.

The `_FILE` pattern also works with Docker secrets (`/run/secrets/proxmox-token`), Kubernetes secret volumes, and any secret manager that writes files.

Regardless of method:
- Use a dedicated Proxmox user with the minimum required permissions
- Avoid using root API tokens
- Rotate API tokens periodically
- The setup wizard enforces 0600 permissions on all files it writes

## Releasing

Releases are automated via GoReleaser. Push a tag to trigger a build:

```bash
git tag v0.1.0
git push origin v0.1.0
```

This builds cross-platform binaries (linux/darwin/windows, amd64/arm64) and creates a GitHub release.

## License

MIT
