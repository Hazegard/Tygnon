package main

import "testing"

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		name     string
		current  string
		new      string
		expected int // -1: current < new, 0: equal, 1: current > new
	}{
		{"semver less", "1.2.3", "1.2.4", -1},
		{"semver greater", "1.2.4", "1.2.3", 1},
		{"semver equal", "1.2.3", "1.2.3", 0},
		{"semver major bump", "1.9.9", "2.0.0", -1},
		{"semver v-prefixed", "v1.0.0", "v1.0.1", -1},
		{"branch tracking, commit count increased", "123.abcd123", "124.def4567", -1},
		{"branch tracking, commit count decreased", "124.def4567", "123.abcd123", 1},
		{"branch tracking, same commit count", "123.abcd123", "123.efgh456", 0},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := CompareVersions(c.current, c.new)
			if sign(got) != c.expected {
				t.Errorf("CompareVersions(%q, %q) = %d, want sign %d", c.current, c.new, got, c.expected)
			}
		})
	}
}

func sign(i int) int {
	switch {
	case i < 0:
		return -1
	case i > 0:
		return 1
	default:
		return 0
	}
}

func TestCompareVersionsBourrin(t *testing.T) {
	cases := []struct {
		v1, v2   string
		expected int
	}{
		{"1.2.3", "1.2.4", -1},
		{"1.2.4", "1.2.3", 1},
		{"1.2.3", "1.2.3", 0},
		{"1.2", "1.2.0", 0},
		{"1.10.0", "1.9.0", 1}, // numeric, not lexicographic, comparison
		{"1.0.0-beta", "1.0.0-alpha", 0},
	}

	for _, c := range cases {
		t.Run(c.v1+"_vs_"+c.v2, func(t *testing.T) {
			got := CompareVersionsBourrin(c.v1, c.v2)
			if sign(got) != c.expected {
				t.Errorf("CompareVersionsBourrin(%q, %q) = %d, want sign %d", c.v1, c.v2, got, c.expected)
			}
		})
	}
}

func TestCompareVersionsSemverError(t *testing.T) {
	if _, err := CompareVersionsSemver("not-a-version", "1.0.0"); err == nil {
		t.Fatalf("expected an error parsing a non-semver current version")
	}
	if _, err := CompareVersionsSemver("1.0.0", "not-a-version"); err == nil {
		t.Fatalf("expected an error parsing a non-semver new version")
	}
}
