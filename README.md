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
| `cluster_status` | Cluster health, quorum, and node membership |

### Nodes
| Tool | Description |
|---|---|
| `node_list` | List all nodes with CPU, memory, uptime |
| `node_status` | Detailed node info (kernel, PVE version, load average) |

### QEMU Virtual Machines
| Tool | Description |
|---|---|
| `qemu_list` | List VMs (cluster-wide or per node) |
| `qemu_status` | Detailed VM config and runtime metrics |
| `qemu_start` | Start a stopped VM |
| `qemu_stop` | Force stop a VM |
| `qemu_shutdown` | ACPI graceful shutdown |
| `qemu_reboot` | Reboot a running VM |
| `qemu_reset` | Hard reset (may cause data loss) |
| `qemu_suspend` | Suspend to RAM |
| `qemu_resume` | Resume from suspend |
| `qemu_snapshot_list` | List VM snapshots |
| `qemu_snapshot_create` | Create a snapshot |
| `qemu_snapshot_rollback` | Rollback to snapshot (destructive) |
| `qemu_exec` | Execute command via QEMU Guest Agent |

### LXC Containers
| Tool | Description |
|---|---|
| `lxc_list` | List containers (cluster-wide or per node) |
| `lxc_status` | Detailed container config and metrics |
| `lxc_start` | Start a stopped container |
| `lxc_stop` | Force stop a container |
| `lxc_shutdown` | Graceful shutdown |
| `lxc_reboot` | Reboot a running container |
| `lxc_snapshot_list` | List container snapshots |
| `lxc_snapshot_create` | Create a snapshot |

### Templates
| Tool | Description |
|---|---|
| `template_list` | List VM templates (cluster-wide or per node) |
| `template_create` | Create a template from a qcow2/raw image URL |
| `template_update_disk` | Replace a template's disk with a new image (safe for full clones, breaks linked clones) |
| `template_delete` | Delete a VM template |

### Storage
| Tool | Description |
|---|---|
| `storage_list` | List storage pools with capacity info |
| `storage_content` | List content of a storage pool |

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
