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
