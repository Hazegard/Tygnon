package main

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/rs/zerolog"
)

type Config struct {
	Tokens      map[string]string `name:"token" short:"T" help:"Personal access token" env:""`
	Verbose     int               `name:"verbose" short:"v" optional:"true" help:"verbose output" type:"counter" default:"0" env:""`
	Force       bool              `name:"force" short:"f" optional:"true" help:"force overwriting formulas" env:""`
	Path        string            `arg:"" optional:"" help:"path to project directory" default:"."`
	Interactive bool              `name:"interactive" negatable:"" short:"i" help:"interactive mode" default:"true"`
	NoPush      bool              `name:"no-push" help:"disable git push" default:"false"`
}

const APPNAME = "tygnon"

func main() {
	l := zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Caller().Logger()
	zerolog.CallerMarshalFunc = func(pc uintptr, file string, line int) string {
		p := strings.Split(file, string(filepath.Separator))
		var result []string
		for i := len(p) - 1; i >= 0; i-- {
			if strings.HasPrefix(strings.ToLower(p[i]), APPNAME) {
				break
			}
			result = append(result, p[i])
		}
		slices.Reverse(result)
		return filepath.Join(result...) + ":" + strconv.Itoa(line)
	}
	l = l.Level(zerolog.InfoLevel)
	config, err := parseArgs()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error parsing cli arguments")
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
		case XmGitlab:
			gitClient, err = NewGitlabApi(config.Tokens[gitDomain], fmt.Sprintf("https://%s", gitDomain))
		case Gitlab:
			gitClient, err = NewGitlabApi(config.Tokens[gitDomain], fmt.Sprintf("https://%s", gitDomain))
		case Github:
			gitClient = NewGithubApi()
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

func parseArgs() (Config, error) {
	c := Config{}

	kongOptions := []kong.Option{
		kong.Name(APPNAME),
		kong.Description("Application used to bump releases on internal brew tap"),
		kong.UsageOnError(),
		kong.DefaultEnvars(strings.ToUpper(APPNAME)),
	}
	_ = kong.Parse(&c, kongOptions...)
	return c, nil
}
