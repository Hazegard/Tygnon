package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

const APPNAME = "tygnon"

func GetRoot() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return ""
	}
	return filepath.Dir(file)
}

func main() {
	root := GetRoot()
	l := zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Caller().Logger()
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		return fmt.Sprintf("%s:%d", strings.TrimPrefix(file, root+"/"), line)
	}
	l = l.Level(zerolog.InfoLevel)
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

	formulaFiles, err := FindBrewFormulaFiles(config.Path)
	if err != nil {
		l.Error().Stack().Err(err).Msg("error finding brew formula files")
		return
	}
	var formulas []FormulaInfo
	for _, f := range formulaFiles {
		formula, err := ParseFormula(f)
		if err != nil {
			l.Error().Stack().Err(err).Msg("error parsing formula")
			continue
		}
		formulas = append(formulas, formula)
	}
	if len(formulas) == 0 {
		l.Warn().Str("Path", config.Path).Msg("no formula files found, aborting")
		return
	}

	var updatedFormulas []string
	for _, formula := range formulas {
		l.Info().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Msg("Checking new version...")
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
			continue
		}
		if err != nil {
			l.Error().Stack().Err(err).Msg("error creating private gitlab api client")
			return
		}
		newUrl := ""
		c := 0
		newVersion := ""
		isMaster, err := formula.IsOnMaster()
		if err != nil {
			l.Error().Err(err).Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Msg("error checking if new version is on-master")
		}

		if !isMaster {
			newVersion, err = gitClient.GetLatestVersion(formula.Homepage)
			if err != nil {
				l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Err(err).Msg("error getting new version")
				continue
			}
			newVersion = strings.TrimPrefix(newVersion, "v")
			newUrl = formula.GetNewVersionURL(newVersion)
			c, err = CompareVersions(formula.Version, newVersion)
			if err != nil {
				l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("error comparing versions")
			}
		} else {
			newUrl = formula.URL
			newVersion, err = gitClient.GetMasterVersionId(formula.Homepage)
			if err != nil {
				l.Error().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Stack().Err(err).Msg("error getting new version from master")
			}
			c, err = CompareVersions(formula.Version, newVersion)
			if err != nil {
				l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("error comparing versions")
			}
		}
		if c == 0 {
			if config.Force {
				l.Trace().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Version", formula.Version).Msg("No new version, but force option enabled, continuing...")
			} else {
				l.Trace().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Version", formula.Version).Msg("No new version, skipping")
				continue
			}
		}
		if c > 0 {
			if config.Force {
				l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, but force option enabled, continuing...")
			} else {
				l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, skipping...")
				continue
			}
		}
		l.Info().Str("New", newVersion).Str("Old", formula.Version).Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Msg("New version found")
		newReleaseArchive, err := gitClient.HttpGet(newUrl, config.Tokens[gitDomain])
		if err != nil {
			l.Warn().Err(err).Str("Url", newUrl).Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Msg("error downloading release")
		}

		description, err := gitClient.GetDescription(formula.Homepage)
		if err != nil {
			l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Err(err).Msg("error getting description")
		}
		newSha256 := Sha256(newReleaseArchive)
		formula.Version = newVersion
		formula.SHA256 = newSha256
		formula.Description = description
		err = formula.Update()
		if err != nil {
			l.Warn().Str("Formula", formula.GetLocalFile(config)).Str("Url", formula.Homepage).Str("File", formula.File).Err(err).Msg("error updating formula")
		}
		updatedFormulas = append(updatedFormulas, strings.TrimPrefix(formula.File, config.Path))
	}

	if len(updatedFormulas) == 0 {
		l.Info().Str("Path", config.Path).Msg("No new versions found")
		return
	}
	l.Warn().Str("Path", config.Path).Msg("Updated formula files:")

	for _, updatedFormula := range updatedFormulas {
		l.Warn().Msgf("  - %s", updatedFormula)
	}

	git := NewGit(config)

	l.Info().Str("Path", config.Path).Msg("Running git -p")
	err = git.Add(config)
	if err != nil {
		l.Error().Stack().Err(err).Msg("error adding files to staged commit")
		return
	}

	l.Info().Str("Path", config.Path).Msg("Commiting files")
	err = git.CommitFiles()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error committing files")
		return
	}

	if !config.NoPush {
		l.Info().Str("Path", config.Path).Msg("Pushing...")
		err = git.Push()
		if err != nil {
			l.Error().Stack().Err(err).Msg("error pushing files")
			return
		}
	}

	l.Info().Str("Path", config.Path).Msg("Done!")
}
