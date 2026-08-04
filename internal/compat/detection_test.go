package compat

import "testing"

// TestMatchesVersion guards the version matcher against segment over-matching.
// The docker_29_force_close_blobs workaround is scoped to 29.0-29.2 only; a raw
// string-prefix wildcard would leak it onto 29.10-29.29, causing the very
// "short copy" errors the workaround's comment warns about.
func TestMatchesVersion(t *testing.T) {
	cases := []struct {
		version string
		major   int
		minor   int
		pattern string
		want    bool
	}{
		// exact
		{"29.2.0", 29, 2, "29.2.0", true},

		// single-segment wildcard matches any minor/patch under the major
		{"29.5.1", 29, 5, "29.*", true},
		{"18.09.1", 18, 9, "18.*", true},
		{"3.4.0", 3, 4, "29.*", false},

		// two-segment wildcard matches only that exact minor line
		{"29.1.3", 29, 1, "29.1.*", true},
		{"29.2.0", 29, 2, "29.2.*", true},

		// the bug: double-digit minors must NOT match a single-digit-minor pattern
		{"29.10.5", 29, 10, "29.1.*", false},
		{"29.19.0", 29, 19, "29.1.*", false},
		{"29.20.0", 29, 20, "29.2.*", false},
		{"29.29.9", 29, 29, "29.2.*", false},

		// 29.3+ excluded from the 29.0-29.2 force-close range
		{"29.3.0", 29, 3, "29.2.*", false},
		{"29.0.1", 29, 0, "29.0.*", true},

		// major-only pattern
		{"29.2.0", 29, 2, "29", true},
		{"28.4.0", 28, 4, "29", false},
	}

	for _, c := range cases {
		info := &ClientInfo{Version: c.version, VersionMajor: c.major, VersionMinor: c.minor}
		if got := info.MatchesVersion(c.pattern); got != c.want {
			t.Errorf("MatchesVersion(version=%q, pattern=%q) = %v, want %v", c.version, c.pattern, got, c.want)
		}
	}
}
