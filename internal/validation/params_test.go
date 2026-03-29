package validation

import (
	"strings"
	"testing"

	"github.com/lableaks/fusebox/internal/config"
)

func TestValidateParams_Regex_Valid(t *testing.T) {
	schema := map[string]config.Param{
		"branch": {Type: "regex", Pattern: `^[a-z0-9\-]+$`},
	}
	err := ValidateParams(map[string]string{"branch": "feature-123"}, schema)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateParams_Regex_Invalid(t *testing.T) {
	schema := map[string]config.Param{
		"branch": {Type: "regex", Pattern: `^[a-z0-9\-]+$`},
	}
	err := ValidateParams(map[string]string{"branch": "UPPER_CASE"}, schema)
	if err == nil {
		t.Fatal("expected error for invalid regex match")
	}
	assertParamError(t, err, "branch", "regex")
}

func TestValidateParams_Regex_EmptyString(t *testing.T) {
	schema := map[string]config.Param{
		"name": {Type: "regex", Pattern: `^.+$`},
	}
	err := ValidateParams(map[string]string{"name": ""}, schema)
	if err == nil {
		t.Fatal("expected error for empty string against .+ pattern")
	}
}

func TestValidateParams_Regex_Unicode(t *testing.T) {
	schema := map[string]config.Param{
		"label": {Type: "regex", Pattern: `^\p{L}+$`},
	}
	err := ValidateParams(map[string]string{"label": "café"}, schema)
	if err != nil {
		t.Fatalf("expected unicode to match, got: %v", err)
	}
}

func TestValidateParams_Enum_Valid(t *testing.T) {
	schema := map[string]config.Param{
		"env": {Type: "enum", Values: []string{"dev", "staging", "prod"}},
	}
	err := ValidateParams(map[string]string{"env": "staging"}, schema)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateParams_Enum_Invalid(t *testing.T) {
	schema := map[string]config.Param{
		"env": {Type: "enum", Values: []string{"dev", "staging", "prod"}},
	}
	err := ValidateParams(map[string]string{"env": "test"}, schema)
	if err == nil {
		t.Fatal("expected error for invalid enum value")
	}
	assertParamError(t, err, "env", "enum")
}

func TestValidateParams_Enum_EmptyString(t *testing.T) {
	schema := map[string]config.Param{
		"env": {Type: "enum", Values: []string{"dev", "staging"}},
	}
	err := ValidateParams(map[string]string{"env": ""}, schema)
	if err == nil {
		t.Fatal("expected error for empty string not in enum")
	}
}

func TestValidateParams_Int_Valid(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "8080"}, schema)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidateParams_Int_BoundaryLow(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "1"}, schema)
	if err != nil {
		t.Fatalf("expected boundary value 1 to pass, got: %v", err)
	}
}

func TestValidateParams_Int_BoundaryHigh(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "65535"}, schema)
	if err != nil {
		t.Fatalf("expected boundary value 65535 to pass, got: %v", err)
	}
}

func TestValidateParams_Int_BelowRange(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "0"}, schema)
	if err == nil {
		t.Fatal("expected error for value below range")
	}
	assertParamError(t, err, "port", "int")
}

func TestValidateParams_Int_AboveRange(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "65536"}, schema)
	if err == nil {
		t.Fatal("expected error for value above range")
	}
}

func TestValidateParams_Int_NotAnInteger(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 65535}},
	}
	err := ValidateParams(map[string]string{"port": "abc"}, schema)
	if err == nil {
		t.Fatal("expected error for non-integer value")
	}
	assertParamError(t, err, "port", "int")
}

func TestValidateParams_Int_EmptyString(t *testing.T) {
	schema := map[string]config.Param{
		"count": {Type: "int", Range: []int{0, 100}},
	}
	err := ValidateParams(map[string]string{"count": ""}, schema)
	if err == nil {
		t.Fatal("expected error for empty string as int")
	}
}

func TestValidateParams_Int_NoRange(t *testing.T) {
	schema := map[string]config.Param{
		"count": {Type: "int"},
	}
	err := ValidateParams(map[string]string{"count": "-999"}, schema)
	if err != nil {
		t.Fatalf("expected int without range to accept any integer, got: %v", err)
	}
}

func TestValidateParams_UnrecognizedParam(t *testing.T) {
	schema := map[string]config.Param{
		"branch": {Type: "regex", Pattern: `^.+$`},
	}
	err := ValidateParams(map[string]string{"branch": "main", "extra": "nope"}, schema)
	if err == nil {
		t.Fatal("expected error for unrecognized param")
	}
	if !strings.Contains(err.Error(), "extra") {
		t.Fatalf("error should mention 'extra', got: %v", err)
	}
}

func TestValidateParams_MissingRequired(t *testing.T) {
	schema := map[string]config.Param{
		"branch": {Type: "regex", Pattern: `^.+$`},
		"env":    {Type: "enum", Values: []string{"dev"}},
	}
	err := ValidateParams(map[string]string{"branch": "main"}, schema)
	if err == nil {
		t.Fatal("expected error for missing required param")
	}
	if !strings.Contains(err.Error(), "env") {
		t.Fatalf("error should mention 'env', got: %v", err)
	}
}

func TestValidateParams_UnknownType(t *testing.T) {
	schema := map[string]config.Param{
		"x": {Type: "float"},
	}
	err := ValidateParams(map[string]string{"x": "1.5"}, schema)
	if err == nil {
		t.Fatal("expected error for unknown type")
	}
}

func TestValidateParams_VeryLongValue(t *testing.T) {
	schema := map[string]config.Param{
		"data": {Type: "regex", Pattern: `^[a-z]+$`},
	}
	longVal := strings.Repeat("a", 100000)
	err := ValidateParams(map[string]string{"data": longVal}, schema)
	if err != nil {
		t.Fatalf("expected long valid value to pass, got: %v", err)
	}
}

func TestValidateParams_EmptySchemaEmptyParams(t *testing.T) {
	err := ValidateParams(map[string]string{}, map[string]config.Param{})
	if err != nil {
		t.Fatalf("expected no error for empty schema and params, got: %v", err)
	}
}

func TestValidateParams_StructuredError(t *testing.T) {
	schema := map[string]config.Param{
		"port": {Type: "int", Range: []int{1, 100}},
	}
	err := ValidateParams(map[string]string{"port": "999"}, schema)
	if err == nil {
		t.Fatal("expected error")
	}
	ve, ok := err.(*ValidationErrors)
	if !ok {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}
	if len(ve.Errors) != 1 {
		t.Fatalf("expected 1 error, got %d", len(ve.Errors))
	}
	pe := ve.Errors[0]
	if pe.Param != "port" || pe.Type != "int" {
		t.Fatalf("unexpected error fields: %+v", pe)
	}
}

func assertParamError(t *testing.T, err error, param, typ string) {
	t.Helper()
	ve, ok := err.(*ValidationErrors)
	if !ok {
		t.Fatalf("expected *ValidationErrors, got %T", err)
	}
	for _, pe := range ve.Errors {
		if pe.Param == param && pe.Type == typ {
			return
		}
	}
	t.Fatalf("expected error for param %q type %q, got: %v", param, typ, err)
}
