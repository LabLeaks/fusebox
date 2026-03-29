package mcp

import "fmt"

// ToolSchema represents an MCP tool definition.
type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// ActionToTool converts an ActionDescriptor to an MCP tool schema.
func ActionToTool(a ActionDescriptor) ToolSchema {
	properties := make(map[string]interface{}, len(a.Params))
	required := make([]string, 0, len(a.Params))

	for name, p := range a.Params {
		properties[name] = ParamToJSONSchema(p)
		required = append(required, name)
	}

	inputSchema := map[string]interface{}{
		"type": "object",
	}
	if len(properties) > 0 {
		inputSchema["properties"] = properties
		inputSchema["required"] = required
	}

	return ToolSchema{
		Name:        fmt.Sprintf("fusebox_%s", a.Name),
		Description: a.Description,
		InputSchema: inputSchema,
	}
}

// ParamToJSONSchema converts a ParamDescriptor to a JSON Schema property.
func ParamToJSONSchema(p ParamDescriptor) map[string]interface{} {
	switch p.Type {
	case "regex":
		schema := map[string]interface{}{"type": "string"}
		if p.Pattern != "" {
			schema["pattern"] = p.Pattern
		}
		return schema
	case "enum":
		return map[string]interface{}{
			"type": "string",
			"enum": p.Values,
		}
	case "int":
		schema := map[string]interface{}{"type": "integer"}
		if p.Min != nil {
			schema["minimum"] = *p.Min
		}
		if p.Max != nil {
			schema["maximum"] = *p.Max
		}
		return schema
	default:
		return map[string]interface{}{"type": "string"}
	}
}
