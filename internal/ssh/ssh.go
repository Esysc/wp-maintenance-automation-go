package ssh

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"
)

const defaultConnectTimeout = 15 * time.Second

type SSHOptions struct {
	Host string
	Port int
	User string
	Key  string
}

func NewSSHOptions(host, user string, port int) *SSHOptions {
	return &SSHOptions{Host: host, User: user, Port: port}
}

// Client is the interface used by the job pipeline and API handlers to
// interact with a remote WordPress host. The concrete *SSHClient talks over
// SSH; tests may inject a fake implementation backed by local directories.
type Client interface {
	Close() error
	DSN() string
	RunCommand(cmd string) (string, error)
	RunCommandRaw(cmd string) string
	FileExists(remotePath string) (bool, error)
	CommandExists(cmd string) (bool, error)
	DetectWPRoot() (string, error)
	GetWpVersion(wpRoot string) (string, error)
	ParseDBConfig(wpRoot string) (string, string, string, string, error)
	DownloadFile(remotePath, localPath string) error
	UploadFile(localPath, remotePath string) error
	SyncDir(sourceDir, destDir string, sourceIsRemote bool, excludes []string, delete bool, onFile func(string)) error
	CountRemoteFiles(dir string) (int, error)
	RemoveRemoteFile(remotePath string) error
}

type SSHClient struct {
	opts      *SSHOptions
	conn      *ssh.Client
	sftpConn  *sftp.Client
	connected bool
	authErr   error
}

func NewClient(opts *SSHOptions) *SSHClient {
	return &SSHClient{opts: opts}
}

func (c *SSHClient) DSN() string {
	return fmt.Sprintf("%s@%s", c.opts.User, c.opts.Host)
}

func (c *SSHClient) connect() error {
	if c.connected {
		return nil
	}

	addr := fmt.Sprintf("%s:%d", c.opts.Host, c.opts.Port)
	if c.opts.Port == 0 || c.opts.Port == 22 {
		addr = fmt.Sprintf("%s:22", c.opts.Host)
	}

	config := &ssh.ClientConfig{
		User:            c.opts.User,
		Auth:            c.authMethods(),
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         defaultConnectTimeout,
	}

	if c.authErr != nil {
		return c.authErr
	}
	if len(config.Auth) == 0 {
		return fmt.Errorf("SSH connection failed: no authentication methods available")
	}

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	return nil
}

func (c *SSHClient) authMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod
	c.authErr = nil

	if c.opts.Key != "" {
		key := normalizeKeyMaterial(c.opts.Key)
		if looksLikePublicKey(key) {
			c.authErr = fmt.Errorf("SSH connection failed: provided ssh_key is a public key. Paste the PRIVATE key (BEGIN/END ... PRIVATE KEY)")
			return nil
		}
		if signer, err := ssh.ParsePrivateKey([]byte(key)); err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
			return methods
		}

		if keyFile := strings.TrimSpace(c.opts.Key); isLocalPath(keyFile) {
			keyBytes, err := os.ReadFile(keyFile)
			if err != nil {
				c.authErr = fmt.Errorf("SSH connection failed: provided ssh_key path cannot be read: %s", keyFile)
				return nil
			}
			if signer, err := ssh.ParsePrivateKey(keyBytes); err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
				return methods
			}
			c.authErr = fmt.Errorf("SSH connection failed: provided ssh_key path is not a valid unencrypted private key: %s", keyFile)
			return nil
		}

		c.authErr = fmt.Errorf("SSH connection failed: provided ssh_key is not a valid unencrypted private key")
		return nil
	}

	if sock := os.Getenv("SSH_AUTH_SOCK"); sock != "" {
		if agentConn, err := net.Dial("unix", sock); err == nil {
			methods = append(methods, ssh.PublicKeysCallback(agent.NewClient(agentConn).Signers))
		}
	}

	for _, keyPath := range []string{
		filepath.Join(os.Getenv("HOME"), ".ssh/id_rsa"),
		filepath.Join(os.Getenv("HOME"), ".ssh/id_ed25519"),
		filepath.Join(os.Getenv("HOME"), ".ssh/id_ecdsa"),
		filepath.Join(os.Getenv("HOME"), ".ssh/id_dsa"),
	} {
		if key, err := os.ReadFile(keyPath); err == nil {
			if signer, err := ssh.ParsePrivateKey(key); err == nil {
				methods = append(methods, ssh.PublicKeys(signer))
			}
		}
	}

	return methods
}

func normalizeKeyMaterial(key string) string {
	trimmed := strings.TrimSpace(key)
	if strings.Contains(trimmed, `\n`) && !strings.Contains(trimmed, "\n") {
		trimmed = strings.ReplaceAll(trimmed, `\n`, "\n")
	}
	return strings.ReplaceAll(trimmed, "\r\n", "\n")
}

func looksLikePublicKey(s string) bool {
	s = strings.TrimSpace(s)
	for _, prefix := range []string{
		"ssh-ed25519 ",
		"ssh-rsa ",
		"ecdsa-sha2-",
		"sk-ssh-ed25519@openssh.com ",
		"sk-ecdsa-sha2-nistp256@openssh.com ",
	} {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}

func (c *SSHClient) getSFTP() (*sftp.Client, error) {
	if c.sftpConn != nil {
		return c.sftpConn, nil
	}
	if err := c.connect(); err != nil {
		return nil, err
	}
	sftpConn, err := sftp.NewClient(c.conn)
	if err != nil {
		return nil, fmt.Errorf("SFTP client failed: %w", err)
	}
	c.sftpConn = sftpConn
	return sftpConn, nil
}

func (c *SSHClient) Close() error {
	if c.sftpConn != nil {
		c.sftpConn.Close()
		c.sftpConn = nil
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *SSHClient) RunCommand(cmd string) (string, error) {
	if err := c.connect(); err != nil {
		return "", err
	}
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %w\nOutput: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *SSHClient) RunCommandRaw(cmd string) string {
	out, err := c.RunCommand(cmd)
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	return out
}

func (c *SSHClient) RunCommandWithEnv(cmd string, env []string) (string, error) {
	if err := c.connect(); err != nil {
		return "", err
	}
	session, err := c.conn.NewSession()
	if err != nil {
		return "", fmt.Errorf("SSH session failed: %w", err)
	}
	defer session.Close()

	for _, e := range env {
		parts := strings.SplitN(e, "=", 2)
		if len(parts) == 2 {
			session.Setenv(parts[0], parts[1])
		}
	}

	output, err := session.CombinedOutput(cmd)
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %w\nOutput: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *SSHClient) FileExists(remotePath string) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("test -f '%s' && echo 'exists'", remotePath))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(output) == "exists", nil
}

func (c *SSHClient) CommandExists(cmd string) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("command -v %s", cmd))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(output) != "", nil
}

func (c *SSHClient) ReadRemoteFile(remotePath string) (string, error) {
	return c.RunCommand(fmt.Sprintf("cat '%s'", remotePath))
}

func (c *SSHClient) WriteRemoteFile(remotePath, content string) error {
	cmd := fmt.Sprintf("cat > '%s' << 'EOF'\n%s\nEOF", remotePath, content)
	_, err := c.RunCommand(cmd)
	return err
}

func (c *SSHClient) RemoveRemoteFile(remotePath string) error {
	_, err := c.RunCommand(fmt.Sprintf("rm -f '%s'", remotePath))
	return err
}

func (c *SSHClient) WPCLI(wpRoot, args string) (string, error) {
	return c.RunCommand(fmt.Sprintf("cd '%s' && %s %s", wpRoot, "wp", args))
}

func (c *SSHClient) GetWpVersion(wpRoot string) (string, error) {
	ver, err := c.WPCLI(wpRoot, "core version")
	if err == nil {
		ver = strings.TrimSpace(ver)
		if ver != "" {
			return ver, nil
		}
	}

	content, err := c.ReadRemoteFile(filepath.Join(wpRoot, "wp-includes", "version.php"))
	if err != nil {
		return "", fmt.Errorf("could not determine WordPress version: %w", err)
	}

	ver = parseWpVersion(content)
	if ver == "" {
		return "", fmt.Errorf("could not find WordPress version in version.php")
	}
	return ver, nil
}

func parseWpVersion(content string) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$wp_version") {
			start := strings.Index(line, "'")
			if start == -1 {
				continue
			}
			end := strings.Index(line[start+1:], "'")
			if end == -1 {
				continue
			}
			return line[start+1 : start+1+end]
		}
	}
	return ""
}

func (c *SSHClient) ParseDBConfig(wpRoot string) (string, string, string, string, error) {
	var readErrors []string
	for _, cfgPath := range candidateWPConfigPaths(wpRoot) {
		content, err := c.ReadRemoteFile(cfgPath)
		if err != nil {
			readErrors = append(readErrors, fmt.Sprintf("%s: %v", cfgPath, err))
			continue
		}

		dbName := parseDefineValue(content, "DB_NAME")
		dbUser := parseDefineValue(content, "DB_USER")
		dbPass := parseDefineValue(content, "DB_PASSWORD")
		dbHost := parseDefineValue(content, "DB_HOST")

		if dbName != "" && dbUser != "" && dbHost != "" {
			return dbName, dbUser, dbPass, dbHost, nil
		}
		readErrors = append(readErrors, fmt.Sprintf("%s: DB constants incomplete", cfgPath))
	}

	dbName, dbUser, dbPass, dbHost, err := c.parseDBConfigViaWPCLI(wpRoot)
	if err == nil {
		return dbName, dbUser, dbPass, dbHost, nil
	}

	if len(readErrors) > 0 {
		return "", "", "", "", fmt.Errorf("could not parse DB config from wp-config.php (read attempts: %s; wp-cli: %v)", strings.Join(readErrors, " | "), err)
	}
	return "", "", "", "", err
}

func candidateWPConfigPaths(wpRoot string) []string {
	cleanRoot := strings.TrimSpace(filepath.Clean(wpRoot))
	parent := filepath.Dir(cleanRoot)
	paths := []string{filepath.Join(cleanRoot, "wp-config.php")}
	if parent != "." && parent != cleanRoot {
		paths = append(paths, filepath.Join(parent, "wp-config.php"))
	}
	return paths
}

func (c *SSHClient) parseDBConfigViaWPCLI(wpRoot string) (string, string, string, string, error) {
	keys := []string{"DB_NAME", "DB_USER", "DB_PASSWORD", "DB_HOST"}
	values := make([]string, 0, len(keys))

	for _, key := range keys {
		out, err := c.RunCommand(fmt.Sprintf(
			"cd %s && wp config get %s --type=constant --quiet 2>/dev/null", shellQuote(wpRoot), key,
		))
		if err != nil {
			return "", "", "", "", fmt.Errorf("could not parse DB config from wp-config.php")
		}
		values = append(values, strings.TrimSpace(out))
	}

	if values[0] == "" || values[1] == "" || values[3] == "" {
		return "", "", "", "", fmt.Errorf("incomplete DB config values from WP-CLI")
	}

	return values[0], values[1], values[2], values[3], nil
}

func parseDefineValue(content, key string) string {
	quotedKey := regexp.QuoteMeta(key)
	patterns := []string{
		fmt.Sprintf(`(?m)define\(\s*['\"]%s['\"]\s*,\s*'([^']*)'\s*\)`, quotedKey),
		fmt.Sprintf(`(?m)define\(\s*['\"]%s['\"]\s*,\s*"([^"]*)"\s*\)`, quotedKey),
	}

	for _, pattern := range patterns {
		re := regexp.MustCompile(pattern)
		matches := re.FindStringSubmatch(content)
		if len(matches) == 2 {
			return strings.TrimSpace(matches[1])
		}
	}
	return ""
}

func (c *SSHClient) UploadFile(localPath, remotePath string) error {
	sf, err := c.getSFTP()
	if err != nil {
		return err
	}

	localFile, err := os.Open(localPath)
	if err != nil {
		return fmt.Errorf("cannot open local file: %w", err)
	}
	defer localFile.Close()

	if err := sf.MkdirAll(filepath.Dir(remotePath)); err != nil {
		return fmt.Errorf("cannot create remote directory: %w", err)
	}

	remoteFile, err := sf.Create(remotePath)
	if err != nil {
		return fmt.Errorf("cannot create remote file: %w", err)
	}
	defer remoteFile.Close()

	if _, err := io.Copy(remoteFile, localFile); err != nil {
		return fmt.Errorf("file upload failed: %w", err)
	}
	return nil
}

func (c *SSHClient) DownloadFile(remotePath, localPath string) error {
	sf, err := c.getSFTP()
	if err != nil {
		return err
	}

	remoteFile, err := sf.Open(remotePath)
	if err != nil {
		return fmt.Errorf("cannot open remote file: %w", err)
	}
	defer remoteFile.Close()

	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return fmt.Errorf("cannot create local directory: %w", err)
	}

	localFile, err := os.Create(localPath)
	if err != nil {
		return fmt.Errorf("cannot create local file: %w", err)
	}
	defer localFile.Close()

	if _, err := io.Copy(localFile, remoteFile); err != nil {
		return fmt.Errorf("file download failed: %w", err)
	}
	return nil
}

func (c *SSHClient) SyncDir(sourceDir, destDir string, sourceIsRemote bool, excludes []string, delete bool, onFile func(string)) error {
	if err := c.connect(); err != nil {
		return err
	}

	excludeSet := make(map[string]bool)
	for _, e := range excludes {
		if e != "" {
			excludeSet[e] = true
		}
	}

	isExcluded := func(name string) bool {
		base := filepath.Base(name)
		for e := range excludeSet {
			if matched, _ := filepath.Match(e, base); matched {
				return true
			}
		}
		return false
	}

	if sourceIsRemote {
		return c.downloadTar(sourceDir, destDir, isExcluded, onFile)
	}
	return c.uploadTar(sourceDir, destDir, isExcluded, delete, onFile)
}

func (c *SSHClient) downloadTar(sourceDir, destDir string, isExcluded func(string) bool, onFile func(string)) error {
	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	cmd := fmt.Sprintf("tar cf - -C %s .", shellQuote(sourceDir))

	stdout, err := session.StdoutPipe()
	if err != nil {
		return err
	}

	if err := session.Start(cmd); err != nil {
		return err
	}

	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	tr := tar.NewReader(stdout)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		name := filepath.Clean(header.Name)
		if name == "." {
			continue
		}
		if isExcluded(name) {
			continue
		}

		if onFile != nil {
			onFile(name)
		}

		target := filepath.Join(destDir, name)

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, os.FileMode(header.Mode&0o777)); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
				return err
			}
			f, err := os.OpenFile(target, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, os.FileMode(header.Mode&0o777))
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
		}
	}

	return session.Wait()
}

func (c *SSHClient) uploadTar(sourceDir, destDir string, isExcluded func(string) bool, delete bool, onFile func(string)) error {
	if _, err := c.RunCommand(fmt.Sprintf("mkdir -p %s", shellQuote(destDir))); err != nil {
		return fmt.Errorf("cannot create remote directory: %w", err)
	}

	var remoteFiles map[string]struct{}
	if delete {
		remoteFiles = make(map[string]struct{})
		out, err := c.RunCommand(fmt.Sprintf("cd %s && find . -type f 2>/dev/null || true", shellQuote(destDir)))
		if err == nil && out != "" {
			for _, f := range strings.Split(out, "\n") {
				f = strings.TrimPrefix(strings.TrimSpace(f), "./")
				if f != "" {
					remoteFiles[f] = struct{}{}
				}
			}
		}
	}

	session, err := c.conn.NewSession()
	if err != nil {
		return err
	}
	defer session.Close()

	stdin, err := session.StdinPipe()
	if err != nil {
		return err
	}

	cmd := fmt.Sprintf("tar xf - -C %s", shellQuote(destDir))
	if err := session.Start(cmd); err != nil {
		return err
	}

	tw := tar.NewWriter(stdin)
	sentFiles := make(map[string]struct{})

	err = filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(sourceDir, path)
		if relPath == "." {
			return nil
		}
		if isExcluded(relPath) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		if onFile != nil {
			onFile(relPath)
		}

		info, err := os.Stat(path)
		if err != nil {
			return err
		}

		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
			return tw.WriteHeader(header)
		}

		sentFiles[relPath] = struct{}{}

		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}
	if err := stdin.Close(); err != nil {
		return err
	}

	if err := session.Wait(); err != nil {
		return err
	}

	if delete && remoteFiles != nil {
		for f := range remoteFiles {
			if _, sent := sentFiles[f]; !sent {
				c.RunCommand(fmt.Sprintf("rm -f %s", shellQuote(filepath.Join(destDir, f))))
			}
		}
		c.RunCommand(fmt.Sprintf("find %s -type d -empty -delete 2>/dev/null || true", shellQuote(destDir)))
	}

	return nil
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}

func isLocalPath(p string) bool {
	p = strings.TrimSpace(p)
	if p == "" {
		return false
	}
	if strings.ContainsAny(p, "\n\r\t") {
		return false
	}
	if !(strings.HasPrefix(p, "/") || strings.HasPrefix(p, "./") || strings.HasPrefix(p, "../") || strings.HasPrefix(p, "~/") || strings.Contains(p, "/")) {
		return false
	}
	_, err := os.Stat(p)
	return err == nil
}

func (c *SSHClient) CountRemoteFiles(dir string) (int, error) {
	out, err := c.RunCommand(fmt.Sprintf("find %s -type f 2>/dev/null | wc -l", shellQuote(dir)))
	if err != nil {
		return 0, err
	}
	count := 0
	fmt.Sscanf(strings.TrimSpace(out), "%d", &count)
	return count, nil
}

func (c *SSHClient) DetectWPRoot() (string, error) {
	loginDir := c.detectLoginDir()
	homeDir := c.detectHomeDir()

	commonPaths := []string{
		loginDir,
		loginDir + "/html",
		loginDir + "/public_html",
		loginDir + "/www",
		loginDir + "/htdocs",
		loginDir + "/wordpress",
		loginDir + "/wp",
		homeDir,
		homeDir + "/html",
		homeDir + "/public_html",
		homeDir + "/www",
		homeDir + "/htdocs",
		homeDir + "/sites",
		homeDir + "/wordpress",
		homeDir + "/wp",
		"/var/www/html",
		"/var/www",
		"/var/www/wordpress",
		"/srv/www",
		"/srv/www/html",
		"/usr/share/nginx/html",
		"/opt/bitnami/wordpress",
	}

	for _, p := range commonPaths {
		exists, _ := c.FileExists(p + "/wp-config.php")
		if exists {
			return p, nil
		}
		exists, _ = c.FileExists(filepath.Join(p, "wp-includes", "version.php"))
		if exists {
			return p, nil
		}
	}

	for _, dir := range []string{loginDir, homeDir, "/var", "/srv", "/opt", "/usr", "/home"} {
		if strings.TrimSpace(dir) == "" {
			continue
		}
		out, err := c.RunCommand(fmt.Sprintf("find %s -maxdepth 12 -type f \\( -name wp-config.php -o -path '*/wp-includes/version.php' \\) 2>/dev/null || true", shellQuote(dir)))
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				if strings.HasSuffix(line, "/wp-includes/version.php") {
					return filepath.Dir(filepath.Dir(line)), nil
				}
				return filepath.Dir(line), nil
			}
		}
	}

	return "", fmt.Errorf("could not find WordPress installation on remote host")
}

func (c *SSHClient) detectLoginDir() string {
	for _, cmd := range []string{
		"pwd -P",
		"pwd",
	} {
		out, err := c.RunCommand(cmd + " || true")
		if err == nil && out != "" {
			return strings.TrimSpace(out)
		}
	}
	return ""
}

func (c *SSHClient) detectHomeDir() string {
	for _, cmd := range []string{
		"cd ~ && pwd",
		"echo ~",
		"echo $HOME",
		"getent passwd $(whoami 2>/dev/null) 2>/dev/null | cut -d: -f6",
		"pwd",
	} {
		out, err := c.RunCommand(cmd + " || true")
		if err == nil && out != "" {
			return out
		}
	}
	return "/root"
}
