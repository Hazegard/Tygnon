package main

import (
	"strings"
	"testing"
)

func TestRedactTokensNoneLoaded(t *testing.T) {
	got := redactTokens(nil)
	want := map[string]string{
		"gitlab.com":      "GL_PERSONAL_ACCESS_TOKEN",
		"github.com":      "GH_PERSONAL_ACCESS_TOKEN",
		"git.example.com": "GL_EXAMPLE_PERSONAL_ACCESS_TOKEN",
	}
	if len(got) != len(want) {
		t.Fatalf("redactTokens(nil) = %v, want %v", got, want)
	}
	for host, placeholder := range want {
		if got[host] != placeholder {
			t.Errorf("redactTokens(nil)[%q] = %q, want %q", host, got[host], placeholder)
		}
	}
}

func TestRedactTokensRealTokensLoaded(t *testing.T) {
	real := map[string]string{
		"gitlab.com":      "glpat-supersecret",
		"git.example.com": "another-real-secret",
	}
	got := redactTokens(real)
	if len(got) != len(real) {
		t.Fatalf("redactTokens should preserve the same set of hosts, got %v", got)
	}
	for host, secret := range real {
		if got[host] == secret {
			t.Fatalf("redactTokens leaked the real secret for %q", host)
		}
		if got[host] != "REDACTED" {
			t.Errorf("redactTokens[%q] = %q, want REDACTED", host, got[host])
		}
	}
}

func TestGenConfigRedactsRealTokens(t *testing.T) {
	c := &Config{
		Tokens: map[string]string{
			"gitlab.com": "glpat-supersecret",
		},
	}
	out, err := c.GenConfig()
	if err != nil {
		t.Fatalf("GenConfig: %v", err)
	}
	if strings.Contains(out, "glpat-supersecret") {
		t.Fatalf("GenConfig leaked a real token into its output:\n%s", out)
	}
	if !strings.Contains(out, "REDACTED") {
		t.Fatalf("expected REDACTED in output, got:\n%s", out)
	}
	if !strings.Contains(out, "gitlab.com") {
		t.Fatalf("expected the host key to be preserved, got:\n%s", out)
	}
	if c.GenerateConfig {
		t.Errorf("GenConfig should reset GenerateConfig to false")
	}
}

func TestGenConfigPlaceholdersWhenNoTokensLoaded(t *testing.T) {
	c := &Config{}
	out, err := c.GenConfig()
	if err != nil {
		t.Fatalf("GenConfig: %v", err)
	}
	if !strings.Contains(out, "GL_PERSONAL_ACCESS_TOKEN") {
		t.Fatalf("expected placeholder token names when no real tokens are loaded, got:\n%s", out)
	}
}

type marshalTestStruct struct {
	Name    string `name:"name" help:"the name field"`
	Enabled bool   `name:"enabled" help:"whether it's enabled"`
	Ignored string // no `name` tag: must be skipped entirely
}

func TestGenerateYAMLWithComments(t *testing.T) {
	s := marshalTestStruct{Name: "foo", Enabled: true, Ignored: "should not appear"}
	out, err := GenerateYAMLWithComments(s)
	if err != nil {
		t.Fatalf("GenerateYAMLWithComments: %v", err)
	}
	if !strings.Contains(out, "name: foo") {
		t.Errorf("expected 'name: foo' in output:\n%s", out)
	}
	if !strings.Contains(out, "enabled: true") {
		t.Errorf("expected 'enabled: true' in output:\n%s", out)
	}
	if !strings.Contains(out, "the name field") {
		t.Errorf("expected help comment in output:\n%s", out)
	}
	if strings.Contains(out, "should not appear") {
		t.Errorf("field without a name tag leaked into output:\n%s", out)
	}
}

func TestMarshalYAML(t *testing.T) {
	s := marshalTestStruct{Name: "bar", Enabled: false}
	data, err := MarshalYAML(s)
	if err != nil {
		t.Fatalf("MarshalYAML: %v", err)
	}
	if !strings.Contains(string(data), "name: bar") {
		t.Errorf("expected 'name: bar' in %s", data)
	}
	if strings.Contains(string(data), "Ignored") {
		t.Errorf("untagged field leaked into %s", data)
	}
}
