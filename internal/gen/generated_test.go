package gen_test

import (
	"errors"
	"flag"
	"io/fs"
	"log/slog"
	"math"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/go-kanna/validator"

	"github.com/go-kanna/config/internal/gen/testdata/emit"
	"github.com/go-kanna/config/source"
)

// The tests below set variables of the process, which t.Setenv rules out for
// parallel tests. Each clears the variables the structs read, so that what the
// person running them has set does not count.

func clearEnv(t *testing.T) {
	t.Helper()

	for _, name := range []string{
		"DATABASE_ADMIN_URL", "DATABASE_WRITER_URL", "DATABASE_POOL_MAX_CONNS", "DATABASE_DIAL_TIMEOUT",
		"DATABASE_RATIO", "DATABASE_DEBUG", "DATABASE_RETRIES", "OAUTH_CLIENT_ID", "OAUTH_CLIENT_ID_FILE",
		"OAUTH_TOKEN", "PORT", "API_MODE", "API_ADMIN", "API_PUBLIC", "API_LEVEL", "API_LISTEN", "API_NAME",
		"API_ALIAS", "PATHS_HOST", "PATHS_HOST_NAME", "LIMITS_FLOOR", "LIMITS_CEILING", "LIMITS_SCALE",
		"UPLOAD_DIR_MODE", "UPLOAD_OFFSET", "FALLBACK_LEVEL", "FALLBACK_ADMIN", "FALLBACK_PUBLIC", "DATABASE_REPLICA_URL",
	} {
		t.Setenv(name, "")
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestGenerated_Defaults(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_WRITER_URL", "postgres://writer@db/app")

	got, err := emit.LoadDatabase(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	want := emit.Database{
		WriterURL: "postgres://writer@db/app", PoolMaxConns: 10, DialTimeout: 5 * time.Second, Ratio: 0.5,
	}
	if got != want {
		t.Errorf("LoadDatabase = %+v, want %+v", got, want)
	}
}

func TestGenerated_Environment(t *testing.T) { //nolint:paralleltest // it sets variables of the process
	clearEnv(t)
	for name, value := range map[string]string{
		"DATABASE_ADMIN_URL": "postgres://admin@db/app", "DATABASE_WRITER_URL": "postgres://writer@db/app",
		"DATABASE_POOL_MAX_CONNS": "20", "DATABASE_DIAL_TIMEOUT": "1m", "DATABASE_RATIO": "0.25",
		"DATABASE_DEBUG": "true", "DATABASE_RETRIES": "3",
	} {
		t.Setenv(name, value)
	}

	got, err := emit.LoadDatabase(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	want := emit.Database{
		AdminURL: "postgres://admin@db/app", WriterURL: "postgres://writer@db/app", PoolMaxConns: 20,
		DialTimeout: time.Minute, Ratio: 0.25, Debug: true, Retries: 3,
	}
	if got != want {
		t.Errorf("LoadDatabase = %+v, want %+v", got, want)
	}
}

// What cannot be read is reported alone, under the name of where it is set,
// without the value: the rules see the settings only once all are read.
func TestGenerated_Unreadable(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_POOL_MAX_CONNS", "ten")
	t.Setenv("DATABASE_RETRIES", "300")

	_, err := emit.LoadDatabase(source.Env())
	want := "validator: DATABASE_POOL_MAX_CONNS: invalid (type=int32); DATABASE_RETRIES: invalid (type=uint8)"
	if got := errString(err); got != want {
		t.Errorf("LoadDatabase error = %q, want %q", got, want)
	}
}

func TestGenerated_Rules(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_POOL_MAX_CONNS", "0")

	_, err := emit.LoadDatabase(source.Env())
	want := "validator: DATABASE_WRITER_URL: blank; DATABASE_POOL_MAX_CONNS: greater_than_or_equal_to (count=1)"
	if got := errString(err); got != want {
		t.Errorf("LoadDatabase error = %q, want %q", got, want)
	}
}

// A setting goes by the name of where it is set: a flag wins over the
// environment, which wins over a file, and the rules it breaks say so.
func TestGenerated_Sources(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_DIAL_TIMEOUT", "2s")

	path := filepath.Join(t.TempDir(), "settings.json")
	settings := `{"database": {"pool_max_conns": 0, "dial_timeout": "9s", "debug": true}}`
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	emit.DefineDatabaseFlags(fs)
	if err := fs.Parse([]string{"-database-writer-url", "postgres://flag@db/app", "-database-debug=false"}); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Flags: fs, Files: []source.File{{Path: path, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = emit.LoadDatabase(src)
	if got, want := errString(err), "validator: database.pool_max_conns: greater_than_or_equal_to (count=1)"; got != want {
		t.Errorf("LoadDatabase error = %q, want %q", got, want)
	}

	if err := os.WriteFile(path, []byte(`{"database": {"pool_max_conns": 5}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err = source.Open(source.Sources{Flags: fs, Files: []source.File{{Path: path, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err := emit.LoadDatabase(src)
	if err != nil {
		t.Fatal(err)
	}
	want := emit.Database{WriterURL: "postgres://flag@db/app", PoolMaxConns: 5, DialTimeout: 2 * time.Second, Ratio: 0.5}
	if got != want {
		t.Errorf("LoadDatabase = %+v, want %+v", got, want)
	}
}

func TestGenerated_SecretFile(t *testing.T) {
	clearEnv(t)
	path := filepath.Join(t.TempDir(), "client-id")
	if err := os.WriteFile(path, []byte("app-1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("OAUTH_CLIENT_ID_FILE", path)
	t.Setenv("OAUTH_TOKEN", "t0k3n")

	got, err := emit.LoadOAuth(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	if want := (emit.OAuth{ClientID: "app-1", AuthToken: "t0k3n"}); got != want {
		t.Errorf("LoadOAuth = %+v, want %+v", got, want)
	}

	t.Setenv("OAUTH_CLIENT_ID", "app-2")
	_, err = emit.LoadOAuth(source.Env())
	want := "validator: OAUTH_CLIENT_ID: both OAUTH_CLIENT_ID and OAUTH_CLIENT_ID_FILE are set; set one of them"
	if got := errString(err); got != want {
		t.Errorf("LoadOAuth error = %q, want %q", got, want)
	}
}

func TestGenerated_Types(t *testing.T) { //nolint:paralleltest // it sets variables of the process
	clearEnv(t)
	for name, value := range map[string]string{
		"PORT": "9090", "API_MODE": "slow", "API_ADMIN": "ops@example.com", "API_PUBLIC": "443",
		"API_LISTEN": "127.0.0.1", "API_NAME": "app",
	} {
		t.Setenv(name, value)
	}

	got, err := emit.LoadAPI(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	admin, _ := emit.NewEmail("ops@example.com")
	public, _ := emit.NewPort(443)
	want := emit.API{
		Port: 9090, Mode: emit.ModeSlow, Admin: admin, Public: public, Level: slog.LevelInfo,
		Listen: netip.MustParseAddr("127.0.0.1"), Name: "app",
	}
	if got != want {
		t.Errorf("LoadAPI = %+v, want %+v", got, want)
	}
}

// What a constructor or UnmarshalText rejects is reported by its code, or
// else as invalid, naming the type rather than the value.
func TestGenerated_Rejected(t *testing.T) {
	clearEnv(t)
	for name, value := range map[string]string{
		"PORT": "0", "API_MODE": "medium", "API_ADMIN": "ops", "API_PUBLIC": "70000", "API_LEVEL": "loud",
		"API_LISTEN": "localhost", "API_NAME": "app",
	} {
		t.Setenv(name, value)
	}

	_, err := emit.LoadAPI(source.Env())
	want := "validator: API_MODE: inclusion; API_ADMIN: email; API_PUBLIC: port; " +
		"API_LEVEL: invalid (type=slog.Level); API_LISTEN: invalid (type=netip.Addr)"
	if got := errString(err); got != want {
		t.Errorf("LoadAPI error = %q, want %q", got, want)
	}

	t.Setenv("API_MODE", "")
	t.Setenv("API_ADMIN", "")
	t.Setenv("API_PUBLIC", "port")
	t.Setenv("API_LEVEL", "")
	t.Setenv("API_LISTEN", "")
	_, err = emit.LoadAPI(source.Env())
	if got, want := errString(err), "validator: API_PUBLIC: invalid (type=Port)"; got != want {
		t.Errorf("LoadAPI error = %q, want %q", got, want)
	}
}

// A default that cannot be read is reported as one, without its text.
func TestGenerated_RejectedDefault(t *testing.T) {
	clearEnv(t)

	_, err := emit.LoadFallback(source.Env())
	want := "validator: FALLBACK_LEVEL: invalid (default=true, type=slog.Level); " +
		"FALLBACK_ADMIN: email (default=true); FALLBACK_PUBLIC: invalid (default=true, type=Port)"
	if got := errString(err); got != want {
		t.Errorf("LoadFallback error = %q, want %q", got, want)
	}

	t.Setenv("FALLBACK_LEVEL", "loud")
	t.Setenv("FALLBACK_ADMIN", "ops@example.com")
	t.Setenv("FALLBACK_PUBLIC", "8080")
	_, err = emit.LoadFallback(source.Env())
	if got, want := errString(err), "validator: FALLBACK_LEVEL: invalid (type=slog.Level)"; got != want {
		t.Errorf("LoadFallback error = %q, want %q", got, want)
	}
}

// The rules and the hook report under the name of each setting.
func TestGenerated_Hook(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "0")
	t.Setenv("API_NAME", "app")
	t.Setenv("API_ALIAS", "app")

	_, err := emit.LoadAPI(source.Env())
	want := "validator: PORT: greater_than_or_equal_to (count=1); API_ALIAS: same_as_name"
	if got := errString(err); got != want {
		t.Errorf("LoadAPI error = %q, want %q", got, want)
	}
}

// The hook's errors move under the path of the setting in their segments too,
// the section and the key of a file, so that Field and Segments agree.
func TestGenerated_HookSegments(t *testing.T) {
	clearEnv(t)
	t.Setenv("API_NAME", "app")

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"api": {"alias": "app"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Files: []source.File{{Path: path, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = emit.LoadAPI(src)
	errs, ok := errors.AsType[validator.Errors](err)
	if !ok || len(errs) != 1 {
		t.Fatalf("LoadAPI error = %v, want one error", err)
	}
	want := []validator.Segment{
		{Kind: validator.FieldSegment, Name: "api"}, {Kind: validator.FieldSegment, Name: "alias"},
	}
	if fe := errs[0]; fe.Field != "api.alias" || !slices.Equal(fe.Segments, want) {
		t.Errorf("LoadAPI error at %q %+v, want %q %+v", fe.Field, fe.Segments, "api.alias", want)
	}
}

// What the hook reports goes under the field whose JSON name is its first
// segment, even when another's JSON name begins with it.
func TestGenerated_HookFirstSegment(t *testing.T) {
	clearEnv(t)
	t.Setenv("PATHS_HOST_NAME", "-")

	_, err := emit.LoadPaths(source.Env())
	if got, want := errString(err), "validator: PATHS_HOST_NAME: dash"; got != want {
		t.Errorf("LoadPaths error = %q, want %q", got, want)
	}
}

// A default that no constant holds is written as the call of package math
// returning it, converted to the field's type.
func TestGenerated_NonFiniteDefaults(t *testing.T) { //nolint:paralleltest // it sets variables of the process
	clearEnv(t)

	got, err := emit.LoadLimits(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	if !math.IsInf(got.Floor, -1) || !math.IsInf(float64(got.Ceiling), 1) || !math.IsNaN(float64(got.Scale)) {
		t.Errorf("LoadLimits = %+v, want -Inf, +Inf, and NaN", got)
	}
}

// An integer reads as a Go literal does, in its default, a variable, a flag,
// and a file alike: 0755 is octal.
func TestGenerated_IntegerLiterals(t *testing.T) {
	clearEnv(t)

	got, err := emit.LoadUpload(source.Env())
	if err != nil {
		t.Fatal(err)
	}
	if want := (emit.Upload{DirMode: 0o755, Offset: -16}); got != want {
		t.Errorf("LoadUpload = %+v, want %+v", got, want)
	}

	for text, want := range map[string]fs.FileMode{
		"0644": 0o644, "0o600": 0o600, "0x1F": 31, "0b101": 5, "1_000": 1000, "0": 0, "10": 10,
	} {
		t.Setenv("UPLOAD_DIR_MODE", text)
		got, err := emit.LoadUpload(source.Env())
		if err != nil {
			t.Errorf("LoadUpload of %q: %v", text, err)
		} else if got.DirMode != want {
			t.Errorf("LoadUpload of %q = %v, want %v", text, got.DirMode, want)
		}
	}

	for _, text := range []string{"0x", "1__000", "_1", "0x1_", "-1", "0x100000000", "0755x", "0_8"} {
		t.Setenv("UPLOAD_DIR_MODE", text)
		_, err := emit.LoadUpload(source.Env())
		if got, want := errString(err), "validator: UPLOAD_DIR_MODE: invalid (type=fs.FileMode)"; got != want {
			t.Errorf("LoadUpload error of %q = %q, want %q", text, got, want)
		}
	}
	// A leading 0 that makes a decimal octal is told, without the value.
	for _, text := range []string{"08", "08080", "0099999999999999999999"} {
		t.Setenv("UPLOAD_DIR_MODE", text)
		_, err := emit.LoadUpload(source.Env())
		if got, want := errString(err), "validator: UPLOAD_DIR_MODE: invalid (octal=true, type=fs.FileMode)"; got != want {
			t.Errorf("LoadUpload error of %q = %q, want %q", text, got, want)
		}
	}
	t.Setenv("UPLOAD_DIR_MODE", "")
	t.Setenv("UPLOAD_OFFSET", "-09")
	_, err = emit.LoadUpload(source.Env())
	if got, want := errString(err), "validator: UPLOAD_OFFSET: invalid (octal=true, type=int16)"; got != want {
		t.Errorf("LoadUpload error of -09 = %q, want %q", got, want)
	}
	t.Setenv("UPLOAD_OFFSET", "")

	path := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(path, []byte(`{"upload": {"dir_mode": "0700"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	flags := flag.NewFlagSet("app", flag.ContinueOnError)
	emit.DefineUploadFlags(flags)
	if err := flags.Parse([]string{"-upload-offset", "-0b11"}); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Flags: flags, Files: []source.File{{Path: path, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}
	got, err = emit.LoadUpload(src)
	if err != nil {
		t.Fatal(err)
	}
	if want := (emit.Upload{DirMode: 0o700, Offset: -3}); got != want {
		t.Errorf("LoadUpload = %+v, want %+v", got, want)
	}
}

// A key of the section that no struct of the package reads is an error under
// its name, without its value; a key of Replica, which shares the section, and
// a section of another struct are not.
func TestGenerated_UnknownKeys(t *testing.T) { //nolint:paralleltest // it sets variables of the process
	clearEnv(t)

	path := filepath.Join(t.TempDir(), "settings.json")
	settings := `{
		"database": {"writer_url": "postgres://writer@db/app", "replica_url": "r", "pool_max_con": 99, "Debug": true},
		"Database": {},
		"api": {"secret": "s"}
	}`
	if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	src, err := source.Open(source.Sources{Files: []source.File{{Path: path, Decode: source.JSON}}})
	if err != nil {
		t.Fatal(err)
	}

	_, err = emit.LoadDatabase(src)
	want := "validator: Database: names no section; did you mean database?; " +
		"database.Debug: is no setting of the section database; did you mean database.debug?; " +
		"database.pool_max_con: is no setting of the section database"
	if got := errString(err); got != want {
		t.Errorf("LoadDatabase error = %q, want %q", got, want)
	}
	if _, err := emit.LoadReplica(src); errString(err) != want {
		t.Errorf("LoadReplica error = %q, want %q", errString(err), want)
	}
}

// An error of a setting in a file goes under its section and its key, as two
// segments, and one of a variable under the variable, as one.
func TestGenerated_FileSegments(t *testing.T) {
	clearEnv(t)

	load := func(t *testing.T, settings string) validator.Errors {
		t.Helper()

		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(settings), 0o600); err != nil {
			t.Fatal(err)
		}
		src, err := source.Open(source.Sources{Files: []source.File{{Path: path, Decode: source.JSON}}})
		if err != nil {
			t.Fatal(err)
		}
		_, err = emit.LoadDatabase(src)
		errs, _ := errors.AsType[validator.Errors](err)
		return errs
	}
	fields := func(names ...string) []validator.Segment {
		segments := make([]validator.Segment, len(names))
		for i, n := range names {
			segments[i] = validator.Segment{Kind: validator.FieldSegment, Name: n}
		}
		return segments
	}
	check := func(t *testing.T, errs validator.Errors, want ...[]validator.Segment) {
		t.Helper()

		got := make([][]validator.Segment, len(errs))
		for i, fe := range errs {
			got[i] = fe.Segments
		}
		if !slices.EqualFunc(got, want, slices.Equal) {
			t.Errorf("LoadDatabase errors %v at %+v, want %+v", errs, got, want)
		}
	}

	errs := load(t, `{"database": {"writer_url": "w", "pool_max_conns": "many", "pool_max_con": 1}}`)
	check(t, errs, fields("database", "pool_max_conns"), fields("database", "pool_max_con"))

	errs = load(t, `{"database": {"writer_url": "w", "pool_max_conns": 0}}`)
	check(t, errs, fields("database", "pool_max_conns"))

	t.Setenv("DATABASE_POOL_MAX_CONNS", "0")
	errs = load(t, `{"database": {"writer_url": "w"}}`)
	check(t, errs, fields("DATABASE_POOL_MAX_CONNS"))
}
