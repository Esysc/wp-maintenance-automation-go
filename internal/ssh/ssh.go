package ssh

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

const (
	DefaultBatchMode             = "yes"
	DefaultStrictHostKeyChecking = "accept-new"
	DefaultConnectTimeout        = "15"
	DefaultRemoteCommandPath     = "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin"
)

type SSHOptions struct {
	Host                  string
	Port                  int
	User                  string
	BatchMode             string
	StrictHostKeyChecking string
	ConnectTimeout        string
	IdentityFile          string
	ExtraArgs             []string
}

func NewSSHOptions(host, user string, port int) *SSHOptions {
	return &SSHOptions{
		Host:                  host,
		Port:                  port,
		User:                  user,
		BatchMode:             DefaultBatchMode,
		StrictHostKeyChecking: DefaultStrictHostKeyChecking,
		ConnectTimeout:        DefaultConnectTimeout,
	}
}

func (s *SSHOptions) DSN() string {
	return fmt.Sprintf("%s@%s", s.User, s.Host)
}

func (s *SSHOptions) BuildArgs() []string {
	args := []string{
		"-o", fmt.Sprintf("BatchMode=%s", s.BatchMode),
		"-o", fmt.Sprintf("StrictHostKeyChecking=%s", s.StrictHostKeyChecking),
		"-o", fmt.Sprintf("ConnectTimeout=%s", s.ConnectTimeout),
	}
	if s.Port > 0 && s.Port != 22 {
		args = append(args, "-o", fmt.Sprintf("Port=%d", s.Port))
	}
	if s.IdentityFile != "" {
		args = append(args, "-i", s.IdentityFile)
	}
	args = append(args, s.ExtraArgs...)
	return args
}

type Client struct {
	opts *SSHOptions
}

func NewClient(opts *SSHOptions) *Client {
	return &Client{opts: opts}
}

func (c *Client) DSN() string {
	return c.opts.DSN()
}

func (c *Client) BuildArgs() []string {
	return c.opts.BuildArgs()
}

func (c *Client) RunCommand(cmd string) (string, error) {
	args := append(c.BuildArgs(), c.DSN(), cmd)
	sshCmd := exec.Command("ssh", args...)
	sshCmd.Env = append(os.Environ(),
		fmt.Sprintf("PATH=%s", DefaultRemoteCommandPath),
	)
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SSH command failed: %w\nOutput: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) RunCommandWithEnv(cmd string, env []string) (string, error) {
	args := append(c.BuildArgs(), c.DSN(), cmd)
	sshCmd := exec.Command("ssh", args...)
	sshCmd.Env = append(os.Environ(), env...)
	output, err := sshCmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("SSH command with env failed: %w\nOutput: %s", err, string(output))
	}
	return strings.TrimSpace(string(output)), nil
}

func (c *Client) UploadFile(localPath, remotePath string) error {
	opts := c.BuildArgs()
	args := append(opts, localPath, fmt.Sprintf("%s:%s", c.DSN(), remotePath))
	cmd := exec.Command("rsync", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync upload failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (c *Client) DownloadFile(remotePath, localPath string) error {
	opts := c.BuildArgs()
	args := append(opts, fmt.Sprintf("%s:%s", c.DSN(), remotePath), localPath)
	cmd := exec.Command("rsync", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync download failed: %w\nOutput: %s", err, string(output))
	}
	return nil
}

func (c *Client) SyncDir(sourceDir, destDir string, excludes []string, delete bool) error {
	args := []string{"-a", "--no-owner", "--no-group", "--timeout=60"}
	for _, exclude := range excludes {
		if exclude != "" {
			args = append(args, "--exclude", exclude)
		}
	}
	if delete {
		args = append(args, "--delete")
	}
	sshOpts := strings.Join(c.BuildArgs(), " ")
	args = append(args, "-e", fmt.Sprintf("ssh %s", sshOpts))
	args = append(args, sourceDir, destDir)
	cmd := exec.Command("rsync", args...)
	cmd.Env = os.Environ()
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("rsync sync failed: %w\nOutput: %s", err, string(output))
	}
	return nil
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
