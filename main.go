package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
)

type Config struct {
	Token   string
	Verbose bool
	Path    string
}

func main() {
	l := zerolog.New(os.Stderr).Output(zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: time.RFC3339}).With().Timestamp().Caller().Logger()
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
			l.Trace().Str("Formula", formula.Homepage).Str("Version", formula.Version).Msg("No new version, skipping")
			continue
		}
		if c > 0 {
			l.Warn().Str("Formula", formula.Homepage).Str("Current", formula.Version).Str("New", newVersion).Err(err).Msg("New version lower than current, skipping...")
			continue
		}
		l.Info().Str("New", newVersion).Str("Old", formula.Version).Str("Formula", formula.Homepage).Msg("New version found")
		newReleaseArchive, err := gitClient.HttpGet(newUrl, config.Token)

		if err != nil {
			l.Warn().Err(err).Str("Url", newUrl).Str("Formula", formula.Homepage).Msg("error downloading release")
		}
		newSha256 := Sha256(newReleaseArchive)
		formula.Version = newVersion
		formula.SHA256 = newSha256
		err = formula.Update()
		if err != nil {
			l.Warn().Str("Formula", formula.Homepage).Str("File", formula.File).Err(err).Msg("error updating formula")
		}
	}

}

func parseArgs() (Config, error) {
	c := Config{}
	// Define the --token flag; it's a required flag so we check it after parsing.
	token := flag.String("token", "", "The token value (required)")
	verbose := flag.Bool("v", false, "verbose mode")

	// Parse the flags from the command line.
	flag.Parse()

	// Validate that the required token flag was provided.
	if *token == "" {
		return Config{}, fmt.Errorf("token missing")
	}
	c.Token = *token
	c.Verbose = *verbose

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
