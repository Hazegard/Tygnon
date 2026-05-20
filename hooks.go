package main

import (
	"github.com/rs/zerolog/log"
	"os/exec"
	"path/filepath"
	"strings"
)

func Hook(cfg Config, formula string) error {
	if len(cfg.HooksFolder) == 0 {
		return nil
	}

	hookFile := filepath.Join(cfg.HooksFolder, formula)
	hookFile = strings.ReplaceAll(hookFile, ".rb", ".sh")
	if !fileExists(hookFile) {
		return nil
	}

	cmd := exec.Command(hookFile)
	log.Info().Str("Formula", formula).Msg("Executing hook")
	err := cmd.Run()
	if err != nil {
		log.Error().Str("Formula", formula).Err(err).Msg("Error executing hook")
		return err
	}
	log.Info().Str("Formula", formula).Msg("Hook executed")
	return nil
}
