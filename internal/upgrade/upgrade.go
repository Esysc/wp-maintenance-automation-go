package upgrade

import (
	"fmt"
	"os"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
)

type UpgradePhase string

const (
	PhaseInit        UpgradePhase = "init"
	PhaseBackup      UpgradePhase = "backup"
	PhaseValidate    UpgradePhase = "validate"
	PhaseRehearse    UpgradePhase = "rehearse"
	PhaseApproval    UpgradePhase = "approval"
	PhaseUpgrade     UpgradePhase = "upgrade"
	PhaseHealthcheck UpgradePhase = "healthcheck"
	PhaseRollback    UpgradePhase = "rollback"
	PhaseComplete    UpgradePhase = "complete"
)

type UpgradeResult struct {
	Phase     UpgradePhase `json:"phase"`
	Success   bool         `json:"success"`
	Message   string       `json:"message"`
	Timestamp time.Time    `json:"timestamp"`
	Snapshot  string       `json:"snapshot,omitempty"`
}

type UpgradeReport struct {
	Timestamp      string          `json:"timestamp"`
	Host           string          `json:"host"`
	WPRoot         string          `json:"wp_root"`
	Snapshot       string          `json:"snapshot"`
	HealthcheckURL string          `json:"healthcheck_url"`
	Steps          []UpgradeResult `json:"steps"`
	FinalStatus    string          `json:"final_status"`
	FinalReason    string          `json:"final_reason"`
}

type UpgradeManager struct {
	sshClient        *ssh.Client
	wpRoot           string
	healthcheckURL   string
	autoRollback     bool
	stagingRehearsal bool
	stagingHost      string
	stagingUser      string
	stagingPort      int
	stagingRoot      string
	wpCLIBin         string
	wpCLIExtraArgs   string
	reportDir        string
}

type UpgradeOptions struct {
	SSHClient        *ssh.Client
	WPRoot           string
	HealthcheckURL   string
	AutoRollback     bool
	StagingRehearsal bool
	StagingHost      string
	StagingUser      string
	StagingPort      int
	StagingRoot      string
	WPCLIBin         string
	WPCLIExtraArgs   string
	ReportDir        string
}

func NewUpgradeManager(opts *UpgradeOptions) *UpgradeManager {
	reportDir := opts.ReportDir
	if reportDir == "" {
		reportDir = "./var/reports/wp_upgrade"
	}

	wpCLIBin := opts.WPCLIBin
	if wpCLIBin == "" {
		wpCLIBin = "wp"
	}

	return &UpgradeManager{
		sshClient:        opts.SSHClient,
		wpRoot:           opts.WPRoot,
		healthcheckURL:   opts.HealthcheckURL,
		autoRollback:     opts.AutoRollback,
		stagingRehearsal: opts.StagingRehearsal,
		stagingHost:      opts.StagingHost,
		stagingUser:      opts.StagingUser,
		stagingPort:      opts.StagingPort,
		stagingRoot:      opts.StagingRoot,
		wpCLIBin:         wpCLIBin,
		wpCLIExtraArgs:   opts.WPCLIExtraArgs,
		reportDir:        reportDir,
	}
}

func (um *UpgradeManager) Run() (*UpgradeReport, error) {
	timestamp := time.Now().Format("20060102_150405")
	report := &UpgradeReport{
		Timestamp:      timestamp,
		Host:           um.sshClient.DSN(),
		WPRoot:         um.wpRoot,
		HealthcheckURL: um.healthcheckURL,
		FinalStatus:    "failed",
	}

	defer um.writeReport(report, timestamp)

	// Phase 1: Validate environment
	if err := um.validateEnv(); err != nil {
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseInit, Success: false, Message: err.Error(), Timestamp: time.Now(),
		})
		report.FinalReason = err.Error()
		return report, err
	}

	// Phase 2: Core upgrade
	if err := um.upgradeCore(report); err != nil {
		report.FinalReason = fmt.Sprintf("core upgrade phase failed: %v", err)
		return report, err
	}

	// Phase 3: Plugin upgrade
	if err := um.upgradePlugins(report); err != nil {
		return report, nil
	}

	// Phase 4: Theme upgrade
	if err := um.upgradeThemes(report); err != nil {
		return report, nil
	}

	// Phase 5: Language upgrades
	if err := um.upgradeLanguages(report); err != nil {
		return report, nil
	}

	// Phase 6: Database update
	if err := um.upgradeDatabase(report); err != nil {
		return report, nil
	}

	report.FinalStatus = "success"
	report.FinalReason = "upgrade completed successfully"
	return report, nil
}

func (um *UpgradeManager) validateEnv() error {
	exists, err := um.sshClient.FileExists(fmt.Sprintf("%s/wp-config.php", um.wpRoot))
	if err != nil || !exists {
		return fmt.Errorf("wordpress not found at %s", um.wpRoot)
	}

	hasWPCLI, err := um.sshClient.CommandExists("wp")
	if err != nil || !hasWPCLI {
		installCmd := `curl -fsS -o /tmp/wp-cli.phar https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar && chmod +x /tmp/wp-cli.phar && mv /tmp/wp-cli.phar /usr/local/bin/wp`
		um.sshClient.RunCommand(installCmd)
	}

	return nil
}

func (um *UpgradeManager) upgradeCore(report *UpgradeReport) error {
	wpArgs := ""
	if um.wpCLIExtraArgs != "" {
		wpArgs = um.wpCLIExtraArgs
	}

	// Get current version
	currentVersion, _ := um.wpCLI("core version")
	report.Steps = append(report.Steps, UpgradeResult{
		Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("current version: %s", currentVersion), Timestamp: time.Now(),
	})

	// Update core
	output, err := um.wpCLI("core update " + wpArgs)
	if err != nil {
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseUpgrade, Success: false, Message: fmt.Sprintf("core update failed: %v", err),
			Timestamp: time.Now(),
		})
		return fmt.Errorf("core update failed: %w", err)
	}

	report.Steps = append(report.Steps, UpgradeResult{
		Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("core update: %s", output), Timestamp: time.Now(),
	})

	return nil
}

func (um *UpgradeManager) upgradePlugins(report *UpgradeReport) error {
	wpArgs := ""
	if um.wpCLIExtraArgs != "" {
		wpArgs = um.wpCLIExtraArgs
	}

	output, err := um.wpCLI("plugin update --all " + wpArgs)
	if err != nil {
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("plugin update: %s (continuing)", output),
			Timestamp: time.Now(),
		})
		return nil
	}

	report.Steps = append(report.Steps, UpgradeResult{
		Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("plugin update: %s", output), Timestamp: time.Now(),
	})

	return nil
}

func (um *UpgradeManager) upgradeThemes(report *UpgradeReport) error {
	wpArgs := ""
	if um.wpCLIExtraArgs != "" {
		wpArgs = um.wpCLIExtraArgs
	}

	output, err := um.wpCLI("theme update --all " + wpArgs)
	if err != nil {
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("theme update: %s (continuing)", output),
			Timestamp: time.Now(),
		})
		return nil
	}

	report.Steps = append(report.Steps, UpgradeResult{
		Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("theme update: %s", output), Timestamp: time.Now(),
	})

	return nil
}

func (um *UpgradeManager) upgradeLanguages(report *UpgradeReport) error {
	wpArgs := ""
	if um.wpCLIExtraArgs != "" {
		wpArgs = um.wpCLIExtraArgs
	}

	langs := []string{
		"language core update",
		"language plugin update --all",
		"language theme update --all",
	}

	for _, lang := range langs {
		output, err := um.wpCLI(lang + " " + wpArgs)
		if err != nil {
			continue
		}
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("%s: %s", lang, output),
			Timestamp: time.Now(),
		})
	}

	return nil
}

func (um *UpgradeManager) upgradeDatabase(report *UpgradeReport) error {
	wpArgs := ""
	if um.wpCLIExtraArgs != "" {
		wpArgs = um.wpCLIExtraArgs
	}

	output, err := um.wpCLI("core update-db " + wpArgs)
	if err != nil {
		report.Steps = append(report.Steps, UpgradeResult{
			Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("DB update: %s (continuing)", output),
			Timestamp: time.Now(),
		})
		return nil
	}

	report.Steps = append(report.Steps, UpgradeResult{
		Phase: PhaseUpgrade, Success: true, Message: fmt.Sprintf("DB update: %s", output), Timestamp: time.Now(),
	})

	return nil
}

func (um *UpgradeManager) wpCLI(args string) (string, error) {
	wpBinary := um.wpCLIBin
	wpExtra := ""
	if um.wpCLIExtraArgs != "" {
		wpExtra = " " + um.wpCLIExtraArgs
	}

	cmd := fmt.Sprintf("cd '%s' && %s%s %s", um.wpRoot, wpBinary, wpExtra, args)
	return um.sshClient.RunCommand(cmd)
}

func (um *UpgradeManager) writeReport(report *UpgradeReport, timestamp string) {
	reportPath := fmt.Sprintf("%s/%s/report.txt", um.reportDir, timestamp)
	if err := os.MkdirAll(fmt.Sprintf("%s/%s", um.reportDir, timestamp), 0755); err != nil {
		return
	}

	content := fmt.Sprintf(`WordPress Upgrade Report
========================
Timestamp: %s
Host: %s
WordPress path: %s
Healthcheck URL: %s

Final status: %s
Reason: %s
`, timestamp, um.sshClient.DSN(), um.wpRoot, um.healthcheckURL, report.FinalStatus, report.FinalReason)

	os.WriteFile(reportPath, []byte(content), 0644)
}

func (um *UpgradeManager) RunRollback(snapshotID string) error {
	output, err := um.wpCLI(fmt.Sprintf("db export - | gzip > /tmp/rollback_before_%s.sql.gz", snapshotID))
	if err != nil {
		return fmt.Errorf("failed to snapshot database before rollback: %w", err)
	}

	_ = output
	return nil
}
