package main

import (
	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
	"os"
	"os/exec"
	"slices"
)

type Hook struct {
	Name       string   `yaml:"name"`
	Path       string   `yaml:"path"`
	Parameters []string `yaml:"parameters"`
	Formulas   []string `yaml:"formulas"`
}

func (h *Hook) ApplyHook(f FormulaInfo, hookName string, c Config, l zerolog.Logger) error {
	params := append([]string{f.File}, h.Parameters...)
	l.Info().Str("Formula", f.File).Str("Hook", hookName).Msg("Applying hook")
	l.Trace().Strs("Parameters", params).Msg("Parameters")

	cmd := exec.Command(h.Path, params...)
	if c.Verbose > 0 {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	}
	return cmd.Run()
}

type Hooks []Hook

func (h *Hooks) Decode(ctx *kong.DecodeContext) error {
	token := ctx.Scan.PopValueInto("hooks", h)

	return token
}

func (h *Hooks) ApplyHook(f FormulaInfo, c Config, l zerolog.Logger) {
	for _, hook := range *h {
		if slices.Contains(hook.Formulas, f.GetLocalFile()) || slices.Contains(hook.Formulas, f.GetName()) {
			err := hook.ApplyHook(f, hook.Name, c, l)
			if err != nil {
				l.Error().Err(err).Str("Hook", hook.Name).Str("Formula", f.File).Msg("Error applying hook")
			}
		}
	}
}
