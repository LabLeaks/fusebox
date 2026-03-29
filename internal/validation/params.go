package validation

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/lableaks/fusebox/internal/config"
)

// ParamError describes a validation failure for a single parameter.
type ParamError struct {
	Param    string
	Type     string
	Expected string
	Got      string
}

func (e *ParamError) Error() string {
	return fmt.Sprintf("param %q (%s): expected %s, got %q", e.Param, e.Type, e.Expected, e.Got)
}

// ValidationErrors collects multiple parameter validation failures.
type ValidationErrors struct {
	Errors []ParamError
}

func (e *ValidationErrors) Error() string {
	msgs := make([]string, len(e.Errors))
	for i, pe := range e.Errors {
		msgs[i] = pe.Error()
	}
	return strings.Join(msgs, "; ")
}

// ValidateParams checks all provided params against the schema.
// Every param in schema is required. Unrecognized params are rejected.
func ValidateParams(params map[string]string, schema map[string]config.Param) error {
	var errs []ParamError

	// Reject unrecognized parameters.
	for name := range params {
		if _, ok := schema[name]; !ok {
			errs = append(errs, ParamError{
				Param:    name,
				Type:     "unknown",
				Expected: "parameter to be declared in schema",
				Got:      name,
			})
		}
	}

	// Require all declared parameters and validate values.
	for name, spec := range schema {
		val, ok := params[name]
		if !ok {
			errs = append(errs, ParamError{
				Param:    name,
				Type:     spec.Type,
				Expected: "parameter to be provided",
				Got:      "(missing)",
			})
			continue
		}

		if err := validateParam(name, val, spec); err != nil {
			errs = append(errs, *err)
		}
	}

	if len(errs) > 0 {
		return &ValidationErrors{Errors: errs}
	}
	return nil
}

func validateParam(name, value string, spec config.Param) *ParamError {
	switch spec.Type {
	case "regex":
		return validateRegex(name, value, spec)
	case "enum":
		return validateEnum(name, value, spec)
	case "int":
		return validateInt(name, value, spec)
	default:
		return &ParamError{
			Param:    name,
			Type:     spec.Type,
			Expected: "type to be regex, enum, or int",
			Got:      spec.Type,
		}
	}
}

func validateRegex(name, value string, spec config.Param) *ParamError {
	re, err := regexp.Compile(spec.Pattern)
	if err != nil {
		return &ParamError{
			Param:    name,
			Type:     "regex",
			Expected: fmt.Sprintf("valid regex pattern %q", spec.Pattern),
			Got:      fmt.Sprintf("compile error: %v", err),
		}
	}
	if !re.MatchString(value) {
		return &ParamError{
			Param:    name,
			Type:     "regex",
			Expected: fmt.Sprintf("match for pattern %q", spec.Pattern),
			Got:      value,
		}
	}
	return nil
}

func validateEnum(name, value string, spec config.Param) *ParamError {
	for _, v := range spec.Values {
		if v == value {
			return nil
		}
	}
	return &ParamError{
		Param:    name,
		Type:     "enum",
		Expected: fmt.Sprintf("one of [%s]", strings.Join(spec.Values, ", ")),
		Got:      value,
	}
}

func validateInt(name, value string, spec config.Param) *ParamError {
	n, err := strconv.Atoi(value)
	if err != nil {
		return &ParamError{
			Param:    name,
			Type:     "int",
			Expected: "an integer",
			Got:      value,
		}
	}
	if len(spec.Range) == 2 {
		if n < spec.Range[0] || n > spec.Range[1] {
			return &ParamError{
				Param:    name,
				Type:     "int",
				Expected: fmt.Sprintf("integer in [%d, %d]", spec.Range[0], spec.Range[1]),
				Got:      value,
			}
		}
	}
	return nil
}
