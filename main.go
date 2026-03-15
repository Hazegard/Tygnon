package main

import (
	"fmt"
	"os"
	"strings"
)

const token = ""

func main() {
	formulaDir := ""
	if len(os.Args) < 2 {
		formulaDir = "."
	} else {
		formulaDir = os.Args[1]
	}

	formulaFiles, err := FindBrewFormulaFiles(formulaDir)
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

	gitlabClient, err := NewGitlabApi(token, "https://git.example.fr")
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
		newReleaseArchive, err := HttpGet(newUrl, token)
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
