// Package sandbox manages a local, production-like WordPress test site.
//
// It reuses the Docker build context in ./staging (WordPress + Apache + PHP +
// OpenSSH + MariaDB client) to run an isolated stack on the loopback
// interface, then registers it as a regular site in the database. Because the
// site is reachable over SSH with a real database, the normal backup and
// restore pipeline (SSH + mysqldump + restic) runs against it end-to-end.
//
// This enables "disaster drills": take a backup, intentionally break the site
// (the Break method destroys files and the database), then restore and verify
// that everything is exactly as it was before.
package sandbox

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/auth"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/db"
	"github.com/andreacristalli/wp-maintenance-automation-go/internal/restic"
	"golang.org/x/crypto/ssh"
)

const (
	projectName = "wpm-sandbox"
	siteName    = "Sandbox (Local Test)"
	dbName      = "sandbox"
	dbUser      = "sandbox"
	dbPassword  = "sandboxpass"
	dbHost      = "db"
)

type Manager struct {
	DataDir      string
	BuildContext string
	Database     *db.Database
}

type State struct {
	SiteID     string `json:"site_id"`
	SSHPort    string `json:"ssh_port"`
	HTTPPort   string `json:"http_port"`
	HTTPSPort  string `json:"https_port"`
	HealthURL  string `json:"health_url"`
	AdminPass  string `json:"admin_pass"`
	Broken     bool   `json:"broken"`
	WPVersion  string `json:"wp_version"`
	LastStart  string `json:"last_start"`
	LastAction string `json:"last_action"`
}

// Status is returned to the API/UI.
type Status struct {
	Running    bool   `json:"running"`
	SiteID     string `json:"site_id"`
	HealthURL  string `json:"health_url"`
	WPVersion  string `json:"wp_version"`
	Healthy    bool   `json:"healthy"`
	Broken     bool   `json:"broken"`
	Message    string `json:"message"`
	SSHPort    int    `json:"ssh_port"`
	LastAction string `json:"last_action"`
}

func NewManager(dataDir, buildContext string, database *db.Database) *Manager {
	return &Manager{DataDir: dataDir, BuildContext: buildContext, Database: database}
}

// ---------------------------------------------------------------------------
// docker helpers
// ---------------------------------------------------------------------------

func (m *Manager) composePath() string {
	return filepath.Join(m.DataDir, "docker-compose.sandbox.yml")
}

func dockerAvailable() bool {
	cmd := exec.Command("docker", "version", "--format", "{{.Server.Version}}")
	cmd.Env = os.Environ()
	if out, err := cmd.CombinedOutput(); err != nil {
		log.Printf("sandbox: docker not available: %v\n%s", err, string(out))
		return false
	}
	return true
}

func (m *Manager) compose(args ...string) ([]byte, error) {
	full := append([]string{"compose", "-f", m.composePath(), "-p", projectName}, args...)
	cmd := exec.Command("docker", full...)
	cmd.Env = os.Environ()
	return cmd.CombinedOutput()
}

func (m *Manager) containerPort(service, containerPort string) string {
	out, err := m.compose("port", service, containerPort)
	if err != nil {
		return ""
	}
	s := strings.TrimSpace(string(out))
	if idx := strings.LastIndex(s, ":"); idx >= 0 {
		return s[idx+1:]
	}
	return s
}

// ---------------------------------------------------------------------------
// state persistence
// ---------------------------------------------------------------------------

func (m *Manager) statePath() string {
	return filepath.Join(m.DataDir, "state.json")
}

func (m *Manager) loadState() *State {
	data, err := os.ReadFile(m.statePath())
	if err != nil {
		return nil
	}
	var st State
	if err := json.Unmarshal(data, &st); err != nil {
		return nil
	}
	return &st
}

func (m *Manager) saveState(st *State) error {
	if err := os.MkdirAll(m.DataDir, 0700); err != nil {
		return err
	}
	data, _ := json.MarshalIndent(st, "", "  ")
	return os.WriteFile(m.statePath(), data, 0600)
}

func (m *Manager) ensureDirs() error {
	for _, d := range []string{m.DataDir, filepath.Join(m.DataDir, backupDir())} {
		if err := os.MkdirAll(d, 0700); err != nil {
			return err
		}
	}
	return nil
}

func backupDir() string { return "backups" }

// ---------------------------------------------------------------------------
// SSH keypair
// ---------------------------------------------------------------------------

func (m *Manager) keyPaths() (string, string) {
	return filepath.Join(m.DataDir, "ssh_key"), filepath.Join(m.DataDir, "ssh_key.pub")
}

func (m *Manager) ensureSSHKey() (string, error) {
	priv, pub := m.keyPaths()
	if _, err := os.Stat(priv); err == nil {
		pubData, err := os.ReadFile(pub)
		if err != nil {
			return "", fmt.Errorf("ssh public key missing: %w", err)
		}
		return strings.TrimSpace(string(pubData)), nil
	}

	if err := os.MkdirAll(m.DataDir, 0700); err != nil {
		return "", err
	}

	_, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return "", fmt.Errorf("failed to generate ssh key: %w", err)
	}

	block, err := ssh.MarshalPrivateKey(privKey, "sandbox@wp-maintenance")
	if err != nil {
		return "", fmt.Errorf("failed to marshal ssh key: %w", err)
	}
	if err := os.WriteFile(priv, pem.EncodeToMemory(block), 0600); err != nil {
		return "", err
	}

	sshPub, err := ssh.NewPublicKey(privKey.Public().(ed25519.PublicKey))
	if err != nil {
		return "", fmt.Errorf("failed to build ssh public key: %w", err)
	}
	authKeys := string(ssh.MarshalAuthorizedKey(sshPub))
	if err := os.WriteFile(pub, []byte(authKeys), 0644); err != nil {
		return "", err
	}

	return strings.TrimSpace(authKeys), nil
}

// ---------------------------------------------------------------------------
// restic repo
// ---------------------------------------------------------------------------

func (m *Manager) resticPaths() (string, string) {
	return filepath.Join(m.DataDir, "restic"), filepath.Join(m.DataDir, "restic_password")
}

func (m *Manager) ensureResticRepo() error {
	repo, pass := m.resticPaths()
	if _, err := os.Stat(pass); os.IsNotExist(err) {
		if err := os.WriteFile(pass, []byte("sandbox-restic-password"), 0600); err != nil {
			return err
		}
	}
	if _, err := os.Stat(filepath.Join(repo, "config")); err == nil {
		return nil
	}
	if err := os.MkdirAll(repo, 0700); err != nil {
		return err
	}
	rc, err := restic.NewClient(repo, pass)
	if err != nil {
		return err
	}
	if err := rc.Init(); err != nil {
		return fmt.Errorf("failed to init sandbox restic repo: %w", err)
	}
	return nil
}

// ---------------------------------------------------------------------------
// public API
// ---------------------------------------------------------------------------

// Start brings the local sandbox WordPress up, seeds it with sample content
// and registers it as a site in the database. It is idempotent: if the stack
// is already running it just returns the current status.
func (m *Manager) Start() (*Status, error) {
	if !dockerAvailable() {
		return nil, fmt.Errorf("docker is required to run the local sandbox site")
	}
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}

	st := m.loadState()
	if st == nil {
		st = &State{LastStart: time.Now().Format(time.RFC3339)}
	}

	pubKey, err := m.ensureSSHKey()
	if err != nil {
		return nil, err
	}
	if err := m.ensureResticRepo(); err != nil {
		return nil, err
	}

	absContext, err := filepath.Abs(m.BuildContext)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve build context: %w", err)
	}
	if err := m.renderCompose(absContext, pubKey); err != nil {
		return nil, err
	}

	if out, err := m.compose("up", "-d", "--build"); err != nil {
		return nil, fmt.Errorf("docker compose up failed: %w\n%s", err, string(out))
	}

	if err := m.waitDB(); err != nil {
		return nil, err
	}
	if err := m.waitHTTP(); err != nil {
		return nil, err
	}

	st.SSHPort = m.containerPort("wp", "22")
	st.HTTPPort = m.containerPort("wp", "80")
	st.HTTPSPort = m.containerPort("wp", "443")
	if st.SSHPort == "" || st.HTTPSPort == "" {
		return nil, fmt.Errorf("failed to resolve sandbox container ports (ssh=%q https=%q)", st.SSHPort, st.HTTPSPort)
	}
	st.HealthURL = "https://localhost:" + st.HTTPSPort

	if err := m.setupWordPress(st); err != nil {
		return nil, err
	}

	siteID, err := m.upsertSite(st)
	if err != nil {
		return nil, err
	}
	st.SiteID = siteID
	st.Broken = false
	st.LastAction = "started"
	if err := m.saveState(st); err != nil {
		return nil, err
	}

	log.Printf("sandbox: started at %s (ssh=%s, https=%s, site=%s)", st.HealthURL, st.SSHPort, st.HTTPSPort, st.SiteID)
	return m.Status(), nil
}

// Break intentionally destroys the sandbox site: core files, uploads, the
// front page and the whole database are wiped so the site stops working. This
// is the "disaster" half of the drill. wp-config.php is kept so the restore
// pipeline can still read the database configuration.
func (m *Manager) Break() (*Status, error) {
	if !dockerAvailable() {
		return nil, fmt.Errorf("docker is required for the sandbox")
	}
	st := m.loadState()
	if st == nil {
		return nil, fmt.Errorf("sandbox has not been started yet")
	}

	script := `rm -rf /var/www/html/wp-includes && rm -rf /var/www/html/wp-content/uploads && printf '%s\n' '<?php /* intentionally broken by sandbox drill */ die( "Site is down for the drill" ); ?>' > /var/www/html/index.php`
	if out, err := m.compose("exec", "-T", "wp", "bash", "-c", script); err != nil {
		return nil, fmt.Errorf("failed to break site files: %w\n%s", err, string(out))
	}

	dropSQL := fmt.Sprintf("DROP DATABASE IF EXISTS `%s`; CREATE DATABASE `%s` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;", dbName, dbName)
	if out, err := m.compose("exec", "-T", "db", "mysql", "-h", "127.0.0.1", "-u", "root", "-prootpass", "-e", dropSQL); err != nil {
		return nil, fmt.Errorf("failed to break database: %w\n%s", err, string(out))
	}

	st.Broken = true
	st.LastAction = "broken"
	m.saveState(st)
	log.Printf("sandbox: site intentionally broken (files and database destroyed)")

	return m.Status(), nil
}

// Status reports whether the sandbox is running and whether the site is
// currently healthy (or intentionally broken).
func (m *Manager) Status() *Status {
	st := m.loadState()
	status := &Status{SiteID: "", HealthURL: "", LastAction: ""}

	if st != nil {
		status.SiteID = st.SiteID
		status.HealthURL = st.HealthURL
		status.Broken = st.Broken
		status.LastAction = st.LastAction
		status.SSHPort = parsePort(st.SSHPort)
	}

	status.Running = m.isRunning()
	if !status.Running {
		status.Message = "not running"
		return status
	}

	status.WPVersion = m.wpVersion()
	status.Healthy = m.isHealthy(st)

	if status.Healthy {
		status.Message = "healthy"
	} else if status.Broken {
		status.Message = "intentionally broken (disaster drill in progress)"
	} else {
		status.Message = "unhealthy"
	}
	return status
}

// Stop tears down the sandbox stack. The site record and backup history are
// kept in the database.
func (m *Manager) Stop() error {
	if !dockerAvailable() {
		return fmt.Errorf("docker is required for the sandbox")
	}
	out, err := m.compose("down", "-v", "--remove-orphans")
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\n%s", err, string(out))
	}
	if st := m.loadState(); st != nil {
		st.Broken = false
		st.LastAction = "stopped"
		m.saveState(st)
	}
	log.Printf("sandbox: stopped")
	return nil
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

func (m *Manager) renderCompose(buildContext, pubKey string) error {
	tmpl, err := os.ReadFile(filepath.Join(buildContext, "docker-compose.sandbox.template.yml"))
	if err != nil {
		return fmt.Errorf("failed to read sandbox compose template: %w", err)
	}

	content := string(tmpl)
	content = strings.ReplaceAll(content, "${BUILD_CONTEXT:-.}", buildContext)
	content = strings.ReplaceAll(content, "${SANDBOX_SSH_AUTHORIZED_KEYS:-}", pubKey)
	content = strings.ReplaceAll(content, "${SANDBOX_DB_NAME:-sandbox}", dbName)
	content = strings.ReplaceAll(content, "${SANDBOX_DB_USER:-sandbox}", dbUser)
	content = strings.ReplaceAll(content, "${SANDBOX_DB_PASSWORD:-sandboxpass}", dbPassword)

	if err := os.MkdirAll(m.DataDir, 0700); err != nil {
		return err
	}
	return os.WriteFile(m.composePath(), []byte(content), 0600)
}

func (m *Manager) isRunning() bool {
	out, err := m.compose("ps", "-q", "wp")
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(out)) != ""
}

func (m *Manager) waitDB() error {
	for i := 0; i < 60; i++ {
		if out, err := m.compose("exec", "-T", "db", "mysqladmin", "ping", "-h", "127.0.0.1", "-u", "root", "-prootpass"); err == nil {
			if strings.Contains(string(out), "mysqld is alive") {
				return nil
			}
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("sandbox database did not become ready within 120s")
}

func (m *Manager) waitHTTP() error {
	port := m.containerPort("wp", "80")
	if port == "" {
		return fmt.Errorf("cannot resolve sandbox http port")
	}
	for i := 0; i < 60; i++ {
		conn, err := net.DialTimeout("tcp", "127.0.0.1:"+port, 2*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(2 * time.Second)
	}
	return fmt.Errorf("sandbox web server did not become ready within 120s")
}

func (m *Manager) wpArgs(args ...string) (string, error) {
	full := append([]string{"exec", "-T", "wp", "wp", "--allow-root", "--path=/var/www/html"}, args...)
	out, err := m.compose(full...)
	if err != nil {
		return "", fmt.Errorf("wp %s failed: %w\n%s", strings.Join(args, " "), err, string(out))
	}
	return strings.TrimSpace(string(out)), nil
}

func (m *Manager) wpArgsRetry(args ...string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		out, err := m.wpArgs(args...)
		if err == nil {
			return out, nil
		}
		lastErr = err
		time.Sleep(time.Duration(attempt+1) * 500 * time.Millisecond)
	}
	return "", lastErr
}

func (m *Manager) wpVersion() string {
	if !m.isRunning() {
		return ""
	}
	out, err := m.wpArgs("core", "version")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(out)
}

func (m *Manager) setupWordPress(st *State) error {
	if _, err := m.wpArgs("core", "is-installed"); err == nil {
		return nil
	}

	if _, err := m.wpArgs(
		"config", "create",
		"--dbname="+dbName,
		"--dbuser="+dbUser,
		"--dbpass="+dbPassword,
		"--dbhost="+dbHost,
		"--skip-check",
	); err != nil {
		return fmt.Errorf("failed to create wp-config.php: %w", err)
	}

	if st.AdminPass == "" {
		st.AdminPass = randomPass()
	}

	if _, err := m.wpArgs(
		"core", "install",
		"--url="+st.HealthURL,
		"--title=Local Sandbox Site",
		"--admin_user=admin",
		"--admin_password="+st.AdminPass,
		"--admin_email=admin@example.com",
		"--skip-email",
	); err != nil {
		return fmt.Errorf("failed to install WordPress: %w", err)
	}

	if _, err := m.wpArgsRetry("option", "update", "siteurl", st.HealthURL); err != nil {
		return fmt.Errorf("failed to update siteurl: %w", err)
	}
	if _, err := m.wpArgsRetry("option", "update", "home", st.HealthURL); err != nil {
		return fmt.Errorf("failed to update home url: %w", err)
	}
	if _, err := m.wpArgsRetry("rewrite", "structure", "/%postname%/"); err != nil {
		return fmt.Errorf("failed to configure permalink structure: %w", err)
	}

	posts := []struct{ title, content string }{
		{"Welcome to the local sandbox", "This production-like test site is used for backup and restore drills."},
		{"Disaster drill day", "We will break this site on purpose and then restore it from a backup."},
		{"Backup and restore round trip", "Backup -> break -> restore -> verify. Everything should be exactly as before."},
	}
	for _, p := range posts {
		if _, err := m.wpArgsRetry("post", "create", "--post_type=post", "--post_status=publish",
			"--post_title="+p.title, "--post_content="+p.content); err != nil {
			return fmt.Errorf("failed to seed sandbox post %q: %w", p.title, err)
		}
	}
	if _, err := m.wpArgsRetry("post", "create", "--post_type=page", "--post_status=publish",
		"--post_title=About the sandbox", "--post_content=Test page created during sandbox provisioning."); err != nil {
		return fmt.Errorf("failed to seed sandbox page: %w", err)
	}

	if out, err := m.compose("exec", "-T", "wp", "bash", "-c",
		`mkdir -p /var/www/html/wp-content/uploads/2026/08 && printf '\377\330\377\340 sandbox-photo' > /var/www/html/wp-content/uploads/2026/08/photo.jpg && chown -R www-data:www-data /var/www/html/wp-content/uploads`); err != nil {
		log.Printf("sandbox: failed to seed uploads dir: %v\n%s", err, string(out))
	}

	if v := m.wpVersion(); v != "" {
		st.WPVersion = v
	}
	return nil
}

func (m *Manager) isHealthy(st *State) bool {
	if st == nil || st.HealthURL == "" || st.HTTPSPort == "" {
		return false
	}

	if _, err := m.compose("exec", "-T", "wp", "test", "-f", "/var/www/html/wp-includes/version.php"); err != nil {
		return false
	}

	// Home page responds with the sandbox title.
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
		CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Get(st.HealthURL + "/")
	if err != nil {
		return false
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || !strings.Contains(string(body), "Local Sandbox Site") {
		return false
	}

	// Published posts exist in the database.
	out, err := m.compose("exec", "-T", "db", "mysql", "-h", "127.0.0.1", "-u", "root", "-prootpass", "-sN", "-e",
		fmt.Sprintf("SELECT COUNT(*) FROM `%s`.`wp_posts` WHERE post_type='post' AND post_status='publish';", dbName))
	if err != nil {
		return false
	}
	count := 0
	fmt.Sscanf(strings.TrimSpace(string(out)), "%d", &count)
	return count >= 3
}

func (m *Manager) upsertSite(st *State) (string, error) {
	sites, err := m.Database.ListSites()
	if err != nil {
		return "", fmt.Errorf("failed to list sites: %w", err)
	}

	repo, pass := m.resticPaths()

	build := func(id string) *db.Site {
		privKeyPath, _ := m.keyPaths()
		return &db.Site{
			ID:                 id,
			Name:               siteName,
			WPSSHHost:          "127.0.0.1",
			WPSSHPort:          parsePort(st.SSHPort),
			WPSSHUser:          "root",
			WPSSHKey:           mustRead(privKeyPath),
			WPRoot:             "/var/www/html",
			DBHost:             dbHost,
			DBUser:             dbUser,
			DBPassword:         dbPassword,
			DBName:             dbName,
			ResticRepository:   repo,
			ResticPasswordFile: pass,
			BackupDir:          filepath.Join(m.DataDir, backupDir()),
			RetentionFlags:     "",
			HealthcheckURL:     st.HealthURL,
			StagingEnabled:     false,
		}
	}

	for _, s := range sites {
		if s.Name == siteName {
			site := build(s.ID)
			if err := m.Database.UpdateSite(site); err != nil {
				return "", fmt.Errorf("failed to update sandbox site: %w", err)
			}
			return s.ID, nil
		}
	}

	site := build(auth.GenerateID())
	if err := m.Database.CreateSite(site); err != nil {
		return "", fmt.Errorf("failed to create sandbox site: %w", err)
	}
	return site.ID, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parsePort(s string) int {
	n := 0
	fmt.Sscanf(s, "%d", &n)
	return n
}

func mustRead(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func randomPass() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "sandbox-admin-pass"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
