package staging

import (
	"os/exec"
	"strings"
	"testing"
)

func TestStagingProjectName(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		want       string
	}{
		{name: "long hex id", snapshotID: "abcdef1234567890", want: "wps-abcdef12"},
		{name: "short id", snapshotID: "abc", want: "wps-abc"},
		{name: "uppercase and separators", snapshotID: "ABC_123!", want: "wps-abc123"},
		{name: "leading and trailing whitespace", snapshotID: "  deadbeef  ", want: "wps-deadbeef"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := stagingProjectName(tt.snapshotID); got != tt.want {
				t.Fatalf("stagingProjectName(%q) = %q, want %q", tt.snapshotID, got, tt.want)
			}
		})
	}

	t.Run("empty id still produces a valid name", func(t *testing.T) {
		got := stagingProjectName("")
		if !strings.HasPrefix(got, "wps-") || len(got) <= len("wps-") {
			t.Fatalf("stagingProjectName(%q) = %q, want non-empty wps- prefix", "", got)
		}
	})
}

func TestQuoteMySQLIdentifier(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "wp_", want: "`wp_`"},
		{in: "weird`name", want: "`weird``name`"},
	}

	for _, tt := range tests {
		if got := quoteMySQLIdentifier(tt.in); got != tt.want {
			t.Errorf("quoteMySQLIdentifier(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestQuoteMySQLString(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "plain", want: "'plain'"},
		{in: "O'Brien", want: "'O''Brien'"},
		{in: `back\slash`, want: `'back\\slash'`},
		{in: "line\nbreak", want: `'line\nbreak'`},
	}

	for _, tt := range tests {
		if got := quoteMySQLString(tt.in); got != tt.want {
			t.Errorf("quoteMySQLString(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestRemoveWPConfigConstantsScript(t *testing.T) {
	sedPath, err := exec.LookPath("sed")
	if err != nil {
		t.Skip("sed is not available")
	}

	input := `<?php
define('WP_HOME', 'http://old.example.com');
  define( 'WP_SITEURL', 'http://old.example.com' );
define("WP_HOME", "http://old.example.com");
define('DB_NAME', 'wordpress');
// define('WP_HOME', 'http://commented.example.com');
`
	cmd := exec.Command(sedPath, removeWPConfigConstantsScript)
	cmd.Stdin = strings.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("sed failed: %v", err)
	}

	got := string(out)
	if strings.Contains(got, "old.example.com") {
		t.Fatalf("active WP_HOME/WP_SITEURL definitions were not removed:\n%s", got)
	}
	if !strings.Contains(got, "define('DB_NAME', 'wordpress');") {
		t.Fatalf("unrelated define was removed:\n%s", got)
	}
	if !strings.Contains(got, "// define('WP_HOME'") {
		t.Fatalf("commented definition was removed:\n%s", got)
	}
}

func TestNormalizePHPVersion(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: "8.3.27", want: "8.3"},
		{in: "PHP 8.2.35", want: "8.2"},
		{in: " 8.4 ", want: "8.4"},
		{in: "8", want: ""},
		{in: "8.x", want: ""},
		{in: "5.6.40", want: ""},
		{in: "", want: ""},
	}

	for _, tt := range tests {
		if got := NormalizePHPVersion(tt.in); got != tt.want {
			t.Errorf("NormalizePHPVersion(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}
