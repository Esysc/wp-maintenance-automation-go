package apiserver

import (
	"compress/gzip"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMySQLOptionFileContentQuotesValues(t *testing.T) {
	got := mysqlOptionFileContent(`us"er`, "pa\nss", `db\host`)

	if !strings.Contains(got, `user="us\"er"`) {
		t.Fatalf("user was not escaped: %q", got)
	}
	if !strings.Contains(got, `password="pa\nss"`) {
		t.Fatalf("password was not escaped: %q", got)
	}
	if !strings.Contains(got, `host="db\\host"`) {
		t.Fatalf("host was not escaped: %q", got)
	}
}

func TestFindFirstFile(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "db"), 0755); err != nil {
		t.Fatalf("failed to create db dir: %v", err)
	}
	plainDump := filepath.Join(root, "db", "site.sql")
	gzDump := filepath.Join(root, "db", "site.sql.gz")
	if err := os.WriteFile(plainDump, []byte("dump"), 0644); err != nil {
		t.Fatalf("failed to write plain dump: %v", err)
	}
	if err := os.WriteFile(gzDump, []byte("dump"), 0644); err != nil {
		t.Fatalf("failed to write gz dump: %v", err)
	}

	if got := findFirstFile(root, func(path string, _ fs.DirEntry) bool {
		return strings.HasSuffix(path, ".sql.gz")
	}); got != gzDump {
		t.Fatalf("findFirstFile(.sql.gz) = %q, want %q", got, gzDump)
	}

	if got := findFirstFile(root, func(path string, _ fs.DirEntry) bool {
		return strings.HasSuffix(path, ".zip")
	}); got != "" {
		t.Fatalf("findFirstFile(.zip) = %q, want empty", got)
	}
}

func TestFindFirstDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "www", "wordpress")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatalf("failed to create nested dir: %v", err)
	}

	if got := findFirstDir(root, func(name string) bool {
		return name == "wordpress" || name == "html" || name == "wp"
	}); got != nested {
		t.Fatalf("findFirstDir(wordpress) = %q, want %q", got, nested)
	}

	if got := findFirstDir(root, func(name string) bool { return true }); got == root {
		t.Fatalf("findFirstDir should not return the walk root")
	}
}

func TestDecompressGzipToTempFile(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "dump.sql.gz")

	f, err := os.Create(src)
	if err != nil {
		t.Fatalf("failed to create source: %v", err)
	}
	gz := gzip.NewWriter(f)
	if _, err := gz.Write([]byte("CREATE TABLE t (id INT);")); err != nil {
		t.Fatalf("failed to write gzip data: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("failed to close source: %v", err)
	}

	out, err := decompressGzipToTempFile(src, dir)
	if err != nil {
		t.Fatalf("decompressGzipToTempFile() error = %v", err)
	}
	defer os.Remove(out)

	if filepath.Dir(out) != dir {
		t.Fatalf("decompressed file %q is not inside %q", out, dir)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read decompressed file: %v", err)
	}
	if string(data) != "CREATE TABLE t (id INT);" {
		t.Fatalf("decompressed content = %q", string(data))
	}
}
