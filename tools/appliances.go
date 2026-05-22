package tools

import (
	"context"
	"fmt"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	px "github.com/luthermonson/go-proxmox"
)

func registerApplianceTools(s *server.MCPServer, client *px.Client) {
	s.AddTool(mcp.NewTool("appliance_list",
		mcp.WithDescription("List available appliance templates from the Proxmox catalog. These are pre-built LXC container templates (TurnKey Linux, system images, etc.) that can be downloaded to storage and used to create containers."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node to query the catalog from")),
		mcp.WithString("section", mcp.Description("Filter by section: system, turnkeylinux, mail (optional, lists all if omitted)")),
	), applianceListHandler(client))

	s.AddTool(mcp.NewTool("appliance_download",
		mcp.WithDescription("Download an appliance template from the Proxmox catalog to local storage. Use appliance_list to find available templates. The template name is the 'template' field from appliance_list (e.g. 'ubuntu-24.04-standard_24.04-2_amd64.tar.zst')."),
		mcp.WithString("node", mcp.Required(), mcp.Description("Node to download the template to")),
		mcp.WithString("template", mcp.Required(), mcp.Description("Template name from the catalog (the 'template' field from appliance_list)")),
		mcp.WithString("storage", mcp.Required(), mcp.Description("Storage pool to download to (must support 'vztmpl' content type)")),
	), applianceDownloadHandler(client))
}

func applianceListHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := requiredNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		appliances, err := node.Appliances(ctx)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to list appliances: %v", err)), nil
		}

		args := req.GetArguments()
		sectionFilter, _ := args["section"].(string)

		type applianceSummary struct {
			Template     string `json:"template"`
			Headline     string `json:"headline"`
			Os           string `json:"os"`
			Section      string `json:"section"`
			Version      string `json:"version"`
			Architecture string `json:"architecture"`
			Package      string `json:"package"`
			Description  string `json:"description,omitempty"`
		}

		items := make([]applianceSummary, 0, len(appliances))
		for _, a := range appliances {
			if sectionFilter != "" && a.Section != sectionFilter {
				continue
			}
			items = append(items, applianceSummary{
				Template:     a.Template,
				Headline:     a.Headline,
				Os:           a.Os,
				Section:      a.Section,
				Version:      a.Version,
				Architecture: a.Architecture,
				Package:      a.Package,
				Description:  a.Description,
			})
		}

		return marshalResult(items)
	}
}

func applianceDownloadHandler(client *px.Client) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		nodeName, err := requiredNode(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		template, err := req.RequireString("template")
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		storageName, err := requiredStorageName(req)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}

		node, err := client.Node(ctx, nodeName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to get node %q: %v", nodeName, err)), nil
		}

		ret, err := node.DownloadAppliance(ctx, template, storageName)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("Failed to download appliance %q: %v", template, err)), nil
		}

		result := struct {
			Template string `json:"template"`
			Storage  string `json:"storage"`
			Node     string `json:"node"`
			TaskID   string `json:"task_id"`
			Message  string `json:"message"`
		}{
			Template: template,
			Storage:  storageName,
			Node:     nodeName,
			TaskID:   ret,
			Message:  fmt.Sprintf("Appliance %q download started to storage %q on node %q", template, storageName, nodeName),
		}

		return marshalResult(result)
	}
}
