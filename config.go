package main

import (
	"bytes"
	"fmt"
	"github.com/alecthomas/kong"
	kongyaml "github.com/alecthomas/kong-yaml"
	"gopkg.in/yaml.v3"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

var (
	Commit  = "none"
	Date    = "2006-01-02T15:04:05Z"
	Version = "dev"
)

type Config struct {
	Tokens         map[string]string `name:"token" short:"T" help:"Personal access token" env:""`
	Verbose        int               `name:"verbose" short:"v" optional:"true" help:"verbose output" type:"counter" default:"0" env:""`
	Force          bool              `name:"force" short:"f" optional:"true" help:"force overwriting formulas" env:""`
	Path           []string          `arg:"" optional:"" help:"path to project directory" default:"."`
	Interactive    bool              `name:"interactive" negatable:"" short:"i" help:"interactive mode" default:"true"`
	NoPush         bool              `name:"no-push" help:"disable git push" default:"false"`
	GenerateConfig bool              `name:"generate-config" help:"generate config file" default:"false"`
	Hooks          Hooks             `name:"hooks" help:"hooks" hidden:""`
	Version        bool              `name:"version" help:"show version"`
	KeepRevision   bool              `name:"keep-revision" help:"keep revision number" default:"false"`
	RunHooks       bool              `name:"run-hooks" help:"run configured hooks after updating formulas" default:"false" env:""`
}

func (c *Config) GenConfig() (string, error) {
	c.GenerateConfig = false
	redacted := *c
	redacted.Tokens = redactTokens(c.Tokens)
	return GenerateYAMLWithComments(redacted)
}

// redactTokens hides real tokens for printing (they may already be loaded from
// env/config), falling back to example placeholders when none are set.
func redactTokens(tokens map[string]string) map[string]string {
	if len(tokens) == 0 {
		return map[string]string{
			"gitlab.com":      "GL_PERSONAL_ACCESS_TOKEN",
			"github.com":      "GH_PERSONAL_ACCESS_TOKEN",
			"git.example.com": "GL_EXAMPLE_PERSONAL_ACCESS_TOKEN",
		}
	}
	redacted := make(map[string]string, len(tokens))
	for host := range tokens {
		redacted[host] = "REDACTED"
	}
	return redacted
}

func (c *Config) GetVersion() string {
	return fmt.Sprintf("%s %s-%.8s (%s)", APPNAME, Version, Commit, Date)
}

func parseArgs() (Config, error) {
	c := Config{}
	dir, err := os.Getwd()
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	configSearchDir := []string{
		filepath.Join(dir, strings.ToLower(APPNAME)+".yaml"),
	}
	home, err := os.UserHomeDir()
	if err == nil {
		configSearchDir = append(configSearchDir,
			filepath.Join(home, ".config", strings.ToLower(APPNAME), strings.ToLower(APPNAME)+".yaml"),
			filepath.Join(home, ".config", strings.ToLower(APPNAME)+".yaml"),
		)
	}

	kongOptions := []kong.Option{
		kong.Name(APPNAME),
		kong.Description("Application used to bump releases on internal brew tap"),
		kong.UsageOnError(),
		kong.DefaultEnvars(strings.ToUpper(APPNAME)),
		kong.Configuration(kongyaml.Loader, configSearchDir...),
	}
	_ = kong.Parse(&c, kongOptions...)

	return c, nil
}

// GenerateYAMLWithComments generates a YAML file with comments.
func GenerateYAMLWithComments(cfg any) (string, error) {
	var node yaml.Node

	// Marshal the struct into a yaml.Node.
	data, err := MarshalYAML(cfg)
	if err != nil {
		return "", fmt.Errorf("failed to marshal struct: %w", err)
	}
	if err := yaml.Unmarshal(data, &node); err != nil {
		return "", fmt.Errorf("failed to unmarshal back into yaml.Node: %w", err)
	}

	nameHelpMap := make(map[string]string)
	t := reflect.TypeOf(cfg)

	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		// Get the "name" tag.
		help := field.Tag.Get("help")
		if help == "" {
			continue // Skip fields without the "name" tag.
		}
		name := field.Tag.Get("name")
		if name == "" {
			continue // Skip fields without the "name" tag.
		}
		nameHelpMap[name] = help
	}

	for i, n := range node.Content[0].Content {
		v := n.Value
		n.HeadComment = nameHelpMap[v]
		if i != 0 && n.HeadComment != "" {
			n.HeadComment = fmt.Sprintf("\n%s", nameHelpMap[v])
		}
	}

	var buf bytes.Buffer
	encoder := yaml.NewEncoder(&buf)
	encoder.SetIndent(2)
	if err := encoder.Encode(&node); err != nil {
		return "", fmt.Errorf("failed to encode yaml.Node: %w", err)
	}

	return buf.String(), nil
}

// MarshalYAML implements the yaml.Marshaler interface and uses reflection.
func MarshalYAML(c any) ([]byte, error) {
	v := reflect.ValueOf(c)
	t := reflect.TypeOf(c)

	// Precompute the pointer type for *url.URL.
	urlPtrType := reflect.TypeOf((*url.URL)(nil))
	// Create a map to hold the resulting YAML key-value pairs.
	mapped := make(map[string]interface{})

	// Iterate through the struct fields.
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		value := v.Field(i)

		// Get the "name" tag.
		tag := field.Tag.Get("name")
		if tag == "" {
			continue // Skip fields without the "name" tag.
		}

		// If the field is of type *url.URL, use its string representation.
		if field.Type == urlPtrType {
			if value.IsNil() {
				mapped[tag] = "" // or nil, depending on your preference
			} else {
				// Convert the *url.URL to its string representation.
				u := value.Interface().(*url.URL)
				mapped[tag] = u.String()
			}
		} else {
			mapped[tag] = value.Interface()
		}

	}

	return yaml.Marshal(mapped)
}
