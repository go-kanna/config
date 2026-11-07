package gen_test

import (
	"flag"
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"testing"
	"time"

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
		"API_ALIAS",
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
	want := "validate: DATABASE_POOL_MAX_CONNS: invalid (type=int32); DATABASE_RETRIES: invalid (type=uint8)"
	if got := errString(err); got != want {
		t.Errorf("LoadDatabase error = %q, want %q", got, want)
	}
}

func TestGenerated_Rules(t *testing.T) {
	clearEnv(t)
	t.Setenv("DATABASE_POOL_MAX_CONNS", "0")

	_, err := emit.LoadDatabase(source.Env())
	want := "validate: DATABASE_WRITER_URL: blank; DATABASE_POOL_MAX_CONNS: greater_than_or_equal_to (count=1)"
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
	if got, want := errString(err), "validate: database.pool_max_conns: greater_than_or_equal_to (count=1)"; got != want {
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
	want := "validate: OAUTH_CLIENT_ID: both OAUTH_CLIENT_ID and OAUTH_CLIENT_ID_FILE are set; set one of them"
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
	want := "validate: API_MODE: inclusion; API_ADMIN: email; API_PUBLIC: port; " +
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
	if got, want := errString(err), "validate: API_PUBLIC: invalid (type=Port)"; got != want {
		t.Errorf("LoadAPI error = %q, want %q", got, want)
	}
}

// The rules and the hook report under the name of each setting.
func TestGenerated_Hook(t *testing.T) {
	clearEnv(t)
	t.Setenv("PORT", "0")
	t.Setenv("API_NAME", "app")
	t.Setenv("API_ALIAS", "app")

	_, err := emit.LoadAPI(source.Env())
	want := "validate: PORT: greater_than_or_equal_to (count=1); API_ALIAS: same_as_name"
	if got := errString(err); got != want {
		t.Errorf("LoadAPI error = %q, want %q", got, want)
	}
}
