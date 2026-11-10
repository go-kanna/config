package yaml_test

import (
	"reflect"
	"testing"

	"github.com/go-kanna/config/yaml"
)

// A value is read as the text it is written as, not as the library types it.
func TestDecode(t *testing.T) {
	t.Parallel()

	got, err := yaml.Decode([]byte(`
database:
  writer_url: postgres://writer@db/app
  pool_max_conns: 20
  ratio: 1.20
  whole: 1.0
  hex: 0x1F
  debug: True
  started: 2026-11-07T09:30:00Z
  day: 2026-01-01
  phone: 09012345678
  zip: "0123"
  octal: 0123
  big: 12345678901234567890123
  password:
  tilde: ~
  hosts: [a, b]
base: &base
  port: 8080
other:
  host: &host example.com
api:
  <<: *base
  host: *host
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"database": map[string]any{
			"writer_url": "postgres://writer@db/app", "pool_max_conns": "20", "ratio": "1.20", "whole": "1.0",
			"hex": "0x1F", "debug": "True", "started": "2026-11-07T09:30:00Z", "day": "2026-01-01",
			"phone": "09012345678", "zip": "0123", "octal": "0123", "big": "12345678901234567890123",
			"password": nil, "tilde": nil, "hosts": []any{"a", "b"},
		},
		"base":  map[string]any{"port": "8080"},
		"api":   map[string]any{"port": "8080", "host": "example.com"},
		"other": map[string]any{"host": "example.com"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Decode = %#v, want %#v", got, want)
	}
}

func TestDecode_Empty(t *testing.T) {
	t.Parallel()

	if got, err := yaml.Decode(nil); err != nil || got == nil || len(got) != 0 {
		t.Errorf("Decode(empty) = %v, %v, want no sections", got, err)
	}
}

// A file may mark the start and the end of its one document.
func TestDecode_Markers(t *testing.T) {
	t.Parallel()

	got, err := yaml.Decode([]byte("---\napi:\n  port: 8080\n...\n"))
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]any{"api": map[string]any{"port": "8080"}}; !reflect.DeepEqual(got, want) {
		t.Errorf("Decode = %#v, want %#v", got, want)
	}
}

// An error says where the document is wrong, never what it holds there, which
// may be a secret.
func TestDecode_Error(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, data, want string
	}{
		{"a list", "- a list\n- not a mapping\n", "line 1, column 1 is not a YAML mapping"},
		{"a scalar", "s3cr3t\n", "line 1, column 1 is not a YAML mapping"},
		{"a wrong tag", "db:\n  password: !!int s3cr3t\n", "line 2, column 13 is not valid YAML"},
		{"a colon", "db:\n  password: s3cr3t: x\n", "line 2 is not valid YAML"},
		{"an open quote", "db:\n  password: \"s3cr3t\n", "line 2 is not valid YAML"},
		{"a key twice", "db:\n  password: s3cr3t\n  password: s3cr3t\n", "line 3 is not valid YAML"},
		{
			"a second document", "db:\n  password: s3cr3t\n---\ndb:\n  password: s3cr3t\n",
			"line 3, column 1 starts a second YAML document, and a file holds one",
		},
		{
			"a second document that is wrong", "db:\n  password: s3cr3t\n---\ndb:\n  password: \"s3cr3t\n",
			"line 5 is not valid YAML",
		},
	} {
		_, err := yaml.Decode([]byte(tt.data))
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: Decode error = %v, want %q", tt.name, err, tt.want)
		}
	}
}
