package yaml_test

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/go-kanna/config/yaml"
)

func TestDecode(t *testing.T) {
	t.Parallel()

	got, err := yaml.Decode([]byte(`
database:
  writer_url: postgres://writer@db/app
  pool_max_conns: 20
  ratio: 0.5
  debug: true
  started: 2026-11-07T09:30:00Z
  password:
api:
  port: 8080
`))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"database": map[string]any{
			"writer_url": "postgres://writer@db/app", "pool_max_conns": 20, "ratio": 0.5, "debug": true,
			"started": time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC), "password": nil,
		},
		"api": map[string]any{"port": 8080},
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

func TestDecode_Error(t *testing.T) {
	t.Parallel()

	_, err := yaml.Decode([]byte("- a list\n- not a mapping\n"))
	if err == nil || !strings.Contains(err.Error(), "decode YAML") {
		t.Errorf("Decode error = %v", err)
	}
}
