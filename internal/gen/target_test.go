package gen_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"

	"github.com/go-kanna/config/internal/gen"
)

func targets(t *testing.T, src string) ([]string, []string) {
	t.Helper()

	pkg := pkgtest.Load(t, map[string]string{"test.go": src})
	return gen.Targets(pkg.Fset, []*packages.Package{pkg})
}

// mode is an enum as kanna-value generates one.
const mode = `
type Mode string

const (
	ModeFast Mode = "fast"
	ModeSlow Mode = "slow"
)

func NewMode(v string) (Mode, error) { return Mode(v), nil }
`

func TestTargets_Diagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		fields string
		want   string
	}{
		{
			name: "unexported", fields: "port int",
			want: "test.go:5:2: error: an unexported field cannot be read from the settings (field port)\n" +
				`  tag it config:"-" to leave it out, or give it a name of its own`,
		},
		{name: "embedded", fields: "Mode", want: "an embedded field cannot be read from the settings (field Mode)"},
		{name: "blank", fields: "_ int", want: "a field named _ cannot be read from the settings (field _)"},
		{
			name: "words not in snake case", fields: "Token string `config:\"AuthToken\"`",
			want: `config:"AuthToken" is not words in snake case (field Token)` + "\n" +
				`  write it in lower case, its words joined by _, as config:"auth_token"`,
		},
		{
			name: "env of -", fields: "Port int `env:\"-\"`",
			want: `env:"-" names no variable (field Port)` + "\n" + `  tag it config:"-" to leave it out of every source`,
		},
		{
			name: "an option of caarlos0/env", fields: "Port int `env:\"PORT,required\"`",
			want: `kanna-config takes no option "required" in the env tag (field Port)` + "\n" +
				"  kanna-config does not tell whether a variable is set",
		},
		{
			name: "an unknown option", fields: "Port int `env:\"PORT,loud\"`",
			want: "  kanna-config takes the name of the variable alone",
		},
		{
			name: "a name no variable has", fields: "Port int `env:\"A=B\"`",
			want: `env:"A=B" cannot name a variable, which is neither empty nor holds = or NUL (field Port)`,
		},
		{
			name: "a tag of caarlos0/env", fields: "Port int `envDefault:\"8080\"`",
			want: "kanna-config does not read the tag envDefault of caarlos0/env (field Port)\n" +
				`  write the default as default:"…"`,
		},
		{
			name: "a type it cannot read", fields: "Hosts []string",
			want: "kanna-config cannot read []string (field Hosts)",
		},
		{
			name: "a default it cannot read", fields: "Port int `default:\"ten\"`",
			want: `the default "ten" is not an int (field Port)`,
		},
		{
			name: "a default out of range", fields: "Port int8 `default:\"300\"`",
			want: `the default "300" is not an int8 (field Port)`,
		},
		{
			name: "a default not a Go literal", fields: "Mode uint32 `default:\"08\"`",
			want: `the default "08" is not an uint32 (field Mode)`,
		},
		{
			name: "a default out of range in hex", fields: "Port int8 `default:\"0x80\"`",
			want: `the default "0x80" is not an int8 (field Port)`,
		},
		{
			name: "an octal default a rule rejects", fields: "Mode uint32 `default:\"0755\" validate:\"lte=400\"`",
			want: `the default "0755" breaks a rule of its validate tag: less_than_or_equal_to (count=400) (field Mode)`,
		},
		{
			name: "a default a rule rejects", fields: "Port int `default:\"0\" validate:\"gte=1\"`",
			want: `the default "0" breaks a rule of its validate tag: greater_than_or_equal_to (count=1) (field Port)`,
		},
		{
			name: "a default no constant holds", fields: "Mode Mode `default:\"fats\"`",
			want: `the default "fats" is none of the constants of Mode; did you mean "fast"? (field Mode)`,
		},
		{
			name: "a default of a field not read", fields: "Secret string `config:\"-\" default:\"x\"`",
			want: `a field tagged config:"-" is not read, so its default is never used (field Secret)`,
		},
		{
			name: "rules of a field not read", fields: "Secret string `config:\"-\" validate:\"required\"`",
			want: "its validate tag is never checked: kanna-config does not read the field, " +
				"and Settings has no //kanna:validate (field Secret)",
		},
		{
			name: "two fields of one variable", fields: "A string `env:\"SHARED\"`\n\tB string `env:\"SHARED\"`",
			want: "B reads the variable SHARED that A reads too (field B)",
		},
		{
			name: "a variable naming the file of another", fields: "Password string\n\tPasswordFile string",
			want: "PasswordFile reads the variable SETTINGS_PASSWORD_FILE, which names the file of " +
				"the variable SETTINGS_PASSWORD that Password reads (field PasswordFile)",
		},
		{
			name:   "a variable naming the file of a later one",
			fields: "KeyFile string `env:\"KEY_FILE\"`\n\tKey string `env:\"KEY\"`",
			want: "KeyFile reads the variable KEY_FILE, which names the file of " +
				"the variable KEY that Key reads (field KeyFile)",
		},
		{
			name: "a rule that does not apply", fields: "Port int `validate:\"max=5\"`",
			want: `rule "max" cannot apply to int (field Port)`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := "package test\n\n//kanna:config\ntype Settings struct {\n\t" + tt.fields + "\n}\n" + mode
			names, diags := targets(t, src)
			if len(names) > 0 {
				t.Errorf("targets = %q, want none", names)
			}
			if !slices.ContainsFunc(diags, func(d string) bool { return strings.Contains(d, tt.want) }) {
				t.Errorf("diagnostics = %q, want one containing %q", diags, tt.want)
			}
		})
	}
}

func TestTargets_StructDiagnostics(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		src  string
		want string
	}{
		{
			name: "a prefix not in snake case",
			src:  "package test\n\n//kanna:config prefix=OAuth\ntype OAuth struct{ ClientID string }\n",
			want: "test.go:3:1: error: prefix=OAuth is not words in snake case\n" +
				"  write it in lower case, its words joined by _, as prefix=o_auth",
		},
		{
			name: "an argument it does not take",
			src:  "package test\n\n//kanna:config strict\ntype Settings struct{ Port int }\n",
			want: "strict",
		},
		{
			name: "two structs agreeing on some names",
			src: "package test\n\n//kanna:config\ntype Database struct{ WriterURL string }\n\n" +
				"//kanna:config\ntype DatabaseWriter struct{ URL string }\n",
			want: "test.go:7:29: error: DatabaseWriter.URL reads the variable DATABASE_WRITER_URL and the flag " +
				"-database-writer-url that Database.WriterURL reads too, but the key database_writer.url " +
				"where it reads the key database.writer_url (field URL)\n" +
				`  give the two the same names to share the setting, or names of their own, with prefix=, config:"…", or env:"…"`,
		},
		{
			name: "a variable naming the file of another struct's",
			src: "package test\n\n//kanna:config\ntype App struct{ Log string }\n\n" +
				"//kanna:config\ntype AppLog struct{ File string }\n",
			want: "AppLog.File reads the variable APP_LOG_FILE, which names the file of " +
				"the variable APP_LOG that App.Log reads (field File)",
		},
		{
			name: "a variable naming the file of a later struct's",
			src: "package test\n\n//kanna:config\ntype AppLog struct{ File string }\n\n" +
				"//kanna:config\ntype App struct{ Log string }\n",
			want: "AppLog.File reads the variable APP_LOG_FILE, which names the file of " +
				"the variable APP_LOG that App.Log reads (field File)",
		},
		{
			name: "generic",
			src:  "package test\n\n//kanna:config\ntype Box[T any] struct{ V T }\n",
			want: "test.go:4:6: error: generic struct Box is not supported",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			_, diags := targets(t, tt.src)
			if !slices.ContainsFunc(diags, func(d string) bool { return strings.Contains(d, tt.want) }) {
				t.Errorf("diagnostics = %q, want one containing %q", diags, tt.want)
			}
		})
	}
}

// What only a struct marked //kanna:validate checks, kanna-config leaves to
// it, and a struct of no field to read is still read.
func TestTargets_Fine(t *testing.T) {
	t.Parallel()

	names, diags := targets(t, `package test

//kanna:config
//kanna:validate
type Settings struct {
	Secret string `+"`config:\"-\" validate:\"required\"`"+`
	Port   int    `+"`validate:\"gte=1\" default:\"8080\"`"+`
	Level  Level  `+"`validate:\"required\" default:\"0\"`"+`
}

// Level counts from 0, which its IsZero does not take for unset.
type Level int

func (Level) IsZero() bool { return false }

//kanna:config
type Empty struct{}

// DB reads the setting Settings.Port reads, as sharing it may be meant.
//
//kanna:config prefix=settings
type DB struct{ Port int }
`)
	if want := []string{"Settings", "Empty", "DB"}; !slices.Equal(names, want) || len(diags) > 0 {
		t.Errorf("targets = %q, diagnostics = %q, want %q and none", names, diags, want)
	}
}
