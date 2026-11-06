package staging

import (
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
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
	return m.CreateWithOptions(snapshotID, rc, CreateOptions{})
}

// CreateWithPublicHost creates a staging environment and sets the host used in
// its public healthcheck URL.
func (m *Manager) CreateWithPublicHost(snapshotID string, rc *restic.ResticClient, publicHost string) (*StagingEnv, error) {
	return m.CreateWithOptions(snapshotID, rc, CreateOptions{PublicHost: publicHost})
}

// CreateOptions controls how a staging environment is built.
type CreateOptions struct {
	// PublicHost is the hostname used in the staging healthcheck URL.
	PublicHost string
	// PHPVersion is the production PHP version to mirror (for example
	// "8.3" or "8.3.27"). The major/minor pair is used for the Docker image.
	PHPVersion string
}

// removeWPConfigConstantsScript removes active WP_HOME, WP_SITEURL,
// WPCACHEHOME, and WP_CACHE definitions from wp-config.php (in both quote
// styles) so the staging copy uses its own URL, cache path, and fresh content.
// Commented-out definitions are kept.
const removeWPConfigConstantsScript = `/^[[:space:]]*define[[:space:]]*([[:space:]]*['"]\(WP_HOME\|WP_SITEURL\|WPCACHEHOME\|WP_CACHE\)['"]/d`

func (m *Manager) CreateWithOptions(snapshotID string, rc *restic.ResticClient, opts CreateOptions) (*StagingEnv, error) {
	snapshotID = strings.TrimSpace(snapshotID)
	if snapshotID == "" {
		return nil, fmt.Errorf("snapshot ID is required")
	}

	restoreDir, err := os.MkdirTemp("", "wpmaintenance-staging-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create staging restore dir: %w", err)
	}

	env := &StagingEnv{
		SnapshotID:   snapshotID,
		RestoreDir:   restoreDir,
		ProjectName:  stagingProjectName(snapshotID),
		PublicHost:   strings.TrimSpace(opts.PublicHost),
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

	// Mirror the production PHP version when it was detected. Falling back to
	// detectVersions' default keeps staging usable when detection is not
	// possible.
	if phpVersion := NormalizePHPVersion(opts.PHPVersion); phpVersion != "" {
		env.PHPVersion = phpVersion
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
		env.cleanup(false)
		return nil, fmt.Errorf("docker compose up failed: %w", err)
	}

	if err := env.waitForDB(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("database not ready: %w", err)
	}

	if err := env.createUser(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("DB user creation failed: %w", err)
	}

	if err := env.importDB(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("database import failed: %w", err)
	}

	if err := env.copyFiles(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("file copy failed: %w", err)
	}

	if err := env.disableStagingCaches(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("staging cache disabling failed: %w", err)
	}

	if err := env.patchWPConfig(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("wp-config patch failed: %w", err)
	}

	// Remove site URL and cache-path constants from wp-config.php so the
	// staging URL from the database and the staging cache paths take effect.
	removeConstCmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp", "sed", "-i",
		removeWPConfigConstantsScript,
		"/var/www/html/wp-config.php",
	)
	removeConstCmd.Env = os.Environ()
	if out, err := removeConstCmd.CombinedOutput(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("failed to remove WP_HOME/WP_SITEURL constants: %w\nOutput: %s", err, string(out))
	}

	if err := env.readTablePrefix(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("table prefix detection failed: %w", err)
	}

	if err := env.resolvePorts(); err != nil {
		env.cleanup(false)
		return nil, fmt.Errorf("port resolution failed: %w", err)
	}

	if err := env.updateSiteURL(); err != nil {
		env.cleanup(false)
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
	return env.cleanup(false)
}

func (m *Manager) DestroyForce(env *StagingEnv) error {
	if env == nil {
		return nil
	}
	return env.cleanup(true)
}

func (env *StagingEnv) WPCLI(args string) (string, error) {
	wpArgs := strings.Fields(strings.TrimSpace(args))
	if len(wpArgs) == 0 {
		return "", fmt.Errorf("wp-cli args are required")
	}
	globalArgs := make([]string, 0, len(wpArgs))
	commandArgs := make([]string, 0, len(wpArgs))
	for _, arg := range wpArgs {
		if strings.HasPrefix(arg, "--skip-") || strings.HasPrefix(arg, "--url=") || arg == "--quiet" {
			globalArgs = append(globalArgs, arg)
			continue
		}
		commandArgs = append(commandArgs, arg)
	}
	cmdArgs := []string{
		"compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T",
		"-e", "HTTP_X_FORWARDED_PROTO=https",
		"wp",
		"wp", "--allow-root", "--path=/var/www/html",
	}
	if len(globalArgs) > 0 {
		cmdArgs = append(cmdArgs, globalArgs...)
	}
	cmdArgs = append(cmdArgs, commandArgs...)
	cmd := exec.Command("docker", cmdArgs...)
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
		"CREATE DATABASE IF NOT EXISTS %s; CREATE USER IF NOT EXISTS %s@'%%' IDENTIFIED BY %s; GRANT ALL ON %s.* TO %s@'%%'; FLUSH PRIVILEGES;",
		quoteMySQLIdentifier(env.DBName),
		quoteMySQLString(env.DBUser), quoteMySQLString(env.DBPassword),
		quoteMySQLIdentifier(env.DBName), quoteMySQLString(env.DBUser),
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

	f, err := os.Open(dumpFile)
	if err != nil {
		return fmt.Errorf("failed to open database dump: %w", err)
	}
	defer f.Close()

	var reader io.Reader = f
	if strings.HasSuffix(dumpFile, ".gz") {
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("failed to open gzipped database dump: %w", err)
		}
		defer gz.Close()
		reader = gz
	}

	mysql := exec.Command("docker", mysqlArgs...)
	mysql.Env = os.Environ()
	mysql.Stdin = reader

	if output, err := mysql.CombinedOutput(); err != nil {
		return fmt.Errorf("mysql import failed: %w\nOutput: %s", err, string(output))
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

// disableStagingCaches removes page-cache and object-cache drop-ins copied
// from production. Their absolute paths and backing services belong to the
// production host, and serving a cached production page would hide upgrade
// changes from the rehearsal healthcheck and page comparison.
func (env *StagingEnv) disableStagingCaches() error {
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"exec", "-T", "wp",
		"rm", "-rf",
		"/var/www/html/wp-content/advanced-cache.php",
		"/var/www/html/wp-content/object-cache.php",
		"/var/www/html/wp-content/cache",
	)
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w\nOutput: %s", err, string(out))
	}
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

	re := regexp.MustCompile(`table_prefix\s*=\s*['"]([^'"]*)['"]`)
	matches := re.FindSubmatch(data)
	if len(matches) < 2 {
		env.TablePrefix = "wp_"
		return nil
	}

	env.TablePrefix = string(matches[1])
	return nil
}

func (env *StagingEnv) updateSiteURL() error {
	oldURL, _ := env.getSiteURL()
	log.Printf("staging: getSiteURL returned '%s' (db=%s, prefix=%s)", oldURL, env.DBName, env.TablePrefix)

	host := env.PublicHost
	if host == "" {
		host = "localhost"
	}
	newURL := fmt.Sprintf("https://%s:%s", host, env.HTTPSPort)
	log.Printf("staging: newURL='%s'", newURL)

	env.applySiteURL(oldURL, newURL)
	return nil
}

func (env *StagingEnv) applySiteURL(oldURL, newURL string) {
	prefixSQL := strings.TrimSpace(env.TablePrefix)
	if prefixSQL == "" {
		prefixSQL = "wp_"
	}

	updateSQL := fmt.Sprintf(
		"UPDATE %s.%s SET option_value=%s WHERE option_name IN ('siteurl','home')",
		quoteMySQLIdentifier(env.DBName), quoteMySQLIdentifier(prefixSQL+"options"), quoteMySQLString(newURL),
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
}

func (env *StagingEnv) getSiteURL() (string, error) {
	prefixSQL := strings.TrimSpace(env.TablePrefix)
	if prefixSQL == "" {
		prefixSQL = "wp_"
	}
	selectSQL := fmt.Sprintf(
		"SELECT option_value FROM %s.%s WHERE option_name='siteurl' LIMIT 1",
		quoteMySQLIdentifier(env.DBName), quoteMySQLIdentifier(prefixSQL+"options"),
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

func (env *StagingEnv) cleanup(force bool) error {
	if env.ComposeFile == "" {
		if force {
			return env.forceCleanup()
		}
		return nil
	}
	cmd := exec.Command("docker", "compose",
		"-f", env.ComposeFile,
		"-p", env.ProjectName,
		"down", "-v", "--remove-orphans",
	)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil && !force {
		return fmt.Errorf("docker compose down failed: %w\nOutput: %s", err, string(output))
	}
	if force {
		if forceErr := env.forceCleanup(); forceErr != nil {
			return forceErr
		}
	}

	if env.RestoreDir != "" {
		os.RemoveAll(env.RestoreDir)
	}

	if env.ComposeFile != "" {
		os.Remove(env.ComposeFile)
	}

	return nil
}

func (env *StagingEnv) forceCleanup() error {
	if env.ProjectName == "" {
		return nil
	}
	listCmd := exec.Command("docker", "ps", "-aq", "--filter", "label=com.docker.compose.project="+env.ProjectName)
	listCmd.Env = os.Environ()
	idsOut, err := listCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker ps force cleanup failed: %w\nOutput: %s", err, string(idsOut))
	}
	ids := strings.Fields(string(idsOut))
	if len(ids) > 0 {
		args := append([]string{"rm", "-f"}, ids...)
		killCmd := exec.Command("docker", args...)
		killCmd.Env = os.Environ()
		killOut, killErr := killCmd.CombinedOutput()
		if killErr != nil {
			return fmt.Errorf("docker rm -f failed: %w\nOutput: %s", killErr, string(killOut))
		}
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

var wpVersionRe = regexp.MustCompile(`wp_version\s*=\s*['"]([^'"]+)['"]`)
var dbNameRe = regexp.MustCompile(`define\s*\(\s*['"]DB_NAME['"]\s*,\s*['"]([^'"]+)['"]`)
var dbUserRe = regexp.MustCompile(`define\s*\(\s*['"]DB_USER['"]\s*,\s*['"]([^'"]+)['"]`)
var dbPassRe = regexp.MustCompile(`define\s*\(\s*['"]DB_PASSWORD['"]\s*,\s*['"]([^'"]+)['"]`)

var phpVersionRe = regexp.MustCompile(`(?i)^\s*(?:PHP\s+)?v?(\d+)\.(\d+)`)

// NormalizePHPVersion extracts the major.minor pair used for Docker image tags
// from a PHP version string such as "8.3.27" or "PHP 8.3.27". It returns an
// empty string when the value is not a supported PHP version.
func NormalizePHPVersion(version string) string {
	matches := phpVersionRe.FindStringSubmatch(strings.TrimSpace(version))
	if len(matches) != 3 {
		return ""
	}
	major, err := strconv.Atoi(matches[1])
	if err != nil || major < 7 {
		return ""
	}
	minor, err := strconv.Atoi(matches[2])
	if err != nil {
		return ""
	}
	if major == 7 && minor < 4 {
		return ""
	}
	return fmt.Sprintf("%d.%d", major, minor)
}

// stagingProjectName builds a Docker Compose project name from a snapshot ID.
// It never slices the raw ID, so short or non-hex IDs cannot panic or produce
// an invalid project name.
func stagingProjectName(snapshotID string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(strings.TrimSpace(snapshotID)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	suffix := b.String()
	if len(suffix) > 8 {
		suffix = suffix[:8]
	}
	if suffix == "" {
		sum := sha256.Sum256([]byte(snapshotID))
		suffix = hex.EncodeToString(sum[:4])
	}
	return "wps-" + suffix
}

func quoteMySQLIdentifier(value string) string {
	return "`" + strings.ReplaceAll(value, "`", "``") + "`"
}

func quoteMySQLString(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		"'", "''",
		"\x00", `\0`,
		"\n", `\n`,
		"\r", `\r`,
	)
	return "'" + replacer.Replace(value) + "'"
}

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
		env.PHPVersion = "8.3"
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
