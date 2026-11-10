package source_test

import (
	"errors"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-kanna/validator"

	"github.com/go-kanna/config/source"
)

var key = source.Key{Env: "DATABASE_WRITER_URL", File: []string{"database", "writer_url"}, Flag: "database-writer-url"}

func writeFile(t *testing.T, name, content string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// The tests below set variables of the process, which t.Setenv rules out for
// parallel tests.

func TestLookup_Env(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "postgres://writer@db/app")

	v, ok, err := source.Env().Lookup(key)
	if err != nil || !ok || v != (source.Value{Text: "postgres://writer@db/app", Name: "DATABASE_WRITER_URL"}) {
		t.Errorf("Lookup = %+v, %v, %v", v, ok, err)
	}
}

func TestLookup_EmptyIsUnset(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "")

	if v, ok, err := source.Env().Lookup(key); err != nil || ok {
		t.Errorf("Lookup = %+v, %v, %v, want nothing", v, ok, err)
	}
}

func TestLookup_File(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "")
	t.Setenv("DATABASE_WRITER_URL_FILE", writeFile(t, "secret", "postgres://secret@db/app\r\n"))

	v, ok, err := source.Env().Lookup(key)
	if err != nil || !ok || v != (source.Value{Text: "postgres://secret@db/app", Name: "DATABASE_WRITER_URL_FILE"}) {
		t.Errorf("Lookup = %+v, %v, %v", v, ok, err)
	}
}

func TestLookup_FileEmpty(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL_FILE", writeFile(t, "secret", "\n"))

	if v, ok, err := source.Env().Lookup(key); err != nil || ok {
		t.Errorf("Lookup = %+v, %v, %v, want nothing", v, ok, err)
	}
}

func TestLookup_FileAndValue(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "postgres://writer@db/app")
	t.Setenv("DATABASE_WRITER_URL_FILE", writeFile(t, "secret", "x"))

	v, _, err := source.Env().Lookup(key)
	if err == nil || v.Name != "DATABASE_WRITER_URL" || !strings.Contains(err.Error(), "both") {
		t.Errorf("Lookup = %+v, %v, want an error on DATABASE_WRITER_URL", v, err)
	}
}

func TestLookup_FileMissing(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL_FILE", filepath.Join(t.TempDir(), "none"))

	v, _, err := source.Env().Lookup(key)
	if err == nil || v.Name != "DATABASE_WRITER_URL_FILE" {
		t.Errorf("Lookup = %+v, %v, want an error on DATABASE_WRITER_URL_FILE", v, err)
	}
}

func TestLookup_FlagsWinOverTheEnvironment(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "postgres://env@db/app")
	t.Setenv("DATABASE_POOL_MAX_CONNS", "20")

	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	source.DefineFlag(fs, "database-writer-url", "Database.WriterURL", "", false)
	source.DefineFlag(fs, "database-pool-max-conns", "Database.PoolMaxConns", "10", false)
	if err := fs.Parse([]string{"-database-writer-url", "postgres://flag@db/app"}); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Flags: fs})
	if err != nil {
		t.Fatal(err)
	}

	if v, ok, err := src.Lookup(key); err != nil || !ok ||
		v != (source.Value{Text: "postgres://flag@db/app", Name: "-database-writer-url"}) {
		t.Errorf("Lookup(writer) = %+v, %v, %v", v, ok, err)
	}
	pool := source.Key{
		Env: "DATABASE_POOL_MAX_CONNS", File: []string{"database", "pool_max_conns"}, Flag: "database-pool-max-conns",
	}
	v, ok, err := src.Lookup(pool)
	if err != nil || !ok || v != (source.Value{Text: "20", Name: "DATABASE_POOL_MAX_CONNS"}) {
		t.Errorf("Lookup(pool) = %+v, %v, %v: a flag not set gives way to the environment", v, ok, err)
	}
}

func TestLookup_TheEnvironmentWinsOverFiles(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "postgres://env@db/app")

	src, err := source.Open(source.Sources{Files: []source.File{
		{Path: writeFile(t, "a.json", `{"database": {"writer_url": "postgres://file@db/app"}}`), Decode: source.JSON},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, err := src.Lookup(key); err != nil || !ok || v.Name != "DATABASE_WRITER_URL" {
		t.Errorf("Lookup = %+v, %v, %v", v, ok, err)
	}
}

func TestLookup_Files(t *testing.T) {
	t.Setenv("DATABASE_WRITER_URL", "")

	base := writeFile(t, "base.json", `{
		"database": {"writer_url": "postgres://base@db/app", "pool_max_conns": 10, "debug": true, "ratio": 0.5,
			"replica": {"url": "x"}, "hosts": ["a", "b"], "none": null},
		"api": "flat",
		"cache": null
	}`)
	prod := writeFile(t, "prod.json", `{"database": {"pool_max_conns": 30}}`)
	src, err := source.Open(source.Sources{Files: []source.File{
		{Path: base, Decode: source.JSON},
		{Path: prod, Decode: source.JSON},
	}})
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		path []string
		want source.Value
		ok   bool
		err  string
	}{
		{
			path: []string{"database", "writer_url"},
			want: source.Value{Text: "postgres://base@db/app", Name: "database.writer_url"},
			ok:   true,
		},
		{
			path: []string{"database", "pool_max_conns"},
			want: source.Value{Text: "30", Name: "database.pool_max_conns"},
			ok:   true,
		},
		{path: []string{"database", "debug"}, want: source.Value{Text: "true", Name: "database.debug"}, ok: true},
		{path: []string{"database", "ratio"}, want: source.Value{Text: "0.5", Name: "database.ratio"}, ok: true},
		{path: []string{"database", "missing"}},
		{path: []string{"database", "none"}},
		{path: []string{"other", "key"}},
		{path: []string{"cache", "url"}},
		{path: nil},
		{
			path: []string{"database", "replica"},
			want: source.Value{Name: "database.replica"},
			err:  "is a section, where a value goes",
		},
		{
			path: []string{"database", "hosts"},
			want: source.Value{Name: "database.hosts"},
			err:  "is a list, where a value goes",
		},
		{
			path: []string{"api", "port"},
			want: source.Value{Name: "api.port"},
			err:  "api of " + base + " is a value, with nothing under it",
		},
	}
	for _, tt := range tests {
		v, ok, err := src.Lookup(source.Key{File: tt.path})
		failed := err != nil && !strings.Contains(err.Error(), tt.err)
		if v != tt.want || ok != tt.ok || (err == nil) != (tt.err == "") || failed {
			t.Errorf("Lookup(%q) = %+v, %v, %v, want %+v, %v, %q", tt.path, v, ok, err, tt.want, tt.ok, tt.err)
		}
	}
}

func TestOpen_Errors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		file source.File
		want string
	}{
		{
			name: "missing",
			file: source.File{Path: filepath.Join(t.TempDir(), "none.json"), Decode: source.JSON},
			want: "read settings",
		},
		{
			name: "not JSON",
			file: source.File{Path: writeFile(t, "bad.json", `{"a":`), Decode: source.JSON},
			want: "line 1, column 6 is not valid JSON",
		},
		{name: "no decoder", file: source.File{Path: writeFile(t, "a.json", `{}`)}, want: "no Decode"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, err := source.Open(source.Sources{Files: []source.File{tt.file}})
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("Open error = %v, want one containing %q", err, tt.want)
			}
		})
	}
}

func TestJSON(t *testing.T) {
	t.Parallel()

	tree, err := source.JSON([]byte(`{"big": 12345678901234567890}`))
	if err != nil {
		t.Fatal(err)
	}
	src := writeFile(t, "big.json", `{"a": {"big": 12345678901234567890}}`)
	opened, err := source.Open(source.Sources{Files: []source.File{{Path: src, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}
	v, ok, err := opened.Lookup(source.Key{File: []string{"a", "big"}})
	if err != nil || !ok || v.Text != "12345678901234567890" {
		t.Errorf("Lookup = %+v, %v, %v, want the number as written", v, ok, err)
	}
	if _, ok := tree["big"]; !ok {
		t.Errorf("JSON = %v", tree)
	}

	if tree, err := source.JSON([]byte("null")); err != nil || tree == nil || len(tree) != 0 {
		t.Errorf("JSON(null) = %v, %v, want no sections", tree, err)
	}
}

// An error says where the data is wrong, never what it holds there, which may
// be a secret.
func TestJSON_Error(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name, data, want string
	}{
		{"a bare word", `{"db": {"password": s3cr3t}}`, "line 1, column 21 is not valid JSON"},
		{"a bad escape", "{\"db\":\n  {\"password\": \"s3\\qcr3t\"}}", "line 2, column 19 is not valid JSON"},
		{"not UTF-8", "{\"db\": {\"password\": \"s3\xffcr3t\"}}", "line 1, column 24 is not valid JSON"},
		{"two objects", "{}\n {}", "line 2, column 2 is not valid JSON"},
		{"a name twice", "{\"debug\": false,\n \"debug\": true}", "line 2, column 2 repeats a name of its object"},
		{
			"a name twice in a section", `{"db": {"password": "s3cr3t", "password": "s3cr3t"}}`,
			"line 1, column 31 repeats a name of its object",
		},
		{"a string", ` "s3cr3t"`, "line 1, column 2 is not a JSON object"},
		{"nothing", ``, "not valid JSON"},
	} {
		_, err := source.JSON([]byte(tt.data))
		if err == nil || err.Error() != tt.want {
			t.Errorf("%s: JSON error = %v, want %q", tt.name, err, tt.want)
		}
	}
}

func TestDefineFlag_Bool(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	source.DefineFlag(fs, "api-debug", "API.Debug", "", true)
	if err := fs.Parse([]string{"-api-debug"}); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Flags: fs})
	if err != nil {
		t.Fatal(err)
	}
	if v, ok, err := src.Lookup(source.Key{Flag: "api-debug"}); err != nil || !ok || v.Text != "true" {
		t.Errorf("Lookup = %+v, %v, %v", v, ok, err)
	}
}

type stringer struct{}

func (stringer) String() string { return "local" }

func TestScalars(t *testing.T) {
	t.Parallel()

	when := time.Date(2026, 11, 7, 9, 30, 0, 0, time.UTC)
	tree := map[string]any{"s": map[string]any{
		"int": 3, "int64": int64(-4), "uint64": uint64(5), "time": when, "stringer": stringer{}, "chan": make(chan int),
	}}
	src, err := source.Open(source.Sources{Files: []source.File{
		{Path: writeFile(t, "any", ""), Decode: func([]byte) (map[string]any, error) { return tree, nil }},
	}})
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"int": "3", "int64": "-4", "uint64": "5", "time": "2026-11-07T09:30:00Z", "stringer": "local",
	} {
		if v, ok, err := src.Lookup(source.Key{File: []string{"s", name}}); err != nil || !ok || v.Text != want {
			t.Errorf("Lookup(%s) = %+v, %v, %v, want %q", name, v, ok, err, want)
		}
	}
	_, _, err = src.Lookup(source.Key{File: []string{"s", "chan"}})
	if err == nil || !strings.Contains(err.Error(), "is a chan int") {
		t.Errorf("Lookup(chan) error = %v", err)
	}
}

func TestCoded(t *testing.T) {
	t.Parallel()

	inclusion := validator.Inclusion()
	if got := source.Coded(inclusion, "model.Priority"); !errors.Is(got, inclusion) {
		t.Errorf("Coded(inclusion) = %v, want it as it is", got)
	}
	got := source.Coded(errors.New(`slog: level string "x": unknown name`), "slog.Level")
	coded, ok := errors.AsType[validator.Coded](got)
	if !ok || coded.Code() != "invalid" || coded.Params()["type"] != "slog.Level" || strings.Contains(got.Error(), `"x"`) {
		t.Errorf("Coded(plain) = %q, want invalid naming the type, not the value", got.Error())
	}
}
