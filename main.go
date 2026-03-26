package main

import (
	"flag"
	"fmt"
	"github.com/rs/zerolog/log"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Config struct {
	Token   string
	Verbose bool
	Force   bool
	Path    string
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
	if config.Verbose {
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
	//for _, formula := range formulas {
	//	fmt.Printf("%+v\n", formula)
	//}

	xmgitlab, err := NewGitlabApi(config.Token, "https://git.example.fr")
	if err != nil {
		l.Error().Stack().Err(err).Msg("error creating private gitlab api client")
		return
	}

	gitlab, err := NewGitlabApi("", "https://gitlab.com")
	if err != nil {
		l.Error().Stack().Err(err).Msg("error creating gitlab api client")
		return
	}

	github := NewGithubApi()

	for _, formula := range formulas {
		l.Info().Str("Formula", formula.Homepage).Msg("Checking new version...")
		var gitClient GitApi
		switch formula.GitInstance() {
		case XmGitlab:
			gitClient = xmgitlab
		case Gitlab:
			gitClient = gitlab
		case Github:
			gitClient = github
		default:
			instance, _ := formula.GetInstance()
			l.Info().Str("Instance", instance).Msg("Unknown git instance")
			continue
		}

		newVersion, err := gitClient.GetLatestVersion(formula.Homepage)
		if err != nil {
			l.Warn().Str("Formula", formula.Homepage).Err(err).Msg("error getting new version")
			continue
		}
		newVersion = strings.TrimPrefix(newVersion, "v")
		newUrl := formula.GetNewVersionURL(newVersion)
		c, err := CompareVersions(formula.Version, newVersion)
		if err != nil {
			l.Warn().Str("Formula", formula.Version).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("error comparing versions")
		}
		if c == 0 {
			if config.Force {
				l.Trace().Str("Formula", formula.Homepage).Str("Version", formula.Version).Msg("No new version, but force option enabled, continuing...")
			} else {
				l.Trace().Str("Formula", formula.Homepage).Str("Version", formula.Version).Msg("No new version, skipping")
				continue
			}
		}
		if c > 0 {
			if config.Force {
				l.Warn().Str("Formula", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, but force option enabled, continuing...")
			} else {
				l.Warn().Str("Formula", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, skipping...")
				continue
			}
		}
		l.Info().Str("New", newVersion).Str("Old", formula.Version).Str("Formula", formula.Homepage).Msg("New version found")
		newReleaseArchive, err := gitClient.HttpGet(newUrl, config.Token)

		if err != nil {
			l.Warn().Err(err).Str("Url", newUrl).Str("Formula", formula.Homepage).Msg("error downloading release")
		}

		description, err := gitClient.GetDescription(formula.Homepage)
		if err != nil {
			l.Warn().Str("Formula", formula.Homepage).Err(err).Msg("error getting description")
		}
		newSha256 := Sha256(newReleaseArchive)
		formula.Version = newVersion
		formula.SHA256 = newSha256
		formula.Description = description
		err = formula.Update()
		if err != nil {
			l.Warn().Str("Formula", formula.Homepage).Str("File", formula.File).Err(err).Msg("error updating formula")
		}
	}

	git := NewGit(&l, config)

	log.Info().Str("Path", config.Path).Msg("Running git -p")
	err = git.AddP()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error adding files to staged commit")
		return
	}

	log.Info().Str("Path", config.Path).Msg("Commiting files")
	err = git.CommitFiles()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error committing files")
		return
	}

	log.Info().Str("Path", config.Path).Msg("Pushing...")
	err = git.Push()
	if err != nil {
		l.Error().Stack().Err(err).Msg("error pushing files")
		return
	}

	log.Info().Str("Path", config.Path).Msg("Done!")
}

func parseArgs() (Config, error) {
	c := Config{}
	// Define the --token flag; it's a required flag so we check it after parsing.
	token := flag.String("token", "", "The token value (required)")
	verbose := flag.Bool("v", false, "verbose mode")
	force := flag.Bool("force", false, "force overwrite")

	// Parse the flags from the command line.
	flag.Parse()

	// Validate that the required token flag was provided.
	if *token == "" {
		return Config{}, fmt.Errorf("token missing")
	}
	c.Token = *token
	c.Verbose = *verbose
	c.Force = *force

	// Handle optional positional argument.
	// All non-flag arguments are returned by flag.Args().
	var positionalArg string
	args := flag.Args()
	if len(args) > 0 {
		positionalArg = args[0]
	}

	if positionalArg != "" {
		c.Path = positionalArg
	} else {
		c.Path = "."
	}
	return c, nil
}
