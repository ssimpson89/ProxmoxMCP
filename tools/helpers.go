package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"time"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

const taskTimeout = 5 * time.Minute

var (
	nodeNameRe    = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]*$`)
	snapNameRe    = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_-]{0,39}$`)
	storageNameRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9_.-]*$`)

	sensitiveConfigKeys = map[string]bool{
		"cipassword": true,
		"sshkeys":    true,
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
	if task == nil {
		return nil
	}
	return task.Wait(ctx, px.DefaultWaitInterval, taskTimeout)
}
