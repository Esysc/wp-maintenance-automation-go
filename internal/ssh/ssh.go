package ssh

import (
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
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

type Client struct {
	opts      *SSHOptions
	conn      *ssh.Client
	sftpConn  *sftp.Client
	connected bool
}

func NewClient(opts *SSHOptions) *Client {
	return &Client{opts: opts}
}

func (c *Client) DSN() string {
	return fmt.Sprintf("%s@%s", c.opts.User, c.opts.Host)
}

func (c *Client) connect() error {
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

	conn, err := ssh.Dial("tcp", addr, config)
	if err != nil {
		return fmt.Errorf("SSH connection failed: %w", err)
	}

	c.conn = conn
	c.connected = true
	return nil
}

func (c *Client) authMethods() []ssh.AuthMethod {
	var methods []ssh.AuthMethod

	if c.opts.Key != "" {
		if signer, err := ssh.ParsePrivateKey([]byte(c.opts.Key)); err == nil {
			methods = append(methods, ssh.PublicKeys(signer))
			return methods
		}
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

func (c *Client) getSFTP() (*sftp.Client, error) {
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

func (c *Client) Close() error {
	if c.sftpConn != nil {
		c.sftpConn.Close()
		c.sftpConn = nil
	}
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

func (c *Client) RunCommand(cmd string) (string, error) {
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

func (c *Client) RunCommandRaw(cmd string) string {
	out, err := c.RunCommand(cmd)
	if err != nil {
		return fmt.Sprintf("ERROR: %v", err)
	}
	return out
}

func (c *Client) RunCommandWithEnv(cmd string, env []string) (string, error) {
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

func (c *Client) FileExists(remotePath string) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("test -f '%s' && echo 'exists'", remotePath))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(output) == "exists", nil
}

func (c *Client) CommandExists(cmd string) (bool, error) {
	output, err := c.RunCommand(fmt.Sprintf("command -v %s", cmd))
	if err != nil {
		return false, nil
	}
	return strings.TrimSpace(output) != "", nil
}

func (c *Client) ReadRemoteFile(remotePath string) (string, error) {
	return c.RunCommand(fmt.Sprintf("cat '%s'", remotePath))
}

func (c *Client) WriteRemoteFile(remotePath, content string) error {
	cmd := fmt.Sprintf("cat > '%s' << 'EOF'\n%s\nEOF", remotePath, content)
	_, err := c.RunCommand(cmd)
	return err
}

func (c *Client) RemoveRemoteFile(remotePath string) error {
	_, err := c.RunCommand(fmt.Sprintf("rm -f '%s'", remotePath))
	return err
}

func (c *Client) WPCLI(wpRoot, args string) (string, error) {
	return c.RunCommand(fmt.Sprintf("cd '%s' && %s %s", wpRoot, "wp", args))
}

func (c *Client) ParseDBConfig(wpRoot string) (string, string, string, string, error) {
	out, err := c.RunCommand(fmt.Sprintf(
		"grep -E \"^define\\([[:space:]]*'DB_(NAME|USER|PASSWORD|HOST)'\" '%s/wp-config.php'", wpRoot,
	))
	if err != nil {
		return c.parseDBConfigViaWPCLI(wpRoot)
	}

	dbName := parseDefineValue(out, "DB_NAME")
	dbUser := parseDefineValue(out, "DB_USER")
	dbPass := parseDefineValue(out, "DB_PASSWORD")
	dbHost := parseDefineValue(out, "DB_HOST")

	if dbName == "" || dbUser == "" || dbPass == "" || dbHost == "" {
		return c.parseDBConfigViaWPCLI(wpRoot)
	}

	return dbName, dbUser, dbPass, dbHost, nil
}

func (c *Client) parseDBConfigViaWPCLI(wpRoot string) (string, string, string, string, error) {
	out, err := c.RunCommand(fmt.Sprintf(
		"cd '%s' && wp config get --type=constant DB_NAME DB_USER DB_PASSWORD DB_HOST 2>/dev/null", wpRoot,
	))
	if err != nil {
		return "", "", "", "", fmt.Errorf("could not parse DB config from wp-config.php")
	}

	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) < 4 {
		return "", "", "", "", fmt.Errorf("unexpected WP-CLI output: %d lines", len(lines))
	}

	return lines[0], lines[1], lines[2], lines[3], nil
}

func parseDefineValue(content, key string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		if strings.Contains(line, fmt.Sprintf("'%s'", key)) {
			line = strings.TrimSpace(line)

			prefix := fmt.Sprintf("define('%s',", key)
			idx := strings.Index(line, prefix)
			if idx == -1 {
				prefix = fmt.Sprintf("define( '%s' ,", key)
				idx = strings.Index(line, prefix)
			}
			if idx == -1 {
				continue
			}

			valPart := line[idx+len(prefix):]
			valPart = strings.TrimSpace(valPart)

			if strings.HasPrefix(valPart, "'") {
				end := strings.Index(valPart[1:], "'")
				if end >= 0 {
					return valPart[1 : 1+end]
				}
			}
		}
	}
	return ""
}

func (c *Client) UploadFile(localPath, remotePath string) error {
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

func (c *Client) DownloadFile(remotePath, localPath string) error {
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

func (c *Client) SyncDir(sourceDir, destDir string, excludes []string, delete bool, onFile func(string)) error {
	sf, err := c.getSFTP()
	if err != nil {
		return err
	}

	sourceIsRemote := !isLocalPath(sourceDir)
	destIsRemote := !isLocalPath(destDir)

	if sourceIsRemote == destIsRemote {
		return fmt.Errorf("SyncDir requires one local and one remote path")
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
		wp := newDirWalker(sf)
		return wp.walkRemote(sourceDir, destDir, isExcluded, delete, onFile)
	}

	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
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
		remotePath := filepath.Join(destDir, relPath)

		if d.IsDir() {
			return sf.MkdirAll(remotePath)
		}

		localFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer localFile.Close()

		if err := sf.MkdirAll(filepath.Dir(remotePath)); err != nil {
			return err
		}
		remoteFile, err := sf.Create(remotePath)
		if err != nil {
			return err
		}
		defer remoteFile.Close()

		_, err = io.Copy(remoteFile, localFile)
		return err
	})
}

type dirWalker struct {
	sf *sftp.Client
}

func newDirWalker(sf *sftp.Client) *dirWalker {
	return &dirWalker{sf: sf}
}

func (w *dirWalker) walkRemote(sourceDir, destDir string, isExcluded func(string) bool, delete bool, onFile func(string)) error {
	remoteFiles := make(map[string]bool)

	return w.walkRemoteDir(sourceDir, destDir, sourceDir, remoteFiles, isExcluded, delete, onFile)
}

func (w *dirWalker) walkRemoteDir(baseSource, baseDest, currentDir string, remoteFiles map[string]bool, isExcluded func(string) bool, delete bool, onFile func(string)) error {
	entries, err := w.sf.ReadDir(currentDir)
	if err != nil {
		return fmt.Errorf("cannot read remote directory %s: %w", currentDir, err)
	}

	for _, entry := range entries {
		relPath, _ := filepath.Rel(baseSource, filepath.Join(currentDir, entry.Name()))
		if isExcluded(relPath) {
			continue
		}

		remoteFiles[relPath] = true

		localPath := filepath.Join(baseDest, relPath)

		if entry.IsDir() {
			if err := os.MkdirAll(localPath, 0755); err != nil {
				return err
			}
			if err := w.walkRemoteDir(baseSource, baseDest, filepath.Join(currentDir, entry.Name()), remoteFiles, isExcluded, delete, onFile); err != nil {
				return err
			}
			continue
		}

		if onFile != nil {
			onFile(relPath)
		}

		localFile, err := os.Create(localPath)
		if err != nil {
			return err
		}

		remoteFile, err := w.sf.Open(filepath.Join(currentDir, entry.Name()))
		if err != nil {
			localFile.Close()
			return err
		}

		_, err = io.Copy(localFile, remoteFile)
		remoteFile.Close()
		localFile.Close()
		if err != nil {
			return err
		}
	}

	return nil
}

func isLocalPath(p string) bool {
	_, err := os.Stat(filepath.Dir(p))
	return err == nil
}

func (c *Client) DetectWPRoot() (string, error) {
	homeDir := c.detectHomeDir()

	commonPaths := []string{
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
	}

	for _, dir := range []string{homeDir, "/var", "/srv", "/opt", "/usr", "/home"} {
		out, err := c.RunCommand(fmt.Sprintf("find %s -maxdepth 5 -type f -name wp-config.php 2>/dev/null || true", dir))
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(out), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line != "" {
				return filepath.Dir(line), nil
			}
		}
	}

	return "", fmt.Errorf("could not find WordPress installation on remote host")
}

func (c *Client) detectHomeDir() string {
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
