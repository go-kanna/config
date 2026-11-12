// Package source finds the settings that the code kanna-config generates reads
// a struct from. A setting is looked for in the flags, then in the environment,
// then in the files, the last first, and the first place that gives it text
// gives its value. An empty text gives none, as an unset one does.
package source

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kanna/validator"

	"github.com/go-kanna/config/internal/octal"
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

// Value is the text a setting is given, and the path it goes by where it is
// found.
type Value struct {
	// Text is what the setting is given, which the code kanna-config generates
	// reads as the text of a variable.
	Text string

	// Path is where Text was found, which an error is reported under: the
	// variable (or NAME_FILE, when it was read from the file that names) or
	// the flag with its dash, such as -database-writer-url, as one field
	// segment, or the path in a file as a segment each, such as database and
	// writer_url, spelled database.writer_url.
	Path validator.Path
}

// Source finds settings. The generated LoadX asks it for each setting through
// Lookup, and for what else a section holds through Unknown, so a Source of
// your own, such as a fixed one in a test, may take the place of what Open and
// Env return, and then decides for itself where to look and in what order. It
// is not part of the order above.
type Source interface {
	// Lookup returns the text k is given where it is found first, and false
	// when it is found nowhere. An error says why what is there cannot be
	// read, and Value.Path says where.
	Lookup(k Key) (Value, bool, error)

	// Unknown returns what is set under section, the words of a struct, but
	// is none of keys, the keys of its fields, under the path each goes by,
	// such as database.pool_max_con, once each: the generated LoadX reports
	// each as an error. A Source that reads no file returns none.
	Unknown(section string, keys []string) []Value
}

// Sources are where settings are read from besides the environment, which is
// always read.
type Sources struct {
	// Flags are the flags parsed, of which those set count, and a FlagSet not
	// parsed yet is an error. The code kanna-config generates defines a
	// struct's flags with DefineFlag.
	Flags *flag.FlagSet

	// Files are read in order, so that a later file wins over an earlier one.
	Files []File
}

// File is a file of settings.
type File struct {
	// Path is the file Open reads, and what an error names it by.
	Path string

	// Decode reads the file into its sections, such as JSON does. The modules
	// github.com/go-kanna/config/yaml and github.com/go-kanna/config/toml
	// read the other formats. Open returns its error as it is, which should
	// say where the data is wrong, never what it holds, which may be a secret.
	//
	// A section is a map[string]any, and a value in it, a leaf, is nil (unset),
	// a string, a bool, an int, an int64, a uint64, a float64, a time.Time
	// (read as RFC 3339), or a fmt.Stringer (read as its String). A value of
	// any other type, a list ([]any) or a section where a value goes included,
	// is an error where the setting is read.
	Decode func(data []byte) (map[string]any, error)
}

// Open reads the files of s, and returns the Source that looks for a setting
// in the flags of s, then in the environment, then in its files.
func Open(s Sources) (Source, error) {
	src := &sources{}
	if s.Flags != nil {
		if !s.Flags.Parsed() {
			return nil, errors.New("read settings: the flags are not parsed; call Parse on the FlagSet before Open")
		}
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

// Unknown returns, for each file in order, the keys under section that are
// none of keys, and the sections at the top whose names differ from section
// in case alone, such as Database. A section of no struct is not looked at,
// as another LoadX may read it.
func (s *sources) Unknown(section string, keys []string) []Value {
	var (
		found []Value
		seen  = map[string]bool{}
	)
	add := func(p validator.Path, v any) {
		if name := p.String(); !seen[name] {
			seen[name] = true
			text, _ := scalar(v)
			found = append(found, Value{Text: text, Path: p})
		}
	}
	for _, f := range s.files {
		for _, name := range slices.Sorted(maps.Keys(f.tree)) {
			if name != section && strings.EqualFold(name, section) {
				add(validator.Field(name), f.tree[name])
			}
		}
		tree, _ := f.tree[section].(map[string]any)
		for _, name := range slices.Sorted(maps.Keys(tree)) {
			if !slices.Contains(keys, name) {
				add(validator.Field(section, name), tree[name])
			}
		}
	}
	return found
}

func (s *sources) Lookup(k Key) (Value, bool, error) {
	if text := s.flags[k.Flag]; k.Flag != "" && text != "" {
		return Value{Text: text, Path: validator.Field("-" + k.Flag)}, true, nil
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
		err := fmt.Errorf("both %s and %s_FILE are set; set one of them", name, name)
		return Value{Path: validator.Field(name)}, false, err
	case text != "":
		return Value{Text: text, Path: validator.Field(name)}, true, nil
	case path == "":
		return Value{}, false, nil
	}
	data, err := os.ReadFile(path) //nolint:gosec // the file the person running the program names
	if err != nil {
		return Value{Path: validator.Field(name + "_FILE")}, false, fmt.Errorf("read the file %s_FILE names: %w", name, err)
	}
	text = strings.TrimSuffix(strings.TrimSuffix(string(data), "\n"), "\r")
	if text == "" {
		return Value{}, false, nil
	}
	return Value{Text: text, Path: validator.Field(name + "_FILE")}, true, nil
}

func (f file) lookup(path []string) (Value, bool, error) {
	if len(path) == 0 {
		return Value{}, false, nil
	}
	at := validator.Field(path[0], path[1:]...)
	var node any = f.tree
	for i, p := range path {
		if node == nil {
			return Value{}, false, nil
		}
		section, ok := node.(map[string]any)
		if !ok {
			above := strings.Join(path[:i], ".")
			return Value{Path: at}, false, fmt.Errorf("%s is %s in %s, where a section goes", above, shape(node), f.path)
		}
		if node, ok = section[p]; !ok {
			return Value{}, false, nil
		}
	}
	text, ok := scalar(node)
	if !ok {
		return Value{Path: at}, false, fmt.Errorf("is %s in %s, where a value goes", shape(node), f.path)
	}
	if text == "" {
		return Value{}, false, nil
	}
	return Value{Text: text, Path: at}, true, nil
}

// scalar returns the text of a value as a decoder of a file gives it, which
// the code kanna-config generates reads as it reads a variable, and false for
// what is not a value, such as a section or a list.
func scalar(v any) (string, bool) {
	switch v := v.(type) {
	case nil:
		return "", true
	case string:
		return v, true
	case bool:
		return strconv.FormatBool(v), true
	case int:
		return strconv.Itoa(v), true
	case int64:
		return strconv.FormatInt(v, 10), true
	case uint64:
		return strconv.FormatUint(v, 10), true
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true
	case time.Time:
		return v.Format(time.RFC3339Nano), true
	case map[string]any, []any:
		return "", false
	case fmt.Stringer:
		return v.String(), true
	}
	return "", false
}

// shape names what v, which a decoder of a file gives, is, for an error to
// say where it does not go.
func shape(v any) string {
	if _, ok := scalar(v); ok {
		return "a value"
	}
	switch reflect.ValueOf(v).Kind() { //nolint:exhaustive // the rest are named by their type
	case reflect.Map, reflect.Struct:
		return "a section"
	case reflect.Slice, reflect.Array:
		return "a list"
	}
	return fmt.Sprintf("something of type %T", v)
}

// DefineFlag is for the generated DefineXFlags, not to be called by hand. It
// defines on fs the flag name, whose text a Source opened with fs
// finds. usage says what it sets, def is the default the flag's help shows,
// and a bool flag may stand alone, as -debug does for -debug=true. A flag fs
// defines already is left as it is, so that two structs reading one setting
// share its flag.
func DefineFlag(fs *flag.FlagSet, name, usage, def string, isBool bool) {
	if fs.Lookup(name) != nil {
		return
	}
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

// Coded is for the generated LoadX, not to be called by hand. It returns err,
// which reading the text of a setting returned, when it carries a code, and
// otherwise invalid naming typ: the text of err may hold the value, which a
// setting may keep secret. An integer that a leading 0 made octal, which reads
// as a decimal, gets octal=true besides.
func Coded(err error, typ string) error {
	if _, ok := errors.AsType[validator.Coded](err); ok {
		return err
	}
	p := map[string]any{"type": typ}
	if octal.Misread(err) {
		p["octal"] = true
	}
	return validator.FailWith("invalid", p)
}

// UnknownKey is for the generated LoadX, not to be called by hand. It returns
// the error of p, which Unknown of a Source returned for section and keys, and
// says which of them it means when it differs in case alone. It does not hold
// the value, which a setting may keep secret.
func UnknownKey(p validator.Path, section string, keys []string) error {
	segments := p.Segments()
	if len(segments) != 2 || segments[0].Name != section {
		return fmt.Errorf("names no section; did you mean %s?", section)
	}
	key := segments[1].Name
	for _, k := range keys {
		if strings.EqualFold(k, key) {
			return fmt.Errorf("is no setting of the section %s; did you mean %s.%s?", section, section, k)
		}
	}
	return fmt.Errorf("is no setting of the section %s", section)
}

// Default is for the generated LoadX, not to be called by hand. It returns err,
// which reading the default of a setting returned, with default=true added to
// its params, so that it tells a default that cannot be read from a setting
// that cannot, without holding the default. errors.Is and errors.As still
// reach err.
func Default(err error) error {
	c, ok := errors.AsType[validator.Coded](err)
	if !ok {
		c = validator.Fail("invalid")
	}
	return defaultError{Coded: c, err: err}
}

type defaultError struct {
	validator.Coded

	err error
}

func (e defaultError) Params() map[string]any {
	p := maps.Clone(e.Coded.Params())
	if p == nil {
		p = make(map[string]any, 1)
	}
	p["default"] = true
	return p
}

func (e defaultError) Error() string {
	return validator.FailWith(e.Code(), e.Params()).Error()
}

func (e defaultError) Unwrap() error {
	return e.err
}
