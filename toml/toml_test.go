package toml_test

import (
	"reflect"
	"testing"

	"github.com/go-kanna/config/toml"
)

// A value is read as text, and a date or a time without a time zone gets none.
func TestDecode(t *testing.T) {
	t.Parallel()

	got, err := toml.Decode([]byte(`
[database]
writer_url = "postgres://writer@db/app"
pool_max_conns = 20
hex = 0x1F
thousand = 1_000
ratio = 1.20
whole = 1.0
large = 1e300
inf = -inf
debug = true
started = 2026-11-07 09:30:00Z
offset = 2026-11-07T09:30:00.5-08:00
day = 1979-05-27
at = 07:32:00.25
local = 1979-05-27T07:32:00
hosts = ["a", "b"]

[api]
port = 8080

[[servers]]
name = "a"
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"database": map[string]any{
			"writer_url": "postgres://writer@db/app", "pool_max_conns": "20", "hex": "31", "thousand": "1000", "ratio": "1.2",
			"whole": "1.0", "large": "1e+300", "inf": "-Inf", "debug": "true",
			"started": "2026-11-07T09:30:00Z", "offset": "2026-11-07T09:30:00.5-08:00",
			"day": "1979-05-27", "at": "07:32:00.25", "local": "1979-05-27T07:32:00", "hosts": []any{"a", "b"},
		},
		"api":     map[string]any{"port": "8080"},
		"servers": []any{map[string]any{"name": "a"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode = %#v, want %#v", got, want)
	}
}

func TestDecode_Empty(t *testing.T) {
	t.Parallel()

	if got, err := toml.Decode(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Decode(empty) = %v, %v, want no tables", got, err)
	}
}

// An error says where the document is wrong, never what it holds there, which
// may be a secret.
func TestDecode_Error(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, data, want string
	}{
		{"no value", "port = \n", "line 1, column 8 is not valid TOML"},
		{"too big", "[db]\npin = 99999999999999999999\n", "line 2, column 7 is not valid TOML"},
		{"bad escape", "[db]\npassword = \"s3\\qcr3t\"\n", "line 2, column 13 is not valid TOML"},
		{"bare", "[db]\npassword = s3cr3t\n", "line 2, column 12 is not valid TOML"},
	} {
		_, err := toml.Decode([]byte(tt.data))
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: Decode error = %v, want %q", tt.name, err, tt.want)
		}
	}
}
