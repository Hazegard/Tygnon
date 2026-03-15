package main

import (
	"fmt"
	"github.com/Masterminds/semver/v3"
)

func CompareVersions(current, _new string) (int, error) {
	currentVersion, err := semver.NewVersion(current)
	if err != nil {
		return 0, fmt.Errorf("error parsing current version: %s", err)
	}
	newVersion, err := semver.NewVersion(_new)
	if err != nil {
		return 0, fmt.Errorf("error parsing new version: %s", err)
	}
	return currentVersion.Compare(newVersion), nil
}
