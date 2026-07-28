package restic

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"
)

type ResticClient struct {
	Repository   string
	PasswordFile string
	Password     string
	ExtraArgs    []string
}

type Snapshot struct {
	ID      string   `json:"id"`
	ShortID string   `json:"short_id"`
	Time    string   `json:"time"`
	Host    string   `json:"hostname"`
	Tags    []string `json:"tags"`
	Paths   []string `json:"paths"`
}

type BackupResult struct {
	SnapshotID  string `json:"snapshot_id"`
	MessageType string `json:"message_type"`
}

func NewClient(repository, passwordFile string) (*ResticClient, error) {
	if _, err := os.Stat(passwordFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("restic password file not found: %s", passwordFile)
	}

	return &ResticClient{
		Repository:   repository,
		PasswordFile: passwordFile,
	}, nil
}

func (r *ResticClient) newCmd(args ...string) *exec.Cmd {
	allArgs := append([]string{
		"--repo", r.Repository,
		"--password-file", r.PasswordFile,
	}, args...)

	return exec.Command("restic", allArgs...)
}

func (r *ResticClient) Backup(path string, tags []string) (*Snapshot, error) {
	args := []string{"backup", path, "--json"}

	for _, tag := range tags {
		if tag != "" {
			args = append(args, "--tag", tag)
		}
	}

	cmd := r.newCmd(args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		outputStr := string(output)
		log.Printf("restic backup exited with %v, attempting to parse snapshot from output", err)
		snap := tryParseSnapshot(outputStr, tags)
		if snap != nil {
			return snap, nil
		}
		return nil, fmt.Errorf("restic backup failed: %w\nOutput: %s", err, outputStr)
	}

	snap := tryParseSnapshot(string(output), tags)
	if snap != nil {
		return snap, nil
	}

	return nil, fmt.Errorf("restic backup produced no snapshot output")
}

func tryParseSnapshot(output string, tags []string) *Snapshot {
	lines := strings.Split(strings.TrimSpace(output), "\n")
	var lastLine string
	for _, line := range lines {
		if line != "" {
			lastLine = line
		}
	}
	if lastLine == "" {
		return nil
	}

	var backupResult BackupResult
	if err := json.Unmarshal([]byte(lastLine), &backupResult); err != nil {
		return nil
	}

	return &Snapshot{
		ID:   backupResult.SnapshotID,
		Tags: tags,
	}
}

func (r *ResticClient) Snapshots() ([]Snapshot, error) {
	cmd := r.newCmd("snapshots", "--json", "--compact")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("restic snapshots failed: %w\nOutput: %s", err, string(output))
	}

	var snapshots []Snapshot
	if err := json.Unmarshal(output, &snapshots); err != nil {
		return nil, fmt.Errorf("failed to parse snapshots: %w", err)
	}

	return snapshots, nil
}

func (r *ResticClient) Restore(snapshotID, targetDir string) error {
	cmd := r.newCmd("restore", snapshotID, "--target", targetDir)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic restore failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *ResticClient) Forget(flags ...string) error {
	args := []string{"forget", "--prune"}
	args = append(args, flags...)

	cmd := r.newCmd(args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic forget failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *ResticClient) Check(readDataSubset string) error {
	args := []string{"check"}

	if readDataSubset != "" {
		args = append(args, "--read-data-subset", readDataSubset)
	}

	cmd := r.newCmd(args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic check failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *ResticClient) Unlock() error {
	cmd := r.newCmd("unlock")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic unlock failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *ResticClient) Init() error {
	cmd := r.newCmd("init")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic init failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}

func (r *ResticClient) Stats() (map[string]interface{}, error) {
	cmd := r.newCmd("stats", "--json")
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("restic stats failed: %w\nOutput: %s", err, string(output))
	}

	var stats map[string]interface{}
	if err := json.Unmarshal(output, &stats); err != nil {
		return nil, fmt.Errorf("failed to parse stats: %w", err)
	}

	return stats, nil
}

func (r *ResticClient) Find(path string, tags []string) ([]Snapshot, error) {
	args := []string{"find", path, "--json"}

	for _, tag := range tags {
		if tag != "" {
			args = append(args, "--tag", tag)
		}
	}

	cmd := r.newCmd(args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("restic find failed: %w\nOutput: %s", err, string(output))
	}

	var snapshots []Snapshot
	if err := json.Unmarshal(output, &snapshots); err != nil {
		return nil, fmt.Errorf("failed to parse find results: %w", err)
	}

	return snapshots, nil
}

func buildTagArgs(snapshotID string, addTags []string) []string {
	args := []string{"tag"}

	var tags []string
	for _, tag := range addTags {
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	if len(tags) > 0 {
		args = append(args, fmt.Sprintf("--set=%s", strings.Join(tags, ",")))
	}

	return append(args, snapshotID)
}

func (r *ResticClient) Tag(snapshotID string, addTags []string, removeTags []string) error {
	args := buildTagArgs(snapshotID, addTags)

	cmd := r.newCmd(args...)
	cmd.Env = os.Environ()

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("restic tag failed: %w\nOutput: %s", err, string(output))
	}

	return nil
}
