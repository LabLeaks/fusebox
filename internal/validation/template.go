package validation

import (
	"fmt"
	"regexp"
	"strings"
)

// placeholderRe matches {param_name} placeholders in exec templates.
var placeholderRe = regexp.MustCompile(`\{([a-zA-Z_][a-zA-Z0-9_]*)\}`)

// ExpandTemplate substitutes {param_name} placeholders with shell-escaped values.
// Params must be pre-validated before calling this function.
func ExpandTemplate(execTemplate string, params map[string]string) (string, error) {
	var missing []string

	result := placeholderRe.ReplaceAllStringFunc(execTemplate, func(match string) string {
		name := match[1 : len(match)-1] // strip { and }
		val, ok := params[name]
		if !ok {
			missing = append(missing, name)
			return match
		}
		return shellQuote(val)
	})

	if len(missing) > 0 {
		return "", fmt.Errorf("template references undefined params: %s", strings.Join(missing, ", "))
	}

	return result, nil
}

// shellQuote wraps a value in single quotes, escaping any embedded single quotes.
// This is the safest quoting for sh -c: 'value' with ' replaced by '\''
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
