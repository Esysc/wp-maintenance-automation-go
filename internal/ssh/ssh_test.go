package ssh

import "testing"

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
