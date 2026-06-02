package main

import (
	"fmt"
	"github.com/rs/zerolog"
	"path/filepath"
	"strings"
)

const APPNAME = "tygnon"

func main() {
	l := NewLogger()
	config, err := parseArgs()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error parsing cli arguments")
		return
	}
	if config.GenerateConfig {
		yamlConfig, err := config.GenConfig()
		if err != nil {
			l.Error().Stack().Err(err).Msg("error generating config")
			return
		}
		fmt.Println(yamlConfig)
		return
	}
	if config.Verbose > 0 {
		l = l.Level(zerolog.TraceLevel)
	}

	for _, path := range config.Path {
		hasUpdate := HandlePath(path, config, l)
		if hasUpdate {
			err = HandleGit(config, path, l)
		}
	}

}

func HandlePath(path string, config Config, l zerolog.Logger) bool {
	absPath, err := filepath.Abs(path)
	if err == nil {
		path = absPath
	}
	l.Info().Str("Path", path).Msg("Checking for new versions...")
	dir, err := GetContainingDir(path)
	if err != nil {
		l.Error().Stack().Err(err).Msg("error getting directory")
		return false
	}

	formulaFileNames, err := FindBrewFormulaFiles(path)
	if err != nil {
		l.Error().Stack().Err(err).Msg("error finding brew formula files")
		return false
	}

	var formulas []FormulaInfo
	for _, f := range formulaFileNames {
		formula, err := ParseFormula(f, dir)
		if err != nil {
			l.Error().Stack().Err(err).Msg("error parsing formula")
			continue
		}
		formulas = append(formulas, formula)
	}
	if len(formulas) == 0 {
		l.Warn().Str("Path", path).Msg("no formula files found, aborting")
		return false
	}

	var updatedFormulas []FormulaInfo
	for _, formula := range formulas {
		err, formula := HandleFormula(formula, config, l)
		if err != nil {
			l.Err(err).Msg("error handling formula")
			continue
		}
		if formula != (FormulaInfo{}) {
			updatedFormulas = append(updatedFormulas, formula)
		}
	}

	if len(updatedFormulas) == 0 {
		l.Info().Str("Path", path).Msg("No new versions found")
		return false
	}
	l.Warn().Str("Path", path).Msg("Updated formula files:")

	for _, updatedFormula := range updatedFormulas {
		l.Warn().Msgf("  - %s", TrimDir(dir, updatedFormula.File))
	}

	if config.Hooks != nil {
		l.Info().Msg("Executing hooks")

		for _, updatedFormula := range updatedFormulas {
			l.Debug().Msgf("  - %s", TrimDir(dir, updatedFormula.File))
			config.Hooks.ApplyHook(updatedFormula, config, l)
			// _ = HookFunc(config, updatedFormula)
		}
	}
	return len(updatedFormulas) > 0
}

func HandleFormula(formula FormulaInfo, config Config, l zerolog.Logger) (error, FormulaInfo) {
	l.Info().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Msg("Checking new version...")
	var gitClient GitApi
	gitDomain, err := formula.GetInstance()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error getting git client")
	}
	switch formula.Instance {
	case Gitlab:
		gitClient, err = NewGitlabApi(config.Tokens[gitDomain], fmt.Sprintf("https://%s", gitDomain))
	case Github:
		gitClient = NewGithubApi(config.Tokens[gitDomain])
	case Gitea:
		gitClient, err = NewGiteaApi(config.Tokens[gitDomain], fmt.Sprintf("https://%s", gitDomain))
	default:
		instance, _ := formula.GetInstance()
		l.Info().Str("Instance", instance).Msg("Unknown git instance")
		return fmt.Errorf("unknown git instance: %s", instance), FormulaInfo{}
	}
	if err != nil {
		l.Error().Stack().Err(err).Msg("error creating private gitlab api client")
		return fmt.Errorf("error creating private gitlab api client: %w", err), FormulaInfo{}
	}
	newUrl := ""
	c := 0
	newVersion := ""
	isMaster, err := formula.IsOnMaster()
	if err != nil {
		l.Error().Err(err).Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Msg("error checking if new version is on-master")
	}

	if !isMaster {
		newVersion, err = gitClient.GetLatestVersion(formula.Homepage, isMaster)
		if err != nil {
			l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Err(err).Msg("error getting new version")
			return fmt.Errorf("error getting new version: %w", err), FormulaInfo{}
		}
		newVersion = strings.TrimPrefix(newVersion, "v")
		newUrl = formula.GetNewVersionURL(newVersion)
	} else {
		newUrl = formula.URL
		newVersion, err = gitClient.GetMasterVersionId(formula.Homepage)
		if err != nil {
			l.Error().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Stack().Err(err).Msg("error getting new version from master")
			return fmt.Errorf("error getting new version from master: %w", err), FormulaInfo{}
		}
	}
	l.Debug().Str("New", newVersion).Str("Old", formula.Version).Str("Url", formula.Homepage).Msg("Comparing versions...")
	c, err = CompareVersions(formula.Version, newVersion, isMaster)
	if err != nil {
		l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("error comparing versions")
	}
	if c == 0 {
		if config.Force {
			l.Trace().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("Version", formula.Version).Msg("No new version, but force option enabled, continuing...")
		} else {
			l.Trace().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("Version", formula.Version).Msg("No new version, skipping")
			return nil, FormulaInfo{}
		}
	}
	if c > 0 {
		if config.Force {
			l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, but force option enabled, continuing...")
		} else {
			l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, skipping...")
			return nil, FormulaInfo{}
		}
	}
	l.Info().Str("New", newVersion).Str("Old", formula.Version).Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Msg("New version found")
	newReleaseArchive, err := gitClient.HttpGet(newUrl, config.Tokens[gitDomain])
	if err != nil {
		l.Warn().Err(err).Str("Url", newUrl).Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Msg("error downloading release")
	}

	description, err := gitClient.GetDescription(formula.Homepage)
	if err != nil {
		l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Err(err).Msg("error getting description")
	}
	newSha256 := Sha256(newReleaseArchive)
	formula.Version = newVersion
	formula.SHA256 = newSha256
	formula.Description = description
	err = formula.Update()
	if err != nil {
		l.Warn().Str("Formula", formula.GetLocalFile()).Str("Url", formula.Homepage).Str("File", formula.File).Err(err).Msg("error updating formula")
	}
	return nil, formula
}

func HandleGit(config Config, path string, l zerolog.Logger) error {
	gitDir, err := GetContainingDir(path)
	if err != nil {
		l.Error().Stack().Err(err).Msg("Error getting directory, skipping...")
		return err
	}

	isGit, err := IsInsideGitWorkTree(gitDir)
	if !isGit {
		l.Info().Str("Path", gitDir).Msg("Not a git repository, skipping...")
		return err
	}
	if err != nil {
		l.Info().Str("Path", gitDir).Err(err).Msg("Error checking if path is a git repository, skipping...")
	}

	git := NewGit(config.Interactive, gitDir)

	l.Info().Str("Path", gitDir).Msg("Running git -p")
	err = git.Add(config)
	if err != nil {
		l.Error().Stack().Err(err).Msg("error adding files to staged commit")
		return err
	}

	l.Info().Str("Path", gitDir).Msg("Commiting files")
	err = git.CommitFiles()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error committing files")
		return err
	}

	if !config.NoPush {
		l.Info().Str("Path", gitDir).Msg("Pushing...")
		err = git.Push()
		if err != nil {
			l.Error().Stack().Err(err).Msg("error pushing files")
			return err
		}
	}

	l.Info().Str("Path", gitDir).Msg("Done!")
	return nil
}
