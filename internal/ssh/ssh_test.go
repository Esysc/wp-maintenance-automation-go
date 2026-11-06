package ssh

import (
	"path/filepath"
	"testing"
)

func TestParseDefineValue(t *testing.T) {
	cases := []struct {
		name string
		line string
		key  string
		want string
	}{
		{"compact", "define('DB_NAME', 'sandbox');", "DB_NAME", "sandbox"},
		{"double quotes", "define(\"DB_NAME\", \"sandbox\");", "DB_NAME", "sandbox"},
		{"wpcli", "define( 'DB_NAME', 'sandbox' );", "DB_NAME", "sandbox"},
		{"spaces around comma", "define( 'DB_USER' , 'user' );", "DB_USER", "user"},
		{"missing", "define('DB_NAME', 'x');", "DB_HOST", ""},
	}
	for _, c := range cases {
		got := parseDefineValue(c.line, c.key)
		if got != c.want {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestParseDefineValueMultiLineConfig(t *testing.T) {
	content := `<?php
define( 'DB_NAME', "wpdb" );
define("DB_USER", 'wpuser');
define( 'DB_PASSWORD' , 'secret-pass' );
define("DB_HOST", "127.0.0.1");
`

	if got := parseDefineValue(content, "DB_NAME"); got != "wpdb" {
		t.Fatalf("DB_NAME: got %q, want %q", got, "wpdb")
	}
	if got := parseDefineValue(content, "DB_USER"); got != "wpuser" {
		t.Fatalf("DB_USER: got %q, want %q", got, "wpuser")
	}
	if got := parseDefineValue(content, "DB_PASSWORD"); got != "secret-pass" {
		t.Fatalf("DB_PASSWORD: got %q, want %q", got, "secret-pass")
	}
	if got := parseDefineValue(content, "DB_HOST"); got != "127.0.0.1" {
		t.Fatalf("DB_HOST: got %q, want %q", got, "127.0.0.1")
	}
}

func TestNormalizeKeyMaterial(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"escaped newlines", "-----BEGIN KEY-----\\nabc\\n-----END KEY-----", "-----BEGIN KEY-----\nabc\n-----END KEY-----"},
		{"crlf", "line1\r\nline2\r\n", "line1\nline2"},
		{"trim spaces", "  key  ", "key"},
	}

	for _, c := range cases {
		got := normalizeKeyMaterial(c.in)
		if got != c.want {
			t.Fatalf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestLooksLikePublicKey(t *testing.T) {
	if !looksLikePublicKey("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host") {
		t.Fatalf("expected ssh-ed25519 to be recognized as public key")
	}
	if looksLikePublicKey("-----BEGIN OPENSSH PRIVATE KEY-----\n...") {
		t.Fatalf("private key block must not be detected as public key")
	}
}

func TestIsLocalPath(t *testing.T) {
	if isLocalPath("ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAI... user@host") {
		t.Fatalf("public key text must not be treated as a local path")
	}
	if isLocalPath("not-a-path") {
		t.Fatalf("bare token must not be treated as a local path")
	}
}

func TestNewExcludeMatcher(t *testing.T) {
	isExcluded := newExcludeMatcher([]string{
		".git",
		"*.log",
		"wp-content/cache",
		"./uploads",
		"vendor/",
	})

	cases := []struct {
		name string
		want bool
	}{
		{".git", true},
		{".git/config", true},
		{"debug.log", true},
		{"logs/debug.log", true},
		{"wp-content/cache/page.html", true},
		{"wp-content/themes/theme/style.css", false},
		{"uploads/2024/image.jpg", true},
		{"vendor/package/file.go", true},
		{"index.php", false},
	}

	for _, c := range cases {
		if got := isExcluded(c.name); got != c.want {
			t.Errorf("isExcluded(%q) = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestSecureArchivePath(t *testing.T) {
	root := t.TempDir()

	tests := []struct {
		name    string
		entry   string
		wantRel string
		wantErr bool
	}{
		{name: "root entry", entry: ".", wantRel: "."},
		{name: "nested file", entry: "wp/index.php", wantRel: "wp/index.php"},
		{name: "dot segments normalize", entry: "wp/./a/../b.txt", wantRel: "wp/b.txt"},
		{name: "parent traversal", entry: "../escape.txt", wantErr: true},
		{name: "nested parent traversal", entry: "wp/../../escape.txt", wantErr: true},
		{name: "absolute path", entry: "/etc/passwd", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := secureArchivePath(root, tt.entry)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("secureArchivePath(%q) = %q, want error", tt.entry, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("secureArchivePath(%q) error = %v", tt.entry, err)
			}
			rel, err := filepath.Rel(root, got)
			if err != nil {
				t.Fatalf("filepath.Rel: %v", err)
			}
			if filepath.ToSlash(rel) != tt.wantRel {
				t.Fatalf("secureArchivePath(%q) rel = %q, want %q", tt.entry, rel, tt.wantRel)
			}
		})
	}
}

func TestParsePHPVersion(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare version", "8.3.27\n", "8.3.27"},
		{"php -v output", "PHP 8.2.35 (cli) (built: Jan 1 2026)\nCopyright (c) The PHP Group", "8.2.35"},
		{"surrounding whitespace", "  8.4.1  ", "8.4.1"},
		{"empty", "", ""},
		{"unrelated output", "WordPress version 6.6.2", ""},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := parsePHPVersion(c.in); got != c.want {
				t.Fatalf("parsePHPVersion(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
