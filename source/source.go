// Package source finds the settings that the code kanna-config generates reads
// a struct from. A setting is looked for in the flags, then in the environment,
// then in the files, the last first, and the first place that gives it text
// gives its value. An empty text gives none, as an unset one does.
package source

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kanna/validate"
)

// Key names one setting in each place it may be found.
type Key struct {
	// Env is the environment variable, such as DATABASE_WRITER_URL.
	Env string

	// File is the path of the setting in a file, its section first, such as
	// database and writer_url.
	File []string

	// Flag is the flag, without its dash, such as database-writer-url.
	Flag string
}

// Value is the text a setting is given, and the name it goes by where it is
// found: the variable, the path in the file, or the flag.
type Value struct {
	Text string
	Name string
}

// Source finds settings.
type Source interface {
	// Lookup returns the text k is given where it is found first, and false
	// when it is found nowhere. An error says why what is there cannot be
	// read, and Value.Name names where.
	Lookup(k Key) (Value, bool, error)
}

// Sources are where settings are read from besides the environment, which is
// always read.
type Sources struct {
	// Flags are the flags parsed, of which those set count. The code
	// kanna-config generates defines a struct's flags with DefineFlag.
	Flags *flag.FlagSet

	// Files are read in order, so that a later file wins over an earlier one.
	Files []File
}

// File is a file of settings.
type File struct {
	Path string

	// Decode reads the file into its sections, such as JSON does. The modules
	// github.com/go-kanna/config/yaml and github.com/go-kanna/config/toml
	// read the other formats.
	Decode func(data []byte) (map[string]any, error)
}

// Open reads the files of s, and returns the Source that looks for a setting
// in the flags of s, then in the environment, then in its files.
func Open(s Sources) (Source, error) {
	src := &sources{}
	if s.Flags != nil {
		src.flags = make(map[string]string)
		s.Flags.Visit(func(f *flag.Flag) { src.flags[f.Name] = f.Value.String() })
	}
	for _, f := range s.Files {
		if f.Decode == nil {
			return nil, fmt.Errorf("read %s: no Decode", f.Path)
		}
		data, err := os.ReadFile(f.Path)
		if err != nil {
			return nil, fmt.Errorf("read settings: %w", err)
		}
		tree, err := f.Decode(data)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Path, err)
		}
		src.files = append(src.files, file{path: f.Path, tree: tree})
	}
	return src, nil
}

// Env returns the Source of the environment alone.
func Env() Source {
	return &sources{}
}

type sources struct {
	flags map[string]string
	files []file
}

type file struct {
	path string
	tree map[string]any
}

func (s *sources) Lookup(k Key) (Value, bool, error) {
	if text := s.flags[k.Flag]; k.Flag != "" && text != "" {
		return Value{Text: text, Name: "-" + k.Flag}, true, nil
	}
	if v, ok, err := lookupEnv(k.Env); err != nil || ok {
		return v, ok, err
	}
	for _, f := range slices.Backward(s.files) {
		if v, ok, err := f.lookup(k.File); err != nil || ok {
			return v, ok, err
		}
	}
	return Value{}, false, nil
}

// lookupEnv finds the variable name, or the file that name_FILE names, whose
// content is the value, less one line ending at its end. That is how a secret
// mounted as a file is given, such as a secret of Kubernetes.
func lookupEnv(name string) (Value, bool, error) {
	if name == "" {
		return Value{}, false, nil
	}
	text, path := os.Getenv(name), os.Getenv(name+"_FILE")
	switch {
	case text != "" && path != "":
		return Value{Name: name}, false, fmt.Errorf("both %s and %s_FILE are set; set one of them", name, name)
	case text != "":
		return Value{Text: text, Name: name}, true, nil
	case path == "":
		return Value{}, false, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the file the person running the program names
	if err != nil {
		return Value{Name: name + "_FILE"}, false, fmt.Errorf("read the file %s_FILE names: %w", name, err)
	}
	text = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if text == "" {
		return Value{}, false, nil
	}
	return Value{Text: text, Name: name + "_FILE"}, true, nil
}

func (f file) lookup(path []string) (Value, bool, error) {
	if len(path) == 0 {
		return Value{}, false, nil
	}
	name := strings.Join(path, ".")
	var node any = f.tree
	for _, p := range path {
		section, ok := node.(map[string]any)
		if !ok {
			above := strings.Join(path[:len(path)-1], ".")
			return Value{Name: name}, false, fmt.Errorf("%s of %s is a value, with nothing under it", above, f.path)
		}
		if node, ok = section[p]; !ok {
			return Value{}, false, nil
		}
	}
	text, err := scalar(node)
	if err != nil {
		return Value{Name: name}, false, fmt.Errorf("%s of %s %w", name, f.path, err)
	}
	if text == "" {
		return Value{}, false, nil
	}
	return Value{Text: text, Name: name}, true, nil
}

// errNotValue reports a section or a list where a setting's value goes.
var errNotValue = errors.New("is not a value")

// scalar returns the text of a value as a decoder of a file gives it, which
// the code kanna-config generates reads as it reads a variable.
func scalar(v any) (string, error) {
	switch v := v.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case bool:
		return strconv.FormatBool(v), nil
	case int:
		return strconv.Itoa(v), nil
	case int64:
		return strconv.FormatInt(v, 10), nil
	case uint64:
		return strconv.FormatUint(v, 10), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case time.Time:
		return v.Format(time.RFC3339Nano), nil
	case map[string]any:
		return "", fmt.Errorf("is a section, where a value goes: %w", errNotValue)
	case []any:
		return "", fmt.Errorf("is a list, where a value goes: %w", errNotValue)
	case fmt.Stringer:
		return v.String(), nil
	}
	return "", fmt.Errorf("is a %T, which is not a value: %w", v, errNotValue)
}

// DefineFlag defines on fs the flag name, whose text a Source opened with fs
// finds. usage says what it sets, def is the default the flag's help shows,
// and a bool flag may stand alone, as -debug does for -debug=true.
func DefineFlag(fs *flag.FlagSet, name, usage, def string, isBool bool) {
	fs.Var(&flagValue{text: def, isBool: isBool}, name, usage)
}

// flagValue holds the text a flag is given, for the code kanna-config
// generates to read as it reads a variable.
type flagValue struct {
	text   string
	isBool bool
}

func (v *flagValue) String() string {
	if v == nil {
		return ""
	}
	return v.text
}

func (v *flagValue) Set(text string) error {
	v.text = text
	return nil
}

func (v *flagValue) IsBoolFlag() bool {
	return v.isBool
}

// Coded returns err, which a constructor or an UnmarshalText returned for the
// text of a setting, when it carries a code, and otherwise invalid naming typ:
// the text of err may hold the value, which a setting may keep secret.
func Coded(err error, typ string) error {
	if _, ok := errors.AsType[validate.Coded](err); ok {
		return err
	}
	return validate.FailWith("invalid", map[string]any{"type": typ})
}
