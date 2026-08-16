package staging

import (
	"fmt"
	"io/fs"
	"log"
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
	PublicHost   string
	BuildContext string
}

func NewManager(buildContext string) *Manager {
	return &Manager{buildContext: buildContext}
}

func NewEnv(composeFile, projectName, restoreDir, healthURL string) *StagingEnv {
	return &StagingEnv{
		ComposeFile: composeFile,
		ProjectName: projectName,
		RestoreDir:  restoreDir,
		HealthURL:   healthURL,
	}
}

func (m *Manager) Create(snapshotID string, rc *restic.ResticClient) (*StagingEnv, error) {
	return m.CreateWithPublicHost(snapshotID, rc, "")
}

func (m *Manager) CreateWithPublicHost(snapshotID string, rc *restic.ResticClient, publicHost string) (*StagingEnv, error) {
	restoreDir, err := os.MkdirTemp("", "wpmaintenance-staging-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create staging restore dir: %w", err)
	}

	env := &StagingEnv{
		SnapshotID:   snapshotID,
		RestoreDir:   restoreDir,
		ProjectName:  "wps-" + snapshotID[:8],
		PublicHost:   strings.TrimSpace(publicHost),
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

	// Remove WP_HOME and WP_SITEURL constants from wp-config.php so they
	// don't override the database values we set in updateSiteURL()
	removeConstCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "sed", "-i",
		"/^define(.WP_HOME./d;/^define(.WP_SITEURL./d",
		"/var/www/html/wp-config.php",
	)
	removeConstCmd.Env = os.Environ()
	removeConstCmd.CombinedOutput()

	if err := env.readTablePrefix(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("table prefix detection failed: %w", err)
	}

	if err := env.resolvePorts(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("port resolution failed: %w", err)
	}

	if err := env.updateSiteURL(); err != nil {
		env.cleanup()
		return nil, fmt.Errorf("site URL update failed: %w", err)
	}

	host := env.PublicHost
	if host == "" {
		host = "localhost"
	}
	env.HealthURL = fmt.Sprintf("https://%s:%s", host, env.HTTPSPort)

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

// IsRunning reports whether any container for this staging environment is
// currently running, regardless of the recorded job status.
func (env *StagingEnv) IsRunning() bool {
	if env == nil || env.ProjectName == "" {
		return false
	}
	// Prefer compose-native status detection because docker label filtering can
	// be unreliable when compose project naming is overridden.
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"ps", "--status", "running", "--services",
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err == nil {
		services := strings.Fields(strings.TrimSpace(string(output)))
		for _, svc := range services {
			if svc == "wp" || svc == "db" {
				return true
			}
		}
	}

	// Fallback for older docker/compose combinations.
	cmd = exec.Command("docker", "ps",
		"--filter", "label=com.docker.compose.project="+env.ProjectName,
		"--filter", "status=running",
		"--format", "{{.ID}}",
	)
	cmd.Env = os.Environ()
	output, err = cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(output)) != ""
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
			"mysql", "-h", "127.0.0.1",
			"-u", "root", "-prootpass",
			"-e", "SELECT 1",
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
		"mysql", "-h", "127.0.0.1",
		"-u", "root", "-prootpass",
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
	dbDir := findDir(env.RestoreDir, "db")
	var dumpFiles, gzFiles []string
	if dbDir != "" {
		dumpFiles, _ = filepath.Glob(filepath.Join(dbDir, "*.sql"))
		gzFiles, _ = filepath.Glob(filepath.Join(dbDir, "*.sql.gz"))
	}

	var dumpFile string
	if len(gzFiles) > 0 {
		dumpFile = gzFiles[0]
	} else if len(dumpFiles) > 0 {
		dumpFile = dumpFiles[0]
	}

	if dumpFile == "" {
		return nil
	}

	mysqlArgs := []string{
		"compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "-h", "127.0.0.1",
		"--max-allowed-packet=1G",
		"-u", "root", "-prootpass", env.DBName,
	}

	if strings.HasSuffix(dumpFile, ".gz") {
		gunzip := exec.Command("gunzip", "-c", dumpFile)
		mysql := exec.Command("docker", mysqlArgs...)
		mysql.Env = os.Environ()
		mysql.Stdin, _ = gunzip.StdoutPipe()

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
	mysql := exec.Command("docker", mysqlArgs...)
	mysql.Env = os.Environ()
	mysql.Stdin, _ = cat.StdoutPipe()

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
	wpDir := findDir(env.RestoreDir, "wp")
	if wpDir == "" {
		return nil
	}

	tarCmd := exec.Command("tar", "cf", "-", "-C", wpDir, ".")
	dockerCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp",
		"tar", "xf", "-", "-C", "/var/www/html/",
	)
	dockerCmd.Env = os.Environ()
	dockerCmd.Stdin, _ = tarCmd.StdoutPipe()

	if err := tarCmd.Start(); err != nil {
		return fmt.Errorf("tar start failed: %w", err)
	}
	if err := dockerCmd.Start(); err != nil {
		tarCmd.Process.Kill()
		return fmt.Errorf("docker tar exec start failed: %w", err)
	}
	if err := tarCmd.Wait(); err != nil {
		return fmt.Errorf("tar failed: %w", err)
	}
	if err := dockerCmd.Wait(); err != nil {
		return fmt.Errorf("docker tar exec failed: %w", err)
	}

	chownCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp",
		"chown", "-R", "www-data:www-data", "/var/www/html/",
	)
	chownCmd.Env = os.Environ()
	chownCmd.Run()

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
	wpDir := findDir(env.RestoreDir, "wp")
	if wpDir == "" {
		env.TablePrefix = "wp_"
		return nil
	}

	wpConfig := filepath.Join(wpDir, "wp-config.php")
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

	oldURL, _ := env.getSiteURL()
	log.Printf("staging: getSiteURL returned '%s' (db=%s, prefix=%s)", oldURL, env.DBName, prefixSQL)

	host := env.PublicHost
	if host == "" {
		host = "localhost"
	}
	newURL := fmt.Sprintf("https://%s:%s", host, env.HTTPSPort)
	log.Printf("staging: newURL='%s'", newURL)

	updateSQL := fmt.Sprintf(
		"UPDATE `%s`.`%soptions` SET option_value='%s' WHERE option_name IN ('siteurl','home')",
		env.DBName, prefixSQL, newURL,
	)
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "-h", "127.0.0.1",
		"-u", "root", "-prootpass",
		"-e", updateSQL,
	)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("staging: SQL siteurl update failed: %v\nOutput: %s", err, string(out))
	}

	// Also update via WP-CLI to ensure WordPress object cache is updated
	wpOptionCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "wp", "option", "update", "siteurl", newURL, "--allow-root",
	)
	wpOptionCmd.Env = os.Environ()
	wpOptionCmd.CombinedOutput()
	wpOptionCmd2 := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "wp", "option", "update", "home", newURL, "--allow-root",
	)
	wpOptionCmd2.Env = os.Environ()
	wpOptionCmd2.CombinedOutput()

	flushCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "wp", "cache", "flush", "--allow-root",
	)
	flushCmd.Env = os.Environ()
	if out, err := flushCmd.CombinedOutput(); err != nil {
		log.Printf("staging: wp cache flush failed: %v\nOutput: %s", err, string(out))
	}

	if oldURL != "" && oldURL != newURL {
		searchOld := strings.TrimRight(oldURL, "/")
		searchNew := strings.TrimRight(newURL, "/")
		log.Printf("staging: wp search-replace from '%s' to '%s'", searchOld, searchNew)
		wpCLICmd := exec.Command("docker", "compose",
			"-f", env.ComposeFile,
			"-p", env.ProjectName,
			"exec", "-T", "wp", "wp", "search-replace",
			"--all-tables", "--allow-root",
			"--precise",
			"--skip-columns=guid",
			searchOld, searchNew,
		)
		wpCLICmd.Env = os.Environ()
		if out, err := wpCLICmd.CombinedOutput(); err != nil {
			log.Printf("staging: wp search-replace from '%s' to '%s' failed: %v\nOutput: %s", searchOld, searchNew, err, string(out))
		}

		// Also replace www version if the old URL doesn't have www
		if !strings.Contains(searchOld, "://www.") {
			wwwOld := strings.Replace(searchOld, "://", "://www.", 1)
			log.Printf("staging: wp search-replace (www) from '%s' to '%s'", wwwOld, searchNew)
			wpCLICmd2 := exec.Command("docker", "compose",
				"-f", env.ComposeFile,
				"-p", env.ProjectName,
				"exec", "-T", "wp", "wp", "search-replace",
				"--all-tables", "--allow-root",
				"--precise",
				"--skip-columns=guid",
				wwwOld, searchNew,
			)
			wpCLICmd2.Env = os.Environ()
			if out, err := wpCLICmd2.CombinedOutput(); err != nil {
				log.Printf("staging: wp search-replace (www) from '%s' to '%s' failed: %v\nOutput: %s", wwwOld, searchNew, err, string(out))
			}

			// Also replace bare domain (without protocol) for URLs stored without http://
			oldDomain := strings.TrimPrefix(wwwOld, "https://")
			oldDomain = strings.TrimPrefix(oldDomain, "http://")
			newDomain := strings.TrimPrefix(searchNew, "https://")
			newDomain = strings.TrimPrefix(newDomain, "http://")
			log.Printf("staging: wp search-replace (bare domain) from '%s' to '%s'", oldDomain, newDomain)
			wpCLICmd3 := exec.Command("docker", "compose",
				"-f", env.ComposeFile,
				"-p", env.ProjectName,
				"exec", "-T", "wp", "wp", "search-replace",
				"--all-tables", "--allow-root",
				"--precise",
				"--skip-columns=guid",
				oldDomain, newDomain,
			)
			wpCLICmd3.Env = os.Environ()
			if out, err := wpCLICmd3.CombinedOutput(); err != nil {
				log.Printf("staging: wp search-replace (bare domain) from '%s' to '%s' failed: %v\nOutput: %s", oldDomain, newDomain, err, string(out))
			}
		}
	}

	// Replace hardcoded URLs in physical files (e.g. Elementor-generated CSS)
	if oldURL != "" && oldURL != newURL {
		searchOld := strings.TrimRight(oldURL, "/")
		searchNew := strings.TrimRight(newURL, "/")

		sedCmd := exec.Command("docker", "compose",
			"-f", env.ComposeFile,
			"-p", env.ProjectName,
			"exec", "-T", "wp",
			"find", "/var/www/html/wp-content/",
			"-type", "f",
			"-exec", "sed", "-i",
			fmt.Sprintf("s|%s|%s|g", searchOld, searchNew),
			"{}", "+",
		)
		sedCmd.Env = os.Environ()
		if out, err := sedCmd.CombinedOutput(); err != nil {
			log.Printf("staging: sed URL replace in files failed: %v\nOutput: %s", err, string(out))
		}

		if !strings.Contains(searchOld, "://www.") {
			wwwOld := strings.Replace(searchOld, "://", "://www.", 1)
			sedWwwCmd := exec.Command("docker", "compose",
				"-f", env.ComposeFile,
				"-p", env.ProjectName,
				"exec", "-T", "wp",
				"find", "/var/www/html/wp-content/",
				"-type", "f",
				"-exec", "sed", "-i",
				fmt.Sprintf("s|%s|%s|g", wwwOld, searchNew),
				"{}", "+",
			)
			sedWwwCmd.Env = os.Environ()
			if out, err := sedWwwCmd.CombinedOutput(); err != nil {
				log.Printf("staging: sed www URL replace in files failed: %v\nOutput: %s", err, string(out))
			}
		}
	}

	return nil
}

func (env *StagingEnv) getSiteURL() (string, error) {
	prefixSQL := strings.ReplaceAll(env.TablePrefix, "`", "")
	selectSQL := fmt.Sprintf(
		"SELECT option_value FROM `%s`.`%soptions` WHERE option_name='siteurl' LIMIT 1",
		env.DBName, prefixSQL,
	)
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "db",
		"mysql", "-h", "127.0.0.1",
		"-u", "root", "-prootpass",
		"-sN", "-e", selectSQL,
	)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (env *StagingEnv) resolvePorts() error {
	httpsPort := env.getContainerPort("443")
	if httpsPort == "" {
		return fmt.Errorf("failed to resolve HTTPS port")
	}
	env.HTTPSPort = httpsPort

	httpPort := env.getContainerPort("80")
	if httpPort == "" {
		return fmt.Errorf("failed to resolve HTTP port")
	}
	env.WPSPort = httpPort

	return nil
}

func (env *StagingEnv) getContainerPort(containerPort string) string {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"port", "wp", containerPort,
	)
	cmd.Env = os.Environ()
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	portStr := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(portStr, ":"); idx >= 0 {
		return portStr[idx+1:]
	}
	return portStr
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

func findDir(root, target string) string {
	var found string
	filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || found != "" {
			return filepath.SkipAll
		}
		if d.IsDir() && d.Name() == target && path != root {
			found = path
			return filepath.SkipAll
		}
		return nil
	})
	return found
}

func findFile(root, pattern string) string {
	matches, _ := filepath.Glob(filepath.Join(root, pattern))
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

var wpVersionRe = regexp.MustCompile(`wp_version\s*=\s*'([^']+)'`)
var dbNameRe = regexp.MustCompile(`define\s*\(\s*'DB_NAME'\s*,\s*'([^']+)'`)
var dbUserRe = regexp.MustCompile(`define\s*\(\s*'DB_USER'\s*,\s*'([^']+)'`)
var dbPassRe = regexp.MustCompile(`define\s*\(\s*'DB_PASSWORD'\s*,\s*'([^']+)'`)

func (env *StagingEnv) detectVersions(restoreDir string) error {
	wpDir := findDir(restoreDir, "wp")
	if wpDir != "" {
		versionFile := filepath.Join(wpDir, "wp-includes", "version.php")
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

	manifestFile := findFile(restoreDir, "manifest.txt")
	if manifestFile != "" {
		data, err := os.ReadFile(manifestFile)
		if err == nil {
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
	}

	return nil
}

func (env *StagingEnv) detectDBCredentials(restoreDir string) error {
	wpDir := findDir(restoreDir, "wp")
	if wpDir == "" {
		env.DBName = "wordpress"
		env.DBUser = "wpuser"
		env.DBPassword = "wppass"
		return nil
	}

	wpConfig := filepath.Join(wpDir, "wp-config.php")
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
	absContext, err := filepath.Abs(buildContext)
	if err != nil {
		return "", fmt.Errorf("failed to resolve build context path: %w", err)
	}
	template, err := os.ReadFile(filepath.Join(absContext, "docker-compose.template.yml"))
	if err != nil {
		return "", fmt.Errorf("failed to read compose template: %w", err)
	}

	composeFile, err := os.CreateTemp("", "wpmaintenance-compose-*.yml")
	if err != nil {
		return "", fmt.Errorf("failed to create compose file: %w", err)
	}

	content := string(template)
	content = strings.ReplaceAll(content, "${BUILD_CONTEXT:-.}", absContext)

	if _, err := composeFile.WriteString(content); err != nil {
		composeFile.Close()
		os.Remove(composeFile.Name())
		return "", err
	}
	composeFile.Close()

	return composeFile.Name(), nil
}
