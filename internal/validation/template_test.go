package validation

import (
	"strings"
	"testing"
)

func TestExpandTemplate_BasicSubstitution(t *testing.T) {
	result, err := ExpandTemplate("git checkout {branch}", map[string]string{"branch": "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "git checkout 'main'" {
		t.Fatalf("expected git checkout 'main', got: %s", result)
	}
}

func TestExpandTemplate_MultipleParams(t *testing.T) {
	result, err := ExpandTemplate("deploy {app} to {env}", map[string]string{
		"app": "web",
		"env": "prod",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "deploy 'web' to 'prod'" {
		t.Fatalf("expected deploy 'web' to 'prod', got: %s", result)
	}
}

func TestExpandTemplate_SameParamTwice(t *testing.T) {
	result, err := ExpandTemplate("echo {x} and {x}", map[string]string{"x": "hello"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo 'hello' and 'hello'" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestExpandTemplate_MissingParam(t *testing.T) {
	_, err := ExpandTemplate("git checkout {branch}", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing param")
	}
	if !strings.Contains(err.Error(), "branch") {
		t.Fatalf("error should mention 'branch', got: %v", err)
	}
}

func TestExpandTemplate_NoParams(t *testing.T) {
	result, err := ExpandTemplate("ls -la", map[string]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "ls -la" {
		t.Fatalf("expected ls -la, got: %s", result)
	}
}

func TestExpandTemplate_ShellInjection_CommandSubstitution(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": "$(rm -rf /)"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The value must be single-quoted so sh -c won't interpret $()
	if strings.Contains(result, "$(") && !strings.Contains(result, "'$(") {
		t.Fatalf("shell injection not prevented: %s", result)
	}
	if result != "echo '$(rm -rf /)'" {
		t.Fatalf("expected quoted value, got: %s", result)
	}
}

func TestExpandTemplate_ShellInjection_Backticks(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": "`whoami`"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo '`whoami`'" {
		t.Fatalf("expected quoted backticks, got: %s", result)
	}
}

func TestExpandTemplate_ShellInjection_Semicolon(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": "foo; rm -rf /"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo 'foo; rm -rf /'" {
		t.Fatalf("expected quoted semicolon, got: %s", result)
	}
}

func TestExpandTemplate_ShellInjection_SingleQuoteEscape(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": "it's dangerous"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Single quote in value must be escaped: 'it'\''s dangerous'
	if result != "echo 'it'\\''s dangerous'" {
		t.Fatalf("expected escaped single quote, got: %s", result)
	}
}

func TestExpandTemplate_Pipe(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": "a | cat /etc/passwd"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo 'a | cat /etc/passwd'" {
		t.Fatalf("expected quoted pipe, got: %s", result)
	}
}

func TestExpandTemplate_EmptyValue(t *testing.T) {
	result, err := ExpandTemplate("echo {val}", map[string]string{"val": ""})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo ''" {
		t.Fatalf("expected empty quoted string, got: %s", result)
	}
}

func TestExpandTemplate_ExtraParamsIgnored(t *testing.T) {
	// Extra params in the map that aren't in the template are fine.
	result, err := ExpandTemplate("echo {a}", map[string]string{"a": "1", "b": "2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "echo '1'" {
		t.Fatalf("unexpected result: %s", result)
	}
}

func TestShellQuote(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"hello", "'hello'"},
		{"", "''"},
		{"it's", "'it'\\''s'"},
		{"a'b'c", "'a'\\''b'\\''c'"},
		{"$(cmd)", "'$(cmd)'"},
		{"`cmd`", "'`cmd`'"},
		{"foo bar", "'foo bar'"},
	}
	for _, tc := range cases {
		got := shellQuote(tc.in)
		if got != tc.want {
			t.Errorf("shellQuote(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
