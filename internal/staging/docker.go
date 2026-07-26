package staging

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/restic"
)

type Manager struct {
	buildContext string
}

type StagingEnv struct {
	SnapshotID   string
	RestoreDir   string
	ComposeFile  string
	ProjectName  string
	WPSPort      string
	HTTPSPort    string
	DBImage      string
	DBTag        string
	WPVersion    string
	PHPVersion   string
	DBName       string
	DBUser       string
	DBPassword   string
	TablePrefix  string
	HealthURL    string
	BuildContext string
}

func NewManager(buildContext string) *Manager {
	return &Manager{buildContext: buildContext}
}

func (m *Manager) Create(snapshotID string, rc *restic.ResticClient) (*StagingEnv, error) {
	restoreDir, err := os.MkdirTemp("", "wpmaintenance-staging-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create staging restore dir: %w", err)
	}

	env := &StagingEnv{
		SnapshotID:   snapshotID,
		RestoreDir:   restoreDir,
		ProjectName:  "wps-" + snapshotID[:8],
		BuildContext: m.buildContext,
	}

	if err := rc.Restore(snapshotID, restoreDir); err != nil {
		os.RemoveAll(restoreDir)
		return nil, fmt.Errorf("restic restore failed: %w", err)
	}

	if err := env.detectVersions(restoreDir); err != nil {
		os.RemoveAll(restoreDir)
		return nil, fmt.Errorf("version detection failed: %w", err)
	}

	if err := env.detectDBCredentials(restoreDir); err != nil {
		os.RemoveAll(restoreDir)
		return nil, fmt.Errorf("DB credential detection failed: %w", err)
	}

	composeFile, err := env.generateComposeFile(m.buildContext)
	if err != nil {
		os.RemoveAll(restoreDir)
		return nil, fmt.Errorf("compose file generation failed: %w", err)
	}
	env.ComposeFile = composeFile

	if err := env.up(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("docker compose up failed: %w", err)
	}

	if err := env.waitForDB(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("database not ready: %w", err)
	}

	if err := env.createUser(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("DB user creation failed: %w", err)
	}

	if err := env.importDB(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("database import failed: %w", err)
	}

	if err := env.copyFiles(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("file copy failed: %w", err)
	}

	if err := env.patchWPConfig(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("wp-config patch failed: %w", err)
	}

	if err := env.readTablePrefix(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("table prefix detection failed: %w", err)
	}

	if err := env.updateSiteURL(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("site URL update failed: %w", err)
	}

	if err := env.resolvePorts(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("port resolution failed: %w", err)
	}

	env.HealthURL = fmt.Sprintf("https://localhost:%s", env.HTTPSPort)

	return env, nil
}

func (m *Manager) Destroy(env *StagingEnv) error {
	if env == nil {
		return nil
	}
	return env.cleanup()
}

func (env *StagingEnv) WPCLI(args string) (string, error) {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp",
		"wp", "--allow-root", "--path=/var/www/html", args,
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("wp-cli failed: %w\nOutput: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (env *StagingEnv) Logs() (string, error) {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"logs", "--tail=50",
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (env *StagingEnv) up() error {
	args := []string{
		"compose", "-f", env.ComposeFile,
		"-p", env.ProjectName,
		"up", "-d", "--build",
	}
	cmd := exec.Command("docker", args...)
	cmd.Env = append(os.Environ(),
		"DB_IMAGE="+env.DBImage,
		"DB_TAG="+env.DBTag,
		"WP_VERSION="+env.WPVersion,
		"WP_PHP_VERSION="+env.PHPVersion,
		"DB_NAME="+env.DBName,
		"DB_USER="+env.DBUser,
		"DB_PASS="+env.DBPassword,
		"BUILD_CONTEXT="+env.BuildContext,
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\nOutput: %s", err, string(output))
	}
	return nil
}

func (env *StagingEnv) waitForDB() error {
	for i := 0; i < 30; i++ {
		cmd := exec.Command("docker", "compose",
			"-f", env.ComposeFile,
			"-p", env.ProjectName,
			"exec", "-T", "db",
			"mysqladmin", "ping", "-h", "localhost",
			"-u", "root", "-prootpass",
		)
		cmd.Env = os.Environ()
		if err := cmd.Run(); err == nil {
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("database did not become ready within 60s")
}

func (env *StagingEnv) createUser() error {
	createSQL := fmt.Sprintf(
		"CREATE DATABASE IF NOT EXISTS `%s`; CREATE USER IF NOT EXISTS '%s'@'%%' IDENTIFIED BY '%s'; GRANT ALL ON `%s`.* TO '%s'@'%%'; FLUSH PRIVILEGES;",
		env.DBName, env.DBUser, env.DBPassword, env.DBName, env.DBUser,
	)
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "-u", "root", "-prootpass",
		"-e", createSQL,
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\nOutput: %s", err, string(output))
	}
	return nil
}

func (env *StagingEnv) importDB() error {
	dumpFiles, _ := filepath.Glob(filepath.Join(env.RestoreDir, "backup_artifacts", "*", "db", "*.sql"))
	gzFiles, _ := filepath.Glob(filepath.Join(env.RestoreDir, "backup_artifacts", "*", "db", "*.sql.gz"))

	var dumpFile string
	if len(gzFiles) > 0 {
		dumpFile = gzFiles[0]
	} else if len(dumpFiles) > 0 {
		dumpFile = dumpFiles[0]
	}

	if dumpFile == "" {
		return nil
	}

	if strings.HasSuffix(dumpFile, ".gz") {
		gunzip := exec.Command("gunzip", "-c", dumpFile)
		mysql := exec.Command("docker", "compose",
			"-f", env.ComposeFile,
			"-p", env.ProjectName,
			"exec", "-T", "db",
			"mysql", "--max-allowed-packet=1G",
			"-u", "root", "-prootpass", env.DBName,
		)
		mysql.Env = os.Environ()
		mysql.Stdin, _ = gunzip.StdoutPipe()
		gunzip.Stdout = nil

		if err := gunzip.Start(); err != nil {
			return fmt.Errorf("gunzip start failed: %w", err)
		}
		if err := mysql.Start(); err != nil {
			gunzip.Process.Kill()
			return fmt.Errorf("mysql start failed: %w", err)
		}
		gunzip.Wait()
		if err := mysql.Wait(); err != nil {
			return fmt.Errorf("mysql import failed: %w", err)
		}
		return nil
	}

	cat := exec.Command("cat", dumpFile)
	mysql := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "--max-allowed-packet=1G",
		"-u", "root", "-prootpass", env.DBName,
	)
	mysql.Env = os.Environ()
	mysql.Stdin, _ = cat.StdoutPipe()
	cat.Stdout = nil

	if err := cat.Start(); err != nil {
		return fmt.Errorf("cat start failed: %w", err)
	}
	if err := mysql.Start(); err != nil {
		cat.Process.Kill()
		return fmt.Errorf("mysql start failed: %w", err)
	}
	cat.Wait()
	if err := mysql.Wait(); err != nil {
		return fmt.Errorf("mysql import failed: %w", err)
	}
	return nil
}

func (env *StagingEnv) copyFiles() error {
	wpDirs, _ := filepath.Glob(filepath.Join(env.RestoreDir, "backup_artifacts", "*", "wp"))
	if len(wpDirs) == 0 {
		return nil
	}
	wpDir := wpDirs[0]

	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"cp", wpDir+"/.", "wp:/var/www/html/",
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\nOutput: %s", err, string(output))
	}

	rmCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "rm", "-f", "/var/www/html/index.html",
	)
	rmCmd.Env = os.Environ()
	rmCmd.Run()

	return nil
}

func (env *StagingEnv) patchWPConfig() error {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T",
		"-e", "WORDPRESS_DB_HOST=db:3306",
		"-e", "WORDPRESS_DB_NAME="+env.DBName,
		"-e", "WORDPRESS_DB_USER="+env.DBUser,
		"-e", "WORDPRESS_DB_PASSWORD="+env.DBPassword,
		"wp", "wp-config-gen.sh",
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%w\nOutput: %s", err, string(output))
	}
	return nil
}

func (env *StagingEnv) readTablePrefix() error {
	wpDirs, _ := filepath.Glob(filepath.Join(env.RestoreDir, "backup_artifacts", "*", "wp"))
	if len(wpDirs) == 0 {
		env.TablePrefix = "wp_"
		return nil
	}

	wpConfig := filepath.Join(wpDirs[0], "wp-config.php")
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		env.TablePrefix = "wp_"
		return nil
	}

	re := regexp.MustCompile(`table_prefix\s*=\s*'([^']*)'`)
	matches := re.FindSubmatch(data)
	if len(matches) < 2 {
		env.TablePrefix = "wp_"
		return nil
	}

	env.TablePrefix = string(matches[1])
	return nil
}

func (env *StagingEnv) updateSiteURL() error {
	prefixSQL := strings.ReplaceAll(env.TablePrefix, "`", "")
	updateSQL := fmt.Sprintf(
		"UPDATE `%s`.`%soptions` SET option_value='https://localhost:%s' WHERE option_name IN ('siteurl','home')",
		env.DBName, prefixSQL, env.HTTPSPort,
	)
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "-u", "root", "-prootpass",
		"-e", updateSQL,
	)
	cmd.Env = os.Environ()
	cmd.Run()

	flushCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "wp", "cache", "flush", "--allow-root",
	)
	flushCmd.Env = os.Environ()
	flushCmd.Run()

	return nil
}

func (env *StagingEnv) resolvePorts() error {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"port", "wp", "443",
	)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get HTTPS port: %w", err)
	}
	env.HTTPSPort = strings.TrimSpace(string(out))

	cmd = exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"port", "wp", "80",
	)
	cmd.Env = os.Environ()
	out, err = cmd.Output()
	if err != nil {
		return fmt.Errorf("failed to get HTTP port: %w", err)
	}
	env.WPSPort = strings.TrimSpace(string(out))

	return nil
}

func (env *StagingEnv) cleanup() error {
	if env.ComposeFile == "" {
		return nil
	}
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"down", "-v", "--remove-orphans",
	)
	cmd.Env = os.Environ()
	cmd.Run()

	if env.RestoreDir != "" {
		os.RemoveAll(env.RestoreDir)
	}

	if env.ComposeFile != "" {
		os.Remove(env.ComposeFile)
	}

	return nil
}

var wpVersionRe = regexp.MustCompile(`wp_version\s*=\s*'([^']+)'`)
var dbNameRe = regexp.MustCompile(`define.*'DB_NAME'.*'([^']+)'`)
var dbUserRe = regexp.MustCompile(`define.*'DB_USER'.*'([^']+)'`)
var dbPassRe = regexp.MustCompile(`define.*'DB_PASSWORD'.*'([^']+)'`)

func (env *StagingEnv) detectVersions(restoreDir string) error {
	wpDirs, _ := filepath.Glob(filepath.Join(restoreDir, "backup_artifacts", "*", "wp"))
	if len(wpDirs) > 0 {
		versionFile := filepath.Join(wpDirs[0], "wp-includes", "version.php")
		data, err := os.ReadFile(versionFile)
		if err == nil {
			matches := wpVersionRe.FindSubmatch(data)
			if len(matches) >= 2 {
				env.WPVersion = string(matches[1])
			}
		}
	}

	if env.WPVersion == "" {
		env.WPVersion = "6.6.2"
	}
	if env.PHPVersion == "" {
		env.PHPVersion = "8.2"
	}
	env.DBImage = "mariadb"
	env.DBTag = "10.11"

	manifestFiles, _ := filepath.Glob(filepath.Join(restoreDir, "backup_artifacts", "*", "manifest.txt"))
	for _, mf := range manifestFiles {
		data, err := os.ReadFile(mf)
		if err != nil {
			continue
		}
		content := string(data)
		for _, line := range strings.Split(content, "\n") {
			if strings.HasPrefix(line, "db_version=") {
				dbVer := strings.TrimPrefix(line, "db_version=")
				if strings.Contains(strings.ToLower(dbVer), "maria") {
					env.DBImage = "mariadb"
					parts := strings.SplitN(dbVer, ".", 3)
					if len(parts) >= 2 {
						env.DBTag = parts[0] + "." + parts[1]
					}
				} else {
					env.DBImage = "mysql"
					parts := strings.SplitN(dbVer, ".", 3)
					if len(parts) >= 2 {
						env.DBTag = parts[0] + "." + parts[1]
					}
				}
			}
		}
	}

	return nil
}

func (env *StagingEnv) detectDBCredentials(restoreDir string) error {
	wpDirs, _ := filepath.Glob(filepath.Join(restoreDir, "backup_artifacts", "*", "wp"))
	if len(wpDirs) == 0 {
		env.DBName = "wordpress"
		env.DBUser = "wpuser"
		env.DBPassword = "wppass"
		return nil
	}

	wpConfig := filepath.Join(wpDirs[0], "wp-config.php")
	data, err := os.ReadFile(wpConfig)
	if err != nil {
		env.DBName = "wordpress"
		env.DBUser = "wpuser"
		env.DBPassword = "wppass"
		return nil
	}

	if m := dbNameRe.FindSubmatch(data); len(m) >= 2 {
		env.DBName = string(m[1])
	}
	if m := dbUserRe.FindSubmatch(data); len(m) >= 2 {
		env.DBUser = string(m[1])
	}
	if m := dbPassRe.FindSubmatch(data); len(m) >= 2 {
		env.DBPassword = string(m[1])
	}

	if env.DBName == "" {
		env.DBName = "wordpress"
	}
	if env.DBUser == "" {
		env.DBUser = "wpuser"
	}
	if env.DBPassword == "" {
		env.DBPassword = "wppass"
	}

	return nil
}

func (env *StagingEnv) generateComposeFile(buildContext string) (string, error) {
	template, err := os.ReadFile(filepath.Join(buildContext, "staging", "docker-compose.template.yml"))
	if err != nil {
		return "", fmt.Errorf("failed to read compose template: %w", err)
	}

	composeFile, err := os.CreateTemp("", "wpmaintenance-compose-*.yml")
	if err != nil {
		return "", fmt.Errorf("failed to create compose file: %w", err)
	}

	content := string(template)
	content = strings.ReplaceAll(content, "${BUILD_CONTEXT:-.}", buildContext)

	if _, err := composeFile.WriteString(content); err != nil {
		composeFile.Close()
		os.Remove(composeFile.Name())
		return "", err
	}
	composeFile.Close()

	return composeFile.Name(), nil
}
