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
	"sync"
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
	DataDir       string
	BuildContext  string
	Database      *db.Database
	wpRunner      func(args ...string) (string, error)
	connectedOnce bool

	startMu sync.Mutex
	stream  *logStream
}

// logStream buffers the log lines produced by a Start run and broadcasts them
// to SSE subscribers so the UI can stream startup progress live.
type logStream struct {
	mu      sync.Mutex
	lines   []string
	closed  bool
	success bool
	subs    []chan map[string]interface{}
}

func (s *logStream) add(line string) {
	s.mu.Lock()
	s.lines = append(s.lines, line)
	ev := map[string]interface{}{"line": line}
	subs := s.subs
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

func (s *logStream) close(success bool) {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return
	}
	s.closed = true
	s.success = success
	ev := map[string]interface{}{"done": true, "success": success}
	subs := s.subs
	s.subs = nil
	s.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- ev:
		default:
		}
	}
}

// subscribe replays the lines buffered so far and returns a channel of
// subsequent events. If the stream is already closed, done is true.
func (s *logStream) subscribe() (lines []string, ch <-chan map[string]interface{}, done bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	lines = append([]string{}, s.lines...)
	if s.closed {
		return lines, nil, true
	}
	sub := make(chan map[string]interface{}, 64)
	s.subs = append(s.subs, sub)
	return lines, sub, false
}

type State struct {
	SiteID     string `json:"site_id"`
	WPHost     string `json:"wp_host"`
	SSHPort    string `json:"ssh_port"`
	HTTPPort   string `json:"http_port"`
	HTTPSPort  string `json:"https_port"`
	HealthURL  string `json:"health_url"`
	SiteURL    string `json:"site_url"`
	AdminPass  string `json:"admin_pass"`
	Broken     bool   `json:"broken"`
	WPVersion  string `json:"wp_version"`
	LastStart  string `json:"last_start"`
	LastAction string `json:"last_action"`
}

// Status is returned to the API/UI.
type Status struct {
	Running    bool     `json:"running"`
	SiteID     string   `json:"site_id"`
	HealthURL  string   `json:"health_url"`
	SiteURL    string   `json:"site_url"`
	WPVersion  string   `json:"wp_version"`
	Healthy    bool     `json:"healthy"`
	Broken     bool     `json:"broken"`
	Message    string   `json:"message"`
	SSHPort    int      `json:"ssh_port"`
	LastAction string   `json:"last_action"`
	Logs       []string `json:"logs,omitempty"`
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

// wpContainerIP resolves the IP of the sandbox web container on the sandbox
// network. The API server (whether it runs on the host or in its own
// container) reaches the sandbox over this address, so it must not use the
// host-loopback ports published by the sandbox compose file.
func (m *Manager) wpContainerIP() (string, error) {
	out, err := m.compose("ps", "-q", "wp")
	if err != nil {
		return "", fmt.Errorf("failed to resolve sandbox wp container: %w\n%s", err, string(out))
	}
	cid := strings.TrimSpace(string(out))
	if cid == "" {
		return "", fmt.Errorf("sandbox wp container is not running")
	}

	cmd := exec.Command("docker", "inspect", "-f", "{{range $name, $net := .NetworkSettings.Networks}}{{$name}}={{$net.IPAddress}} {{end}}", cid)
	cmd.Env = os.Environ()
	out, err = cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("failed to inspect sandbox wp container: %w\n%s", err, string(out))
	}

	defaultNet := projectName + "_default"
	fallback := ""
	for _, tok := range strings.Fields(string(out)) {
		parts := strings.SplitN(tok, "=", 2)
		if len(parts) != 2 || parts[1] == "" {
			continue
		}
		if parts[0] == defaultNet {
			return parts[1], nil
		}
		if fallback == "" {
			fallback = parts[1]
		}
	}
	if fallback != "" {
		return fallback, nil
	}
	return "", fmt.Errorf("sandbox wp container has no network address")
}

// apiContainerID returns the ID of the container this process runs in, or ""
// when running directly on the host. Docker sets HOSTNAME to the container ID
// inside containers, so inspecting it tells us whether we are containerized.
func (m *Manager) apiContainerID() string {
	host := strings.TrimSpace(os.Getenv("HOSTNAME"))
	if host == "" {
		return ""
	}
	out, err := exec.Command("docker", "inspect", "-f", "{{.Id}}", host).CombinedOutput()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// ConnectToNetwork attaches the API server container to the sandbox network
// so that SSH/HTTP/SFTP connections to the sandbox wp container work when the
// API runs inside Docker. It is a no-op when the API runs on the host and
// idempotent when the container is already connected. The result is cached in
// memory so repeated calls (every status poll) are cheap; the cache is reset
// when the connection is torn down in Stop or the process restarts, so the
// connection is transparently re-established after an API container recreate.
func (m *Manager) ConnectToNetwork() error {
	if m.connectedOnce {
		return nil
	}
	cid := m.apiContainerID()
	if cid == "" {
		return nil
	}
	if !m.isRunning() {
		return nil
	}
	netName := projectName + "_default"
	cmd := exec.Command("docker", "network", "connect", netName, cid)
	cmd.Env = os.Environ()
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(out)
		if strings.Contains(msg, "already exists") || strings.Contains(msg, "is already") {
			m.connectedOnce = true
			return nil
		}
		return fmt.Errorf("failed to connect API container to sandbox network: %w\n%s", err, msg)
	}
	m.connectedOnce = true
	log.Printf("sandbox: connected API container %s to network %s", cid, netName)
	return nil
}

// disconnectAPIFromNetwork detaches the API container from the sandbox network
// so that `docker compose down -v` can remove it. No-op when running on the host.
func (m *Manager) disconnectAPIFromNetwork() {
	m.connectedOnce = false
	cid := m.apiContainerID()
	if cid == "" {
		return
	}
	cmd := exec.Command("docker", "network", "disconnect", projectName+"_default", cid)
	cmd.Env = os.Environ()
	if err := cmd.Run(); err != nil {
		log.Printf("sandbox: failed to disconnect API container from sandbox network: %v", err)
	}
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
//
// publicBase is the externally reachable base of the application (e.g.
// "https://192.168.1.116"); the sandbox site is then published under
// publicBase + "/sandbox-site" so it can be viewed from the browser even
// though the sandbox itself only listens on the Docker-internal network.
func (m *Manager) Start(publicBase string) (ret *Status, err error) {
	if !dockerAvailable() {
		return nil, fmt.Errorf("docker is required to run the local sandbox site")
	}
	if err := m.ensureDirs(); err != nil {
		return nil, err
	}

	stream := &logStream{}
	m.startMu.Lock()
	m.stream = stream
	m.startMu.Unlock()
	defer func() {
		stream.close(err == nil)
		m.startMu.Lock()
		m.stream = nil
		m.startMu.Unlock()
	}()

	logs := []string{}
	startLog := func(format string, args ...interface{}) {
		msg := fmt.Sprintf(format, args...)
		line := time.Now().Format("15:04:05") + " " + msg
		logs = append(logs, line)
		stream.add(line)
		log.Printf("sandbox: %s", msg)
	}

	startLog("preparing sandbox start")

	st := m.loadState()
	if st == nil {
		st = &State{LastStart: time.Now().Format(time.RFC3339)}
	}
	startLog("ensuring SSH key and restic repository")

	pubKey, err := m.ensureSSHKey()
	if err != nil {
		return nil, err
	}
	if err := m.ensureResticRepo(); err != nil {
		return nil, err
	}

	startLog("rendering sandbox compose file")
	absContext, err := filepath.Abs(m.BuildContext)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve build context: %w", err)
	}
	if err := m.renderCompose(absContext, pubKey); err != nil {
		return nil, err
	}

	startLog("starting sandbox containers")
	if out, err := m.compose("up", "-d", "--build"); err != nil {
		return nil, fmt.Errorf("docker compose up failed: %w\n%s", err, string(out))
	}

	startLog("waiting for sandbox database")
	if err := m.waitDB(); err != nil {
		return nil, err
	}
	startLog("connecting the API server to the sandbox network")
	m.connectedOnce = false
	if err := m.ConnectToNetwork(); err != nil {
		return nil, err
	}

	startLog("resolving sandbox container address")
	wpIP, err := m.wpContainerIP()
	if err != nil {
		return nil, err
	}

	startLog("waiting for sandbox web server")
	if err := m.waitHTTP(wpIP); err != nil {
		return nil, err
	}

	st.WPHost = wpIP
	st.SSHPort = "22"
	st.HTTPPort = "80"
	st.HTTPSPort = "443"
	st.HealthURL = "https://" + wpIP
	st.SiteURL = m.siteURL(publicBase)

	startLog("provisioning WordPress content")
	if err := m.setupWordPress(st); err != nil {
		return nil, err
	}

	startLog("applying sandbox site URL")
	if err := m.ensureSiteURL(st); err != nil {
		return nil, err
	}

	startLog("registering sandbox site in the database")
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

	startLog("sandbox started (site=%s, health=%s, site_url=%s)", st.SiteID, st.HealthURL, st.SiteURL)
	status := m.Status()
	status.Logs = logs
	return status, nil
}

// StreamStartLogs serves the running Start's progress as Server-Sent Events so
// the UI can show startup logs live. Each event is a JSON object with a
// "line" field, and a final {"done":true,"success":bool} event when the start
// finishes. If no start is in progress the done event is sent right away.
func (m *Manager) StreamStartLogs(w http.ResponseWriter, r *http.Request) error {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming unsupported")
	}

	writeEvent := func(ev map[string]interface{}) error {
		b, err := json.Marshal(ev)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(w, "data: %s\n\n", b); err != nil {
			return err
		}
		flusher.Flush()
		return nil
	}

	stream := func() *logStream {
		m.startMu.Lock()
		defer m.startMu.Unlock()
		return m.stream
	}()

	// The start request may not have created the stream yet; wait briefly.
	for i := 0; stream == nil && i < 50; i++ {
		select {
		case <-r.Context().Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
		stream = func() *logStream {
			m.startMu.Lock()
			defer m.startMu.Unlock()
			return m.stream
		}()
	}
	if stream == nil {
		return writeEvent(map[string]interface{}{"done": true, "success": true})
	}

	lines, ch, done := stream.subscribe()
	for _, line := range lines {
		if err := writeEvent(map[string]interface{}{"line": line}); err != nil {
			return err
		}
	}
	if done {
		return writeEvent(map[string]interface{}{"done": true, "success": stream.success})
	}

	for {
		select {
		case ev := <-ch:
			if err := writeEvent(ev); err != nil {
				return err
			}
			if ev["done"] == true {
				return nil
			}
		case <-r.Context().Done():
			return nil
		}
	}
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
	if err := m.saveState(st); err != nil {
		return nil, fmt.Errorf("failed to save sandbox state after break: %w", err)
	}
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
		status.SiteURL = st.SiteURL
		status.Broken = st.Broken
		status.LastAction = st.LastAction
		status.SSHPort = parsePort(st.SSHPort)
	}

	status.Running = m.isRunning()
	if !status.Running {
		status.Message = "not running"
		return status
	}

	if err := m.ConnectToNetwork(); err != nil {
		log.Printf("sandbox: warning: %v", err)
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

// IsSandboxSiteID reports whether the given site ID belongs to the local
// sandbox. Used by the job worker to keep the sandbox state in sync after a
// restore.
func (m *Manager) IsSandboxSiteID(siteID string) bool {
	st := m.loadState()
	return st != nil && st.SiteID == siteID
}

// MarkRestored records that the sandbox site was successfully restored after a
// disaster drill, clearing the intentionally-broken flag.
func (m *Manager) MarkRestored() error {
	st := m.loadState()
	if st == nil {
		return nil
	}
	if !st.Broken {
		return nil
	}
	st.Broken = false
	st.LastAction = "restored"
	if err := m.saveState(st); err != nil {
		return fmt.Errorf("failed to save sandbox state after restore: %w", err)
	}
	log.Printf("sandbox: site restored from backup")
	return nil
}

// siteURL builds the externally reachable URL of the sandbox site from the
// public base of the application. Returns "" when no public base is known, in
// which case callers fall back to the internal HealthURL.
func (m *Manager) siteURL(publicBase string) string {
	base := strings.TrimRight(strings.TrimSpace(publicBase), "/")
	if base == "" {
		return ""
	}
	return base + "/sandbox-site"
}

// ensureSiteURL makes sure WordPress is configured with the sandbox's public
// site URL (siteurl and home options). It is idempotent and also covers the
// case where the sandbox was started earlier with a different URL.
func (m *Manager) ensureSiteURL(st *State) error {
	if st.SiteURL == "" {
		return nil
	}
	if _, err := m.wpArgsRetry("option", "update", "siteurl", st.SiteURL); err != nil {
		return fmt.Errorf("failed to update siteurl: %w", err)
	}
	if _, err := m.wpArgsRetry("option", "update", "home", st.SiteURL); err != nil {
		return fmt.Errorf("failed to update home url: %w", err)
	}
	return nil
}

// ReapplySiteURL re-applies the sandbox's public site URL after a restore, in
// case the restored database carries a siteurl/home from an earlier run.
func (m *Manager) ReapplySiteURL() error {
	if !m.isRunning() {
		return nil
	}
	st := m.loadState()
	if st == nil {
		return nil
	}
	if err := m.ensureSiteURL(st); err != nil {
		return fmt.Errorf("failed to re-apply site url after restore: %w", err)
	}
	if err := m.ensureRewriteRules(); err != nil {
		log.Printf("sandbox: warning: failed to re-apply rewrite rules after restore: %v", err)
	}
	log.Printf("sandbox: re-applied site url %s", st.SiteURL)
	return nil
}

// ensureRewriteRules writes the standard WordPress pretty-permalink rules to
// the sandbox's .htaccess. The site is served through the application under a
// URL prefix, but the proxy strips that prefix, so the rules target a root
// install (RewriteBase /). wp-cli will not write the file itself, so it is
// written directly. This also covers restores from backups taken before the
// .htaccess existed.
func (m *Manager) ensureRewriteRules() error {
	htaccess := `# BEGIN WordPress
<IfModule mod_rewrite.c>
RewriteEngine On
RewriteBase /
RewriteRule ^index\.php$ - [L]
RewriteCond %{REQUEST_FILENAME} !-f
RewriteCond %{REQUEST_FILENAME} !-d
RewriteRule . /index.php [L]
</IfModule>
# END WordPress`
	htaccessB64 := base64.StdEncoding.EncodeToString([]byte(htaccess))
	out, err := m.compose("exec", "-T", "wp", "bash", "-c",
		"echo "+htaccessB64+" | base64 -d > /var/www/html/.htaccess && chown www-data:www-data /var/www/html/.htaccess")
	if err != nil {
		return fmt.Errorf("failed to write sandbox .htaccess: %w\n%s", err, string(out))
	}
	return nil
}

// SiteProxyTarget returns the base address of the sandbox web server on the
// sandbox network. The API server uses it to serve the sandbox site through
// the application at /sandbox-site, so the site is viewable from the browser
// even though it only listens on the Docker-internal network. HTTPS is used so
// WordPress believes the request is secure and generates https:// asset URLs
// instead of mixing http:// links into the page.
func (m *Manager) SiteProxyTarget() (string, error) {
	if !m.isRunning() {
		return "", fmt.Errorf("sandbox is not running")
	}
	ip, err := m.wpContainerIP()
	if err != nil {
		return "", err
	}
	return "https://" + ip, nil
}

// Stop tears down the sandbox stack. The site record and backup history are
// kept in the database.
func (m *Manager) Stop() error {
	if !dockerAvailable() {
		return fmt.Errorf("docker is required for the sandbox")
	}
	m.disconnectAPIFromNetwork()
	out, err := m.compose("down", "-v", "--remove-orphans")
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\n%s", err, string(out))
	}
	if st := m.loadState(); st != nil {
		st.Broken = false
		st.LastAction = "stopped"
		if err := m.saveState(st); err != nil {
			return fmt.Errorf("failed to save sandbox state after stop: %w", err)
		}
	}
	log.Printf("sandbox: stopped")
	return nil
}

// Delete removes the test site entirely: it tears down the running containers,
// deletes the site record together with its backups and jobs from the
// database, and wipes the sandbox data directory (SSH key, restic repository,
// downloaded backups and state). The next Start provisions a brand new site.
func (m *Manager) Delete() error {
	if !dockerAvailable() {
		return fmt.Errorf("docker is required for the sandbox")
	}

	st := m.loadState()

	if m.isRunning() {
		m.disconnectAPIFromNetwork()
		out, err := m.compose("down", "-v", "--remove-orphans")
		if err != nil {
			return fmt.Errorf("docker compose down failed: %w\n%s", err, string(out))
		}
	}

	if st != nil && st.SiteID != "" {
		if err := m.Database.DeleteSite(st.SiteID); err != nil {
			return fmt.Errorf("failed to delete sandbox site record: %w", err)
		}
		log.Printf("sandbox: deleted site record %s", st.SiteID)
	}

	entries, err := os.ReadDir(m.DataDir)
	if err != nil {
		return fmt.Errorf("failed to read sandbox data directory: %w", err)
	}
	for _, e := range entries {
		// DataDir is a mounted volume, so the mountpoint itself cannot be
		// removed; clear its contents instead. This wipes the SSH key, the
		// restic repository, downloaded backups, state and compose file.
		if err := os.RemoveAll(filepath.Join(m.DataDir, e.Name())); err != nil {
			return fmt.Errorf("failed to remove sandbox data %q: %w", e.Name(), err)
		}
	}

	log.Printf("sandbox: test site deleted (containers stopped, site record and sandbox data removed)")
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

func (m *Manager) waitHTTP(addr string) error {
	for i := 0; i < 60; i++ {
		conn, err := net.DialTimeout("tcp", addr+":80", 2*time.Second)
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

func (m *Manager) runWpArgs(args ...string) (string, error) {
	if m.wpRunner != nil {
		return m.wpRunner(args...)
	}
	return m.wpArgs(args...)
}

func (m *Manager) wpArgsRetry(args ...string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		out, err := m.runWpArgs(args...)
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

	wpURL := st.SiteURL
	if wpURL == "" {
		wpURL = st.HealthURL
	}

	if _, err := m.wpArgs(
		"core", "install",
		"--url="+wpURL,
		"--title=Local Sandbox Site",
		"--admin_user=admin",
		"--admin_password="+st.AdminPass,
		"--admin_email=admin@example.com",
		"--skip-email",
	); err != nil {
		return fmt.Errorf("failed to install WordPress: %w", err)
	}

	if _, err := m.wpArgsRetry("option", "update", "siteurl", wpURL); err != nil {
		return fmt.Errorf("failed to update siteurl: %w", err)
	}
	if _, err := m.wpArgsRetry("option", "update", "home", wpURL); err != nil {
		return fmt.Errorf("failed to update home url: %w", err)
	}
	if _, err := m.wpArgsRetry("rewrite", "structure", "/%postname%/"); err != nil {
		return fmt.Errorf("failed to configure permalink structure: %w", err)
	}
	if err := m.ensureRewriteRules(); err != nil {
		return err
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
		sshHost := st.WPHost
		if sshHost == "" {
			sshHost = "127.0.0.1"
		}
		return &db.Site{
			ID:                 id,
			Name:               siteName,
			WPSSHHost:          sshHost,
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
	data, err := os.ReadFile(path)
	if err != nil {
		log.Printf("error reading file %s: %v", path, err)
		return ""
	}
	return strings.TrimSpace(string(data))
}

func randomPass() string {
	buf := make([]byte, 12)
	if _, err := rand.Read(buf); err != nil {
		return "sandbox-admin-pass"
	}
	return base64.RawURLEncoding.EncodeToString(buf)
}
