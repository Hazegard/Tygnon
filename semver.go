package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/Masterminds/semver/v3"
)

func CompareVersionsSemver(current, _new string) (int, error) {
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

func CompareVersions(current, _new string, onMaster bool) (int, error) {
	// i, err := CompareVersionsSemver(current, _new)
	// if err == nil && !(onMaster && i == 0) {
	// 	return i, nil
	// }
	i := CompareVersionsBourrin(current, _new)
	return i, nil
}

// CompareVersionsBourrin compares two version strings (e.g., "1.2.3" vs "1.2.4").
// It returns 1 if v1 is greater than v2, -1 if v1 is less than v2, and 0 if they are equal.
func CompareVersionsBourrin(v1, v2 string) int {
	parts1 := strings.FieldsFunc(v1, func(r rune) bool {
		return r == '.' || r == '-'
	})
	parts2 := strings.FieldsFunc(v2, func(r rune) bool {
		return r == '.' || r == '-'
	})

	// Determine the maximum number of parts
	maxLen := len(parts1)
	if len(parts2) > maxLen {
		maxLen = len(parts2)
	}

	for i := 0; i < maxLen; i++ {
		var num1, num2 int

		// If a version doesn't have this part, we assume it's zero.
		if i < len(parts1) {
			num1, _ = strconv.Atoi(parts1[i])
		}
		if i < len(parts2) {
			num2, _ = strconv.Atoi(parts2[i])
		}

		// Compare the numeric values.
		if num1 < num2 {
			return -1
		} else if num1 > num2 {
			return 1
		}
	}

	return 0 // Versions are equal
}
