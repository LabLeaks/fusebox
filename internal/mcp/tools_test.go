package mcp

import (
	"encoding/json"
	"testing"
)

func intPtr(v int) *int { return &v }

func TestActionToTool_NoParams(t *testing.T) {
	a := ActionDescriptor{
		Name:        "build",
		Description: "Build the project",
	}
	tool := ActionToTool(a)

	if tool.Name != "fusebox_build" {
		t.Fatalf("expected fusebox_build, got %s", tool.Name)
	}
	if tool.Description != "Build the project" {
		t.Fatalf("unexpected description: %s", tool.Description)
	}
	if tool.InputSchema["type"] != "object" {
		t.Fatalf("expected object type, got %v", tool.InputSchema["type"])
	}
	if _, ok := tool.InputSchema["properties"]; ok {
		t.Fatal("expected no properties for param-less action")
	}
}

func TestActionToTool_RegexParam(t *testing.T) {
	a := ActionDescriptor{
		Name:        "checkout",
		Description: "Switch branch",
		Params: map[string]ParamDescriptor{
			"branch": {Type: "regex", Pattern: `^[a-z0-9\-]+$`},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	branch := props["branch"].(map[string]interface{})
	if branch["type"] != "string" {
		t.Fatalf("expected string type, got %v", branch["type"])
	}
	if branch["pattern"] != `^[a-z0-9\-]+$` {
		t.Fatalf("unexpected pattern: %v", branch["pattern"])
	}
}

func TestActionToTool_EnumParam(t *testing.T) {
	a := ActionDescriptor{
		Name:        "deploy",
		Description: "Deploy app",
		Params: map[string]ParamDescriptor{
			"env": {Type: "enum", Values: []string{"dev", "staging", "prod"}},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	env := props["env"].(map[string]interface{})
	if env["type"] != "string" {
		t.Fatalf("expected string type, got %v", env["type"])
	}

	// enum values come back as []interface{} after map conversion.
	vals := env["enum"].([]string)
	if len(vals) != 3 || vals[0] != "dev" || vals[1] != "staging" || vals[2] != "prod" {
		t.Fatalf("unexpected enum values: %v", vals)
	}
}

func TestActionToTool_IntParam(t *testing.T) {
	a := ActionDescriptor{
		Name:        "scale",
		Description: "Scale replicas",
		Params: map[string]ParamDescriptor{
			"count": {Type: "int", Min: intPtr(1), Max: intPtr(100)},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	count := props["count"].(map[string]interface{})
	if count["type"] != "integer" {
		t.Fatalf("expected integer type, got %v", count["type"])
	}
	if count["minimum"] != 1 {
		t.Fatalf("expected minimum 1, got %v", count["minimum"])
	}
	if count["maximum"] != 100 {
		t.Fatalf("expected maximum 100, got %v", count["maximum"])
	}
}

func TestActionToTool_IntParam_NoRange(t *testing.T) {
	a := ActionDescriptor{
		Name:        "set",
		Description: "Set value",
		Params: map[string]ParamDescriptor{
			"val": {Type: "int"},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	val := props["val"].(map[string]interface{})
	if _, ok := val["minimum"]; ok {
		t.Fatal("expected no minimum for unbounded int")
	}
	if _, ok := val["maximum"]; ok {
		t.Fatal("expected no maximum for unbounded int")
	}
}

func TestActionToTool_MultipleParams(t *testing.T) {
	a := ActionDescriptor{
		Name:        "deploy",
		Description: "Deploy",
		Params: map[string]ParamDescriptor{
			"env":   {Type: "enum", Values: []string{"dev", "prod"}},
			"tag":   {Type: "regex", Pattern: `^v\d+\.\d+\.\d+$`},
			"count": {Type: "int", Min: intPtr(1), Max: intPtr(10)},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	if len(props) != 3 {
		t.Fatalf("expected 3 properties, got %d", len(props))
	}

	required := tool.InputSchema["required"].([]string)
	if len(required) != 3 {
		t.Fatalf("expected 3 required params, got %d", len(required))
	}
}

func TestActionToTool_UnknownType(t *testing.T) {
	a := ActionDescriptor{
		Name:        "x",
		Description: "Unknown",
		Params: map[string]ParamDescriptor{
			"p": {Type: "float"},
		},
	}
	tool := ActionToTool(a)

	props := tool.InputSchema["properties"].(map[string]interface{})
	p := props["p"].(map[string]interface{})
	if p["type"] != "string" {
		t.Fatalf("expected fallback to string, got %v", p["type"])
	}
}

func TestActionToTool_JSONSerializable(t *testing.T) {
	a := ActionDescriptor{
		Name:        "build",
		Description: "Build",
		Params: map[string]ParamDescriptor{
			"target": {Type: "enum", Values: []string{"debug", "release"}},
		},
	}
	tool := ActionToTool(a)

	data, err := json.Marshal(tool)
	if err != nil {
		t.Fatalf("failed to marshal tool: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("failed to unmarshal tool: %v", err)
	}

	if parsed["name"] != "fusebox_build" {
		t.Fatalf("name mismatch after roundtrip: %v", parsed["name"])
	}
}

func TestParamToJSONSchema_Regex(t *testing.T) {
	schema := ParamToJSONSchema(ParamDescriptor{Type: "regex", Pattern: `^\d+$`})
	if schema["type"] != "string" {
		t.Fatalf("expected string, got %v", schema["type"])
	}
	if schema["pattern"] != `^\d+$` {
		t.Fatalf("unexpected pattern: %v", schema["pattern"])
	}
}

func TestParamToJSONSchema_RegexNoPattern(t *testing.T) {
	schema := ParamToJSONSchema(ParamDescriptor{Type: "regex"})
	if schema["type"] != "string" {
		t.Fatalf("expected string, got %v", schema["type"])
	}
	if _, ok := schema["pattern"]; ok {
		t.Fatal("expected no pattern key when pattern is empty")
	}
}

func TestParamToJSONSchema_Enum(t *testing.T) {
	schema := ParamToJSONSchema(ParamDescriptor{Type: "enum", Values: []string{"a", "b"}})
	if schema["type"] != "string" {
		t.Fatalf("expected string, got %v", schema["type"])
	}
	vals := schema["enum"].([]string)
	if len(vals) != 2 || vals[0] != "a" || vals[1] != "b" {
		t.Fatalf("unexpected enum: %v", vals)
	}
}

func TestParamToJSONSchema_Int(t *testing.T) {
	schema := ParamToJSONSchema(ParamDescriptor{Type: "int", Min: intPtr(0), Max: intPtr(65535)})
	if schema["type"] != "integer" {
		t.Fatalf("expected integer, got %v", schema["type"])
	}
	if schema["minimum"] != 0 {
		t.Fatalf("expected minimum 0, got %v", schema["minimum"])
	}
	if schema["maximum"] != 65535 {
		t.Fatalf("expected maximum 65535, got %v", schema["maximum"])
	}
}
