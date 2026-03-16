package main

import (
	"flag"
	"fmt"
	"strings"
)

type Config struct {
	Token string
	Path  string
}

func main() {
	config, err := parseArgs()
	if err != nil {
		fmt.Println(err)
		return
	}

	formulaFiles, err := FindBrewFormulaFiles(config.Path)
	if err != nil {
		fmt.Println(err)
		return
	}
	var formulas []FormulaInfo
	for _, f := range formulaFiles {
		formula, err := ParseFormula(f)
		if err != nil {
			fmt.Println(err)
			continue
		}
		formulas = append(formulas, formula)
	}
	//for _, formula := range formulas {
	//	fmt.Printf("%+v\n", formula)
	//}

	gitlabClient, err := NewGitlabApi(config.Token, "https://git.example.fr")
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, formula := range formulas {
		fmt.Printf("Trying %s...\n", formula.Homepage)
		newVersion := ""
		release, err := gitlabClient.GetLatestReleaseId(formula.Homepage)
		if err == nil {
			newVersion = release.TagName
		} else {
			tag, err := gitlabClient.GetLatestTagId(formula.Homepage)
			if err != nil {
				fmt.Printf("error getting latest tag (%s): %s\n", formula.Homepage, err)
				continue
			}
			newVersion = tag.Name
		}
		newVersion = strings.TrimPrefix(newVersion, "v")
		newUrl := formula.GetNewVersionURL(newVersion)
		c, err := CompareVersions(formula.Version, newVersion)
		if err != nil {
			fmt.Println("error comparing versions: ", err)
		}
		if c == 0 {
			fmt.Printf("No new version, skipping (%s)...\n", formula.Version)
			continue
		}
		if c > 0 {
			fmt.Printf("New version (%s) lower than current(%s), maybe an error, skipping...\n", newVersion, formula.Version)
			continue
		}
		newReleaseArchive, err := HttpGet(newUrl, config.Token)
		fmt.Println("New version found:", newReleaseArchive)

		if err != nil {
			fmt.Printf("error getting new release archive (%s): %s\n", newUrl, err)
		}
		newSha256 := Sha256(newReleaseArchive)
		formula.Version = newVersion
		formula.SHA256 = newSha256
		err = formula.Update()
		if err != nil {
			fmt.Println(err)
		}
	}

}

func parseArgs() (Config, error) {
	c := Config{}
	// Define the --token flag; it's a required flag so we check it after parsing.
	token := flag.String("token", "", "The token value (required)")

	// Parse the flags from the command line.
	flag.Parse()

	// Validate that the required token flag was provided.
	if *token == "" {
		return Config{}, fmt.Errorf("token missing")
	}
	c.Token = *token

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
