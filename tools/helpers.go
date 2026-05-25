package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

const (
	taskTimeout       = 5 * time.Minute
	backupTaskTimeout = 2 * time.Hour
)

var (
	nodeNameRe     = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,62}$`)
	snapNameRe     = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,39}$`)
	storageNameRe  = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)
	ciDriveRe      = regexp.MustCompile(`^(ide|sata|scsi)\d+$`)
	safeFilenameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
	qemuDiskRe     = regexp.MustCompile(`^(scsi|virtio|ide|sata|efidisk)\d+$`)
	lxcDiskRe      = regexp.MustCompile(`^(rootfs|mp\d+)$`)
	diskSizeRe     = regexp.MustCompile(`^\+?\d+[TGMK]?$`)

	sensitiveConfigKeys = map[string]bool{
		"cipassword": true,
		"sshkeys":    true,
		"args":       true,
		"cicustom":   true,
	}
)

func requiredNode(req mcp.CallToolRequest) (string, error) {
	s, err := req.RequireString("node")
	if err != nil {
		return "", err
	}
	if !nodeNameRe.MatchString(s) {
		return "", fmt.Errorf("invalid node name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", s)
	}
	return s, nil
}

func optionalNode(req mcp.CallToolRequest) (string, error) {
	args := req.GetArguments()
	val, ok := args["node"]
	if !ok {
		return "", nil
	}
	s, ok := val.(string)
	if !ok {
		return "", fmt.Errorf("parameter \"node\" must be a string")
	}
	if s == "" {
		return "", nil
	}
	if !nodeNameRe.MatchString(s) {
		return "", fmt.Errorf("invalid node name %q: must match [a-zA-Z0-9][a-zA-Z0-9_-]*", s)
	}
	return s, nil
}

func requiredVMID(req mcp.CallToolRequest) (int, error) {
	args := req.GetArguments()
	val, ok := args["vmid"]
	if !ok {
		return 0, fmt.Errorf("missing required parameter: vmid")
	}
	var f float64
	switch v := val.(type) {
	case float64:
		f = v
	case int:
		f = float64(v)
	default:
		return 0, fmt.Errorf("parameter \"vmid\" must be a number")
	}
	if f != math.Trunc(f) || f < 100 || f > 999999999 {
		return 0, fmt.Errorf("vmid must be an integer between 100 and 999999999, got %v", val)
	}
	return int(f), nil
}

func requiredSnapName(req mcp.CallToolRequest) (string, error) {
	s, err := req.RequireString("name")
	if err != nil {
		return "", err
	}
	if !snapNameRe.MatchString(s) {
		return "", fmt.Errorf("invalid snapshot name %q: must start with a letter, contain only [a-zA-Z0-9_-], max 40 chars", s)
	}
	return s, nil
}

func requiredStorageName(req mcp.CallToolRequest) (string, error) {
	s, err := req.RequireString("storage")
	if err != nil {
		return "", err
	}
	if !storageNameRe.MatchString(s) {
		return "", fmt.Errorf("invalid storage name %q: must start with a letter, contain only [a-zA-Z0-9_.-]", s)
	}
	return s, nil
}

// fileBasedStorageTypes lists Proxmox storage backends that support the
// "import" content type (i.e. can hold a downloaded qcow2/raw disk image
// file). Block-based backends (lvm, lvmthin, zfspool, rbd, iscsi) cannot.
var fileBasedStorageTypes = map[string]bool{
	"dir":       true,
	"nfs":       true,
	"cifs":      true,
	"cephfs":    true,
	"glusterfs": true,
}

func storageContentSet(s *px.Storage) map[string]bool {
	out := make(map[string]bool)
	for _, c := range strings.Split(s.Content, ",") {
		c = strings.TrimSpace(c)
		if c != "" {
			out[c] = true
		}
	}
	return out
}

// pickStagingStorage finds a node-local storage that can hold a downloaded
// disk image (qcow2/raw) for import. Proxmox requires the "import" content
// type for this, and validates file extensions (so the "iso" content type
// will reject .qcow2 downloads). If no eligible storage exists yet, this
// auto-enables "import" on a file-based storage via the cluster storage API
// — the equivalent of `pvesm set <storage> --content ...,import`.
//
// Returns the storage name (always with "import" enabled on return).
func pickStagingStorage(ctx context.Context, client *px.Client, node *px.Node, preferred string) (string, error) {
	storages, err := node.Storages(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list storages on node %q: %v", node.Name, err)
	}

	usable := func(s *px.Storage) bool { return s.Enabled != 0 && s.Active != 0 }

	// 1. Prefer a storage that already has 'import' enabled.
	var withImport, preferredWithImport *px.Storage
	for _, s := range storages {
		if !usable(s) || !storageContentSet(s)["import"] {
			continue
		}
		if preferred != "" && s.Name == preferred {
			preferredWithImport = s
		}
		if withImport == nil {
			withImport = s
		}
	}
	if preferredWithImport != nil {
		return preferredWithImport.Name, nil
	}
	if withImport != nil {
		return withImport.Name, nil
	}

	// 2. No 'import' anywhere — pick a file-based storage and enable it.
	var candidate, preferredCandidate *px.Storage
	for _, s := range storages {
		if !usable(s) || !fileBasedStorageTypes[s.Type] {
			continue
		}
		if preferred != "" && s.Name == preferred {
			preferredCandidate = s
		}
		if candidate == nil {
			candidate = s
		}
	}
	if preferredCandidate != nil {
		candidate = preferredCandidate
	}
	if candidate == nil {
		var avail []string
		for _, s := range storages {
			avail = append(avail, fmt.Sprintf("%s (type=%s, content=%s)", s.Name, s.Type, s.Content))
		}
		return "", fmt.Errorf("no file-based storage (dir/nfs/cifs/cephfs/glusterfs) available to stage the disk image; storages on node %q: %s", node.Name, strings.Join(avail, ", "))
	}

	// Build the new content list = existing + 'import'.
	cs := storageContentSet(candidate)
	cs["import"] = true
	newContent := make([]string, 0, len(cs))
	for c := range cs {
		newContent = append(newContent, c)
	}
	sort.Strings(newContent)

	_, updateErr := client.UpdateClusterStorage(ctx, candidate.Name, px.ClusterStorageOptions{
		Name:  "content",
		Value: strings.Join(newContent, ","),
	})
	// PVE's PUT /storage/<id> returns the updated storage config object, but
	// the go-proxmox library tries to parse that as a task UPID and reports
	// an unmarshal error even on success. Verify by re-reading the storage
	// instead of trusting updateErr.
	verified, verr := node.Storage(ctx, candidate.Name)
	if verr == nil && storageContentSet(verified)["import"] {
		return candidate.Name, nil
	}
	if updateErr != nil {
		return "", fmt.Errorf("storage %q does not have 'import' content type, and auto-enabling it failed (need Datastore.Allocate permission on /storage/%s): %v", candidate.Name, candidate.Name, updateErr)
	}
	return "", fmt.Errorf("attempted to enable 'import' on storage %q but verification did not see the change", candidate.Name)
}

// downloadImageForImport stages a disk image at a node-local storage and
// returns the volume reference suitable for use as `import-from=...` in a
// VM disk parameter. The disk itself can be created on any storage that
// supports disk images — staging happens on a separate file-based storage.
func downloadImageForImport(ctx context.Context, client *px.Client, node *px.Node, imageURL, preferredStaging string) (importFromRef, stagingStorage string, err error) {
	stagingName, err := pickStagingStorage(ctx, client, node, preferredStaging)
	if err != nil {
		return "", "", err
	}

	staging, err := node.Storage(ctx, stagingName)
	if err != nil {
		return "", "", fmt.Errorf("failed to get staging storage %q: %v", stagingName, err)
	}

	filename := filenameFromURL(imageURL)
	dlTask, err := staging.DownloadURL(ctx, "import", filename, imageURL)
	if err != nil {
		return "", "", fmt.Errorf("failed to start image download to %s:import/%s: %v", stagingName, filename, err)
	}
	if err := waitForTaskWithTimeout(ctx, dlTask, 30*time.Minute); err != nil {
		return "", "", fmt.Errorf("image download to %s failed: %v", stagingName, err)
	}

	return fmt.Sprintf("%s:import/%s", stagingName, filename), stagingName, nil
}

func marshalResult(data any) (*mcp.CallToolResult, error) {
	out, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return mcp.NewToolResultError(fmt.Sprintf("Failed to marshal response: %v", err)), nil
	}
	return mcp.NewToolResultText(string(out)), nil
}

func sanitizeConfig(cfg any) any {
	if cfg == nil {
		return nil
	}
	raw, err := json.Marshal(cfg)
	if err != nil {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	for key := range sensitiveConfigKeys {
		delete(m, key)
	}
	return m
}

func withVM(client *px.Client, ctx context.Context, req mcp.CallToolRequest) (*px.VirtualMachine, string, int, error) {
	nodeName, err := requiredNode(req)
	if err != nil {
		return nil, "", 0, err
	}
	vmid, err := requiredVMID(req)
	if err != nil {
		return nil, "", 0, err
	}
	node, err := client.Node(ctx, nodeName)
	if err != nil {
		return nil, nodeName, vmid, fmt.Errorf("failed to get node %q: %v", nodeName, err)
	}
	vm, err := node.VirtualMachine(ctx, vmid)
	if err != nil {
		return nil, nodeName, vmid, fmt.Errorf("failed to get VM %d: %v", vmid, err)
	}
	return vm, nodeName, vmid, nil
}

func withContainer(client *px.Client, ctx context.Context, req mcp.CallToolRequest) (*px.Container, string, int, error) {
	nodeName, err := requiredNode(req)
	if err != nil {
		return nil, "", 0, err
	}
	vmid, err := requiredVMID(req)
	if err != nil {
		return nil, "", 0, err
	}
	node, err := client.Node(ctx, nodeName)
	if err != nil {
		return nil, nodeName, vmid, fmt.Errorf("failed to get node %q: %v", nodeName, err)
	}
	ct, err := node.Container(ctx, vmid)
	if err != nil {
		return nil, nodeName, vmid, fmt.Errorf("failed to get container %d: %v", vmid, err)
	}
	return ct, nodeName, vmid, nil
}

type nodeError struct {
	Node  string `json:"node"`
	Error string `json:"error"`
}

type listResult[T any] struct {
	Items  []T         `json:"items"`
	Errors []nodeError `json:"errors,omitempty"`
}

func forEachNode(ctx context.Context, client *px.Client, fn func(*px.Node) error) ([]nodeError, error) {
	nodes, err := client.Nodes(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list nodes: %v", err)
	}
	var errs []nodeError
	for _, ns := range nodes {
		if ns.Status != "online" {
			continue
		}
		node, err := client.Node(ctx, ns.Node)
		if err != nil {
			errs = append(errs, nodeError{Node: ns.Node, Error: err.Error()})
			continue
		}
		if err := fn(node); err != nil {
			errs = append(errs, nodeError{Node: ns.Node, Error: err.Error()})
		}
	}
	return errs, nil
}

func toolError(msg string) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return mcp.NewToolResultError(msg), nil
	}
}

func waitForTask(ctx context.Context, task *px.Task) error {
	return waitForTaskWithTimeout(ctx, task, taskTimeout)
}

func waitForTaskWithTimeout(ctx context.Context, task *px.Task, timeout time.Duration) error {
	if task == nil {
		return nil
	}
	if err := task.Wait(ctx, px.DefaultWaitInterval, timeout); err != nil {
		return err
	}
	if task.IsFailed {
		return fmt.Errorf("task failed: %s", task.ExitStatus)
	}
	return nil
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
	args := req.GetArguments()
	if _, ok := args[name]; !ok {
		return defaultVal
	}
	v, _ := optionalInt(req, name)
	if v == 0 {
		return defaultVal
	}
	return v
}

func optionalStr(req mcp.CallToolRequest, key, defaultVal string) string {
	if v, ok := req.GetArguments()[key].(string); ok && v != "" {
		return v
	}
	return defaultVal
}
