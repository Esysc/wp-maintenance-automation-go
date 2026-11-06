package version

import (
	"regexp"
	"testing"
)

func TestVersionIsValid(t *testing.T) {
	if Version == "" {
		t.Fatal("Version is empty")
	}
	if !regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+([.-][0-9A-Za-z.-]+)?$`).MatchString(Version) {
		t.Fatalf("Version = %q, want a semantic version", Version)
	}
}
