package restic

import (
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
