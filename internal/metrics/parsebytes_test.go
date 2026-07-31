package metrics

import "testing"

func miB(v float64) uint64 { return uint64(v * float64(1<<20)) }
func giB(v float64) uint64 { return uint64(v * float64(1<<30)) }
func kBy(v float64) uint64 { return uint64(v * float64(1<<10)) }

func TestParseBytes(t *testing.T) {
	cases := []struct {
		in   string
		want uint64
	}{
		{"2.969MiB", miB(2.969)},
		{"5.781GiB", giB(5.781)},
		{"41.1MiB", miB(41.1)},
		{"12.47MiB", miB(12.47)},
		{"2.969 MiB", miB(2.969)},
		{"123B", 123},
		{"1.5 KB", kBy(1.5)},
		{"", 0},
		{"MiB", 0},
	}
	for _, c := range cases {
		if got := parseBytes(c.in); got != c.want {
			t.Errorf("parseBytes(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseMemUsage(t *testing.T) {
	used, limit := parseMemUsage("2.969MiB / 5.781GiB")
	if used != miB(2.969) || limit != giB(5.781) {
		t.Errorf("parseMemUsage = %d/%d", used, limit)
	}
}
