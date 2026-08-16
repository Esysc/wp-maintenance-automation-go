package restic

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildTagArgs(t *testing.T) {
	tests := []struct {
		name       string
		snapshotID string
		addTags    []string
		want       []string
	}{
		{
			name:       "joins non-empty tags into set flag",
			snapshotID: "snapshot-123",
			addTags:    []string{"daily", "prod"},
			want:       []string{"tag", "--set=daily,prod", "snapshot-123"},
		},
		{
			name:       "skips empty tags",
			snapshotID: "snapshot-123",
			addTags:    []string{"", "daily", "", "prod"},
			want:       []string{"tag", "--set=daily,prod", "snapshot-123"},
		},
		{
			name:       "allows empty set",
			snapshotID: "snapshot-123",
			addTags:    nil,
			want:       []string{"tag", "snapshot-123"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildTagArgs(tt.snapshotID, tt.addTags)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("buildTagArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTryParseSnapshot(t *testing.T) {
	tests := []struct {
		name      string
		output    string
		tags      []string
		wantID    string
		wantShort string
		wantNil   bool
	}{
		{
			name:      "parses snapshot from final json line",
			output:    "[INFO] backup started\n{\"snapshot_id\":\"abcdef1234567890\"}",
			tags:      []string{"daily", "prod"},
			wantID:    "abcdef1234567890",
			wantShort: "abcdef12",
		},
		{
			name:    "returns nil for invalid json",
			output:  "[INFO] backup started\nnot-json",
			tags:    []string{"daily"},
			wantNil: true,
		},
		{
			name:    "returns nil for empty output",
			output:  "\n\n",
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tryParseSnapshot(tt.output, tt.tags)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("tryParseSnapshot() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("tryParseSnapshot() = nil, want snapshot")
			}
			if got.ID != tt.wantID {
				t.Fatalf("tryParseSnapshot().ID = %q, want %q", got.ID, tt.wantID)
			}
			if got.ShortID != tt.wantShort {
				t.Fatalf("tryParseSnapshot().ShortID = %q, want %q", got.ShortID, tt.wantShort)
			}
			if !reflect.DeepEqual(got.Tags, tt.tags) {
				t.Fatalf("tryParseSnapshot().Tags = %v, want %v", got.Tags, tt.tags)
			}
		})
	}
}

func TestNewClient(t *testing.T) {
	t.Run("missing password file", func(t *testing.T) {
		_, err := NewClient("/repo", filepath.Join(t.TempDir(), "missing"))
		if err == nil {
			t.Fatal("NewClient() error = nil, want error")
		}
	})

	t.Run("existing password file", func(t *testing.T) {
		passwordFile := filepath.Join(t.TempDir(), "restic-pass")
		if err := os.WriteFile(passwordFile, []byte("secret"), 0600); err != nil {
			t.Fatalf("failed to create password file: %v", err)
		}

		client, err := NewClient("/repo", passwordFile)
		if err != nil {
			t.Fatalf("NewClient() error = %v, want nil", err)
		}
		if client.Repository != "/repo" {
			t.Fatalf("NewClient().Repository = %q, want %q", client.Repository, "/repo")
		}
		if client.PasswordFile != passwordFile {
			t.Fatalf("NewClient().PasswordFile = %q, want %q", client.PasswordFile, passwordFile)
		}
	})
}
