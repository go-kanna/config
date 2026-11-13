package impl_test

import (
	"errors"
	"flag"
	"strconv"
	"strings"
	"testing"

	"github.com/go-kanna/validator"

	"github.com/go-kanna/config/impl"
	"github.com/go-kanna/config/source"
)

func TestDefineFlag_Bool(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	impl.DefineFlag(fs, "api-debug", "API.Debug", "", true)
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

// Two structs of one name in two packages read one setting, defining its flag
// twice.
func TestDefineFlag_Shared(t *testing.T) {
	t.Parallel()

	fs := flag.NewFlagSet("app", flag.ContinueOnError)
	impl.DefineFlag(fs, "config-host", "Config.Host", "db", false)
	impl.DefineFlag(fs, "config-host", "Config.Host", "cache", false)
	if err := fs.Parse([]string{"-config-host", "local"}); err != nil {
		t.Fatal(err)
	}
	if f := fs.Lookup("config-host"); f.DefValue != "db" || f.Value.String() != "local" {
		t.Errorf("flag = %q, default %q, want local, default db", f.Value, f.DefValue)
	}
}

func TestCoded(t *testing.T) {
	t.Parallel()

	inclusion := validator.Inclusion()
	if got := impl.Coded(inclusion, "model.Priority"); !errors.Is(got, inclusion) {
		t.Errorf("Coded(inclusion) = %v, want it as it is", got)
	}
	got := impl.Coded(errors.New(`slog: level string "x": unknown name`), "slog.Level")
	coded, ok := errors.AsType[validator.Coded](got)
	if !ok || coded.Code() != "invalid" || coded.Params()["type"] != "slog.Level" || strings.Contains(got.Error(), `"x"`) {
		t.Errorf("Coded(plain) = %q, want invalid naming the type, not the value", got.Error())
	}
	_, err := strconv.ParseInt("08080", 0, 32)
	if got, want := impl.Coded(err, "int32").Error(), "validator: invalid (octal=true, type=int32)"; got != want {
		t.Errorf("Coded(08080) = %q, want %q", got, want)
	}
}

func TestDefault(t *testing.T) {
	t.Parallel()

	tooLong := validator.FailWith("too_long", map[string]any{"count": 10})
	got := impl.Default(tooLong)
	if !errors.Is(got, tooLong) {
		t.Errorf("Default(too_long) = %v, does not reach it", got)
	}
	if want := "validator: too_long (count=10, default=true)"; got.Error() != want {
		t.Errorf("Default(too_long) = %q, want %q", got.Error(), want)
	}
	if _, ok := tooLong.Params()["default"]; ok {
		t.Error("Default changed the params of the error it marks")
	}

	got = impl.Default(impl.Coded(errors.New(`slog: level string "loud": unknown name`), "slog.Level"))
	if want := "validator: invalid (default=true, type=slog.Level)"; got.Error() != want {
		t.Errorf("Default(invalid) = %q, want %q", got.Error(), want)
	}
}

func TestUnknownKey(t *testing.T) {
	t.Parallel()

	keys := []string{"pool_max_conns", "writer_url"}
	for _, tt := range []struct {
		path validator.Path
		want string
	}{
		{validator.Field("database", "pool_max_con"), "is no setting of the section database"},
		{
			validator.Field("database", "Writer_URL"),
			"is no setting of the section database; did you mean database.writer_url?",
		},
		{validator.Field("Database"), "names no section; did you mean database?"},
		{validator.Field("database.writer_url"), "names no section; did you mean database?"},
	} {
		if got := impl.UnknownKey(tt.path, "database", keys).Error(); got != tt.want {
			t.Errorf("UnknownKey(%s) = %q, want %q", tt.path, got, tt.want)
		}
	}
}
