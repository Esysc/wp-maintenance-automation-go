package apiserver

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/andreacristalli/wp-maintenance-automation-go/internal/ssh"
)

// ensure compile-time check that the fake satisfies ssh.Client.
var _ ssh.Client = (*fakeSSHClient)(nil)

// fakeSSHClient is a test double for ssh.Client that emulates a remote
// WordPress host using local directories:
//
//	serverRoot/www/fakeprod   <- the WordPress filesystem (site root)
//	serverRoot/mysql/fakeprod.sql <- the "MySQL" database (a single SQL dump file)
//	serverRoot/tmp/           <- the remote /tmp used for dumps and credentials
//
// Remote paths are resolved via resolveRemote(): the site root and /tmp are
// mapped into serverRoot, so /tmp files never pollute the site tree.
type fakeSSHClient struct {
	serverRoot string
	siteRoot   string // remote path string, e.g. /srv/www/fakeprod
	siteDir    string // local dir backing siteRoot
	dbName     string
	dbFile     string
	dbUser     string
	dbPassword string
	dbHost     string
}

func newFakeSSHClient(t *testing.T) *fakeSSHClient {
	t.Helper()
	serverRoot := t.TempDir()
	return &fakeSSHClient{
		serverRoot: serverRoot,
		siteRoot:   "/srv/www/fakeprod",
		siteDir:    filepath.Join(serverRoot, "www", "fakeprod"),
		dbName:     "fakeprod",
		dbFile:     filepath.Join(serverRoot, "mysql", "fakeprod.sql"),
		dbUser:     "fakeprod_user",
		dbPassword: "s3cret!pass",
		dbHost:     "localhost",
	}
}

// resolveRemote maps a remote path (site root or /tmp) to a local path under
// serverRoot.
func (f *fakeSSHClient) resolveRemote(p string) string {
	p = filepath.ToSlash(strings.TrimRight(p, "/"))
	switch {
	case p == f.siteRoot || strings.HasPrefix(p, f.siteRoot+"/"):
		rel := strings.TrimPrefix(p, f.siteRoot)
		return filepath.Join(f.serverRoot, "www", "fakeprod", rel)
	case strings.HasPrefix(p, "/tmp/"):
		return filepath.Join(f.serverRoot, "tmp", strings.TrimPrefix(p, "/tmp/"))
	default:
		return filepath.Join(f.serverRoot, strings.TrimLeft(p, "/"))
	}
}

func (f *fakeSSHClient) Close() error                           { return nil }
func (f *fakeSSHClient) DSN() string                            { return "fake@fakeprod" }
func (f *fakeSSHClient) CommandExists(cmd string) (bool, error) { return cmd == "tar", nil }

func (f *fakeSSHClient) FileExists(remotePath string) (bool, error) {
	info, err := os.Stat(f.resolveRemote(remotePath))
	if err != nil {
		return false, nil
	}
	return !info.IsDir(), nil
}

var (
	reCredHeredoc = regexp.MustCompile(`^umask 077 && cat > '([^']+)' << 'EOF'\n`)
	reMysqldump   = regexp.MustCompile(`^mysqldump\s+.* '([^']+)' > '([^']+)'`)
	reMysqlImport = regexp.MustCompile(`^mysql\s+-u\S+\s+-p'[^']*'\s+-h\S+\s+(\S+)\s+<\s*'?([^'\s]+)'?\s*&&\s*rm\s+-f\s+'?([^'\s]+)'?$`)
	reTestS       = regexp.MustCompile(`^test -s '([^']+)'$`)
	reRm          = regexp.MustCompile(`^rm -f '([^']+)'$`)
)

func (f *fakeSSHClient) RunCommand(cmd string) (string, error) {
	switch {
	case reCredHeredoc.MatchString(cmd):
		// Write the mysql credentials file to remote /tmp.
		match := reCredHeredoc.FindStringSubmatch(cmd)
		target := f.resolveRemote(match[1])
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		marker := "<<" + " 'EOF'\n"
		start := strings.Index(cmd, marker) + len(marker)
		end := strings.LastIndex(cmd, "\nEOF")
		if start < 0 || end < 0 || end < start {
			return "", fmt.Errorf("fake ssh: malformed credentials heredoc")
		}
		content := cmd[start:end]
		if err := os.WriteFile(target, []byte(content), 0600); err != nil {
			return "", err
		}
		return "", nil

	case reMysqldump.MatchString(cmd):
		match := reMysqldump.FindStringSubmatch(cmd)
		dbName, outFile := match[1], match[2]
		if dbName != f.dbName {
			return "", fmt.Errorf("fake ssh: mysqldump for unknown database %q", dbName)
		}
		data, err := os.ReadFile(f.dbFile)
		if err != nil {
			return "", fmt.Errorf("fake ssh: cannot dump database %q: %w", dbName, err)
		}
		target := f.resolveRemote(outFile)
		if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
			return "", err
		}
		if err := os.WriteFile(target, data, 0600); err != nil {
			return "", err
		}
		return "", nil

	case reMysqlImport.MatchString(cmd):
		match := reMysqlImport.FindStringSubmatch(cmd)
		dbName, srcFile, rmFile := match[1], match[2], match[3]
		if dbName != f.dbName {
			return "", fmt.Errorf("fake ssh: mysql import into unknown database %q", dbName)
		}
		data, err := os.ReadFile(f.resolveRemote(srcFile))
		if err != nil {
			return "", fmt.Errorf("fake ssh: cannot read dump for import: %w", err)
		}
		if err := os.MkdirAll(filepath.Dir(f.dbFile), 0755); err != nil {
			return "", err
		}
		if err := os.WriteFile(f.dbFile, data, 0644); err != nil {
			return "", err
		}
		if rmFile != "" {
			os.Remove(f.resolveRemote(rmFile))
		}
		return "", nil

	case reTestS.MatchString(cmd):
		match := reTestS.FindStringSubmatch(cmd)
		info, err := os.Stat(f.resolveRemote(match[1]))
		if err != nil || info.Size() == 0 {
			return "", fmt.Errorf("fake ssh: file missing or empty")
		}
		return "", nil

	case reRm.MatchString(cmd):
		match := reRm.FindStringSubmatch(cmd)
		os.Remove(f.resolveRemote(match[1]))
		return "", nil

	default:
		return "", fmt.Errorf("fake ssh: unsupported command: %s", cmd)
	}
}

func (f *fakeSSHClient) RunCommandRaw(cmd string) string {
	out, err := f.RunCommand(cmd)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return out
}

func (f *fakeSSHClient) DetectWPRoot() (string, error) {
	if _, err := os.Stat(filepath.Join(f.siteDir, "wp-config.php")); err == nil {
		return f.siteRoot, nil
	}
	return "", fmt.Errorf("fake ssh: no WordPress installation found")
}

func (f *fakeSSHClient) GetWpVersion(wpRoot string) (string, error) {
	content, err := os.ReadFile(filepath.Join(f.resolveRemote(wpRoot), "wp-includes", "version.php"))
	if err != nil {
		return "", err
	}
	re := regexp.MustCompile(`\$wp_version\s*=\s*'([^']+)'`)
	if m := re.FindStringSubmatch(string(content)); len(m) == 2 {
		return m[1], nil
	}
	return "", fmt.Errorf("fake ssh: version not found")
}

func (f *fakeSSHClient) ParseDBConfig(wpRoot string) (string, string, string, string, error) {
	content, err := os.ReadFile(filepath.Join(f.resolveRemote(wpRoot), "wp-config.php"))
	if err != nil {
		return "", "", "", "", err
	}
	values := make(map[string]string)
	re := regexp.MustCompile(`define\(\s*'DB_(NAME|USER|PASSWORD|HOST)'\s*,\s*'([^']*)'`)
	for _, m := range re.FindAllStringSubmatch(string(content), -1) {
		values[m[1]] = m[2]
	}
	if values["NAME"] == "" || values["USER"] == "" || values["PASSWORD"] == "" || values["HOST"] == "" {
		return "", "", "", "", fmt.Errorf("fake ssh: incomplete DB config")
	}
	return values["NAME"], values["USER"], values["PASSWORD"], values["HOST"], nil
}

func (f *fakeSSHClient) DownloadFile(remotePath, localPath string) error {
	src := f.resolveRemote(remotePath)
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	return copyFile(src, localPath)
}

func (f *fakeSSHClient) UploadFile(localPath, remotePath string) error {
	target := f.resolveRemote(remotePath)
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	return copyFile(localPath, target)
}

func (f *fakeSSHClient) RemoveRemoteFile(remotePath string) error {
	return os.Remove(f.resolveRemote(remotePath))
}

func (f *fakeSSHClient) CountRemoteFiles(dir string) (int, error) {
	count := 0
	err := filepath.WalkDir(f.resolveRemote(dir), func(path string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			count++
		}
		return nil
	})
	return count, err
}

// SyncDir copies between a local dir and the fake remote site dir. It emulates
// rsync semantics: when delete is true, remote files missing from the source
// are removed. Excludes match the real SSH client (basename glob match).
func (f *fakeSSHClient) SyncDir(sourceDir, destDir string, sourceIsRemote bool, excludes []string, delete bool, onFile func(string)) error {
	if sourceIsRemote {
		if !strings.HasPrefix(filepath.ToSlash(sourceDir), f.siteRoot+"/") && filepath.ToSlash(sourceDir) != f.siteRoot {
			return fmt.Errorf("fake ssh: sourceDir is not a remote site path: %s", sourceDir)
		}
		return f.downloadTree(f.resolveRemote(sourceDir), destDir, func(name string) bool {
			base := filepath.Base(name)
			for _, e := range excludes {
				if e != "" {
					if matched, _ := filepath.Match(e, base); matched {
						return true
					}
				}
			}
			return false
		}, onFile)
	}

	destIsRemote := strings.HasPrefix(filepath.ToSlash(destDir), f.siteRoot+"/") || filepath.ToSlash(destDir) == f.siteRoot
	if !destIsRemote {
		return fmt.Errorf("fake ssh: destDir is not a remote site path: %s", destDir)
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
	return f.uploadTree(sourceDir, f.resolveRemote(destDir), isExcluded, delete, onFile)
}

func (f *fakeSSHClient) downloadTree(srcDir, destDir string, isExcluded func(string) bool, onFile func(string)) error {
	return copyTree(srcDir, destDir, isExcluded, false, onFile)
}

func (f *fakeSSHClient) uploadTree(srcDir, destDir string, isExcluded func(string) bool, delete bool, onFile func(string)) error {
	return copyTree(srcDir, destDir, isExcluded, delete, onFile)
}

// copyTree copies src into dst, honoring excludes. When deleteMissing is true
// files present in dst but missing from src are removed (rsync --delete).
func copyTree(srcDir, destDir string, isExcluded func(string) bool, deleteMissing bool, onFile func(string)) error {
	srcDir = filepath.Clean(srcDir)
	destDir = filepath.Clean(destDir)

	sent := make(map[string]struct{})
	err := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relPath, _ := filepath.Rel(srcDir, path)
		if relPath == "." {
			return nil
		}
		relPath = filepath.ToSlash(relPath)
		if isExcluded(relPath) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}

		target := filepath.Join(destDir, relPath)
		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		sent[relPath] = struct{}{}
		if onFile != nil {
			onFile(relPath)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
	if err != nil {
		return err
	}

	if deleteMissing {
		filepath.WalkDir(destDir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			relPath, _ := filepath.Rel(destDir, path)
			if _, ok := sent[filepath.ToSlash(relPath)]; !ok {
				os.Remove(path)
			}
			return nil
		})
	}
	return nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// seedFakeProdSite creates a fake WordPress installation with fake content and
// a fake MySQL database containing a couple of posts.
func (f *fakeSSHClient) seedFakeProdSite() error {
	wp := f.siteDir
	for _, dir := range []string{
		filepath.Join(wp, "wp-includes"),
		filepath.Join(wp, "wp-admin"),
		filepath.Join(wp, "wp-content", "themes", "faketheme"),
		filepath.Join(wp, "wp-content", "uploads", "2026", "08"),
		filepath.Join(wp, "wp-content", "plugins", "fake-plugin"),
	} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	files := map[string]string{
		"index.php":     "<?php /* FAKE PROD INDEX */ echo 'Welcome to the fake prod site!'; ?>",
		"wp-config.php": fmt.Sprintf("<?php\ndefine('DB_NAME', '%s');\ndefine('DB_USER', '%s');\ndefine('DB_PASSWORD', '%s');\ndefine('DB_HOST', '%s');\n", f.dbName, f.dbUser, f.dbPassword, f.dbHost),
		filepath.Join("wp-includes", "version.php"):                              "<?php $wp_version = '6.7.2'; ?>",
		filepath.Join("wp-content", "themes", "faketheme", "index.php"):          "<?php /* theme */ ?>",
		filepath.Join("wp-content", "plugins", "fake-plugin", "fake-plugin.php"): "<?php /* plugin */ ?>",
		filepath.Join("wp-content", "uploads", "2026", "08", "photo.jpg"):        "\xff\xd8\xff\xe0 fake-jpeg-content",
		filepath.Join("wp-content", "uploads", "2026", "08", "article.txt"):      "Fake blog post content: the quick brown fox jumps over the lazy dog.",
		filepath.Join("wp-content", "uploads", "2026", "08", "index.html"):       "<html><body>fake upload index</body></html>",
	}
	for rel, content := range files {
		path := filepath.Join(wp, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(content), 0644); err != nil {
			return err
		}
	}

	db := fmt.Sprintf(`-- Fake prod database dump
-- MySQL dump for %s
CREATE TABLE wp_posts (ID bigint, post_title text);
INSERT INTO wp_posts VALUES (1, 'Hello fake world');
INSERT INTO wp_posts VALUES (2, 'Backup and restore drill');
INSERT INTO wp_posts VALUES (3, 'Live from the fake prod site');
`, f.dbName)
	if err := os.MkdirAll(filepath.Dir(f.dbFile), 0755); err != nil {
		return err
	}
	return os.WriteFile(f.dbFile, []byte(db), 0644)
}
