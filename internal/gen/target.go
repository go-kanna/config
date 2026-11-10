package gen

import (
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/enumtype"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/suggest"
	"github.com/go-kanna/internal/validation"
)

const (
	// directiveKey spells the directive //kanna:config.
	directiveKey = "config"

	// outputFile is the file generated in each package.
	outputFile = "config_gen.go"

	// generator names the tool in the header of the file it writes.
	generator = "kanna-config"

	// sourcePath is the package the generated code finds settings with.
	sourcePath = "github.com/go-kanna/config/source"
)

// The tags of caarlos0/env that kanna-config does not read, and what to write
// instead, so that moving from it says what each meant rather than ignoring it.
var foreignTags = map[string]string{
	"envDefault": `write the default as default:"…"`,
	"envPrefix": "kanna-config reads no nested struct; mark it //kanna:config of its own, " +
		"its prefix with prefix=",
	"envSeparator":       "kanna-config reads no slice; give the field a type of its own with UnmarshalText",
	"envKeyValSeparator": "kanna-config reads no map; give the field a type of its own with UnmarshalText",
}

// The options of the env tag of caarlos0/env, none of which kanna-config takes.
var envOptions = map[string]string{
	"required": `kanna-config does not tell whether a variable is set; validate:"required" asks for a value ` +
		`that is not zero, which 0 and false are not`,
	"notEmpty": `validate:"required" comes near, but rejects 0 and false as well`,
	"file":     `read the file the variable names after LoadX, into a field marked config:"-"`,
	"expand":   "call os.ExpandEnv on the field after LoadX",
	"unset":    "call os.Unsetenv after LoadX",
	"init":     "kanna-config reads no pointer, so there is nothing to make",
}

// target is a struct marked //kanna:config.
type target struct {
	scan.Struct

	// fields are the fields read, in declaration order.
	fields []field

	// hook is the validate(*validator.Errors) method the struct declares, which
	// the generated code calls once the settings are read, or nil.
	hook *types.Func
}

// field is a field read from the settings.
type field struct {
	scan.Field

	key  key
	read reader

	// def is the default, when the field has one.
	def *defaultValue

	// checks are what its validate tags check, or nil, and path is the name
	// a hook puts its errors under.
	checks *validation.Value
	path   string
}

// defaultValue is the default of a field, as its tag writes it, and when it
// is read on generating, as a Go value.
type defaultValue struct {
	text string

	// value is the default read, for a reader whose default is static; lit is
	// it as a Go literal, dur it as a duration, and enum the constant holding
	// it. A NaN or an infinity, which no constant holds, has no value, and
	// mathCall calls the function of package math returning it instead.
	value    constant.Value
	lit      string
	dur      time.Duration
	enum     *types.Const
	mathCall string
}

// targets returns the structs marked //kanna:config in pkgs, and says what is
// wrong with the rest. pkgs are loaded with the types of their expressions,
// which the paths of a hook are checked by.
func targets(fset *token.FileSet, pkgs []*packages.Package) ([]target, []diag.Diag) {
	planner := validation.NewPlanner(fset, nil, nil)
	var (
		found []target
		diags []diag.Diag
	)
	for _, s := range scan.Structs(pkgs) {
		d, ds := directive.Find(fset, s.Doc, directiveKey)
		diags = append(diags, ds...)
		if !d.Found {
			continue
		}
		t, ds := plan(fset, pkgs, planner, s, d)
		diags = append(diags, ds...)
		if !diag.HasErrors(ds) {
			found = append(found, t)
		}
	}
	return found, diags
}

// plan works out how each field of s, marked by d, is read and checked.
func plan(
	fset *token.FileSet, pkgs []*packages.Package, planner validation.Planner, s scan.Struct, d directive.Directive,
) (target, []diag.Diag) {
	t := target{Struct: s}
	args, diags := d.Parse(directive.Spec{Keys: []string{"prefix"}})
	prefix := snakeCase(s.Name)
	for _, a := range args {
		if !snakeWords.MatchString(a.Value) {
			diags = append(diags, diag.Errorf(d.Pos, "prefix=%s is not words in snake case", a.Value).
				WithHints("write it in lower case, its words joined by _, as prefix="+snakeCase(a.Value)))
			continue
		}
		prefix = a.Value
	}
	if s.Named.TypeParams().Len() > 0 {
		return t, append(diags, diag.Errorf(s.Pos, "generic struct %s is not supported", s.Name))
	}

	checked, ds := planner.Struct(s)
	diags = append(diags, ds...)
	checks := make(map[string]*validation.Value, len(checked))
	for _, f := range checked {
		checks[f.Name] = f.Value
	}
	for _, f := range s.Fields {
		fd, ds := fieldOf(fset, s, prefix, f)
		diags = append(diags, ds...)
		if fd == nil {
			if f.Tag.Get("validate") != "" && !hasValidate(fset, s) {
				diags = append(diags, fieldError(f, `its validate tag is never checked: kanna-config does not read the field, `+
					`and %s has no //kanna:validate`, s.Name).WithHints("mark "+s.Name+" //kanna:validate, or remove the tag"))
			}
			continue
		}
		fd.checks = checks[f.Name]
		fd.path, _ = validation.JSONName(f)
		if fd.def != nil && fd.def.value != nil {
			if broken, ok := validation.Rejects(fd.checks, fd.def.value); ok {
				diags = append(diags, fieldError(f, "the default %q breaks a rule of its validate tag: %s", fd.def.text, broken))
			}
		}
		t.fields = append(t.fields, *fd)
	}
	diags = append(diags, collisions(t.fields)...)

	hook, ds := validation.Hook(fset, s)
	diags = append(diags, ds...)
	if hook != nil {
		t.hook = hook
		diags = append(diags, validation.HookPaths(fset, pkgs, s, hook)...)
	}
	return t, diags
}

// hasValidate reports whether s is marked //kanna:validate, which checks the
// fields kanna-config does not read when the program calls Validate.
func hasValidate(fset *token.FileSet, s scan.Struct) bool {
	d, _ := directive.Find(fset, s.Doc, validation.DirectiveKey)
	return d.Found
}

// fieldOf works out how f of s, under prefix, is read, or returns nil for a
// field it does not read: one tagged config:"-", or one in error.
func fieldOf(fset *token.FileSet, s scan.Struct, prefix string, f scan.Field) (*field, []diag.Diag) {
	var diags []diag.Diag
	for tag, hint := range foreignTags {
		if _, ok := f.Tag.Lookup(tag); ok {
			diags = append(diags, fieldError(f, "kanna-config does not read the tag %s of caarlos0/env", tag).WithHints(hint))
		}
	}
	slices.SortFunc(diags, func(a, b diag.Diag) int { return strings.Compare(a.Message, b.Message) })

	name, skip := f.Tag.Lookup("config")
	if skip = skip && name == "-"; skip {
		if _, ok := f.Tag.Lookup("default"); ok {
			diags = append(diags, fieldError(f, `a field tagged config:"-" is not read, so its default is never used`))
		}
		return nil, diags
	}
	switch {
	case f.Name == "_", !f.Exported, f.Embedded:
		what := "an unexported field"
		if f.Name == "_" {
			what = "a field named _"
		} else if f.Embedded {
			what = "an embedded field"
		}
		return nil, append(diags, fieldError(f, "%s cannot be read from the settings", what).
			WithHints(`tag it config:"-" to leave it out, or give it a name of its own`))
	}

	words := snakeCase(f.Name)
	if name != "" {
		if !snakeWords.MatchString(name) {
			return nil, append(diags, fieldError(f, `config:"%s" is not words in snake case`, name).
				WithHints(`write it in lower case, its words joined by _, as config:"`+snakeCase(name)+`"`))
		}
		words = name
	}
	env, ds := envName(f)
	if diags = append(diags, ds...); len(ds) > 0 {
		return nil, diags
	}

	read, ok := readerOf(f.Type)
	if !ok {
		return nil, append(diags, fieldError(f, "kanna-config cannot read %s", typeString(f.Type, s)).WithHints(
			"read a string, a bool, a number, time.Duration, a value type or an enum of kanna-value, "+
				"or a type with UnmarshalText; or tag it config:\"-\""))
	}
	fd := &field{Field: f, key: keyOf(prefix, words, env), read: read}
	if text, ok := f.Tag.Lookup("default"); ok {
		def, err := defaultOf(fset, read, text, s)
		if err != "" {
			return nil, append(diags, fieldError(f, "%s", err))
		}
		fd.def = def
	}
	return fd, diags
}

// envName reads the env tag of f: the variable as written, for one the
// environment names, such as PORT. caarlos0/env writes options after a comma,
// none of which kanna-config takes.
func envName(f scan.Field) (string, []diag.Diag) {
	tag, ok := f.Tag.Lookup("env")
	if !ok {
		return "", nil
	}
	name, opts, hasOpts := strings.Cut(tag, ",")
	switch {
	case name == "-":
		return "", []diag.Diag{fieldError(f, `env:"-" names no variable`).
			WithHints(`tag it config:"-" to leave it out of every source`)}
	case hasOpts:
		var diags []diag.Diag
		for opt := range strings.SplitSeq(opts, ",") {
			hint, known := envOptions[opt]
			if !known {
				hint = "kanna-config takes the name of the variable alone"
			}
			diags = append(diags, fieldError(f, "kanna-config takes no option %q in the env tag", opt).WithHints(hint))
		}
		return "", diags
	case name == "", strings.ContainsAny(name, "=\x00"):
		return "", []diag.Diag{fieldError(f,
			"env:%q cannot name a variable, which is neither empty nor holds = or NUL", name)}
	}
	return name, nil
}

// defaultOf reads text, the default of a field read by r, or says why it
// cannot be read. A default r reads when the program runs is kept as text.
func defaultOf(fset *token.FileSet, r reader, text string, s scan.Struct) (*defaultValue, string) {
	def := &defaultValue{text: text}
	if !r.staticDefault() {
		return def, ""
	}
	bad := func() (*defaultValue, string) {
		return nil, "the default " + strconv.Quote(text) + " is not " + article(typeString(r.typ, s))
	}
	switch r.kind {
	case readText:
		def.value, def.lit = constant.MakeString(text), strconv.Quote(text)
	case readBool:
		b, err := strconv.ParseBool(text)
		if err != nil {
			return bad()
		}
		def.value, def.lit = constant.MakeBool(b), strconv.FormatBool(b)
	case readInt:
		n, err := strconv.ParseInt(text, 0, bitsOr(r.bits))
		if err != nil {
			return bad()
		}
		def.value, def.lit = constant.MakeInt64(n), text
	case readUint:
		n, err := strconv.ParseUint(text, 0, bitsOr(r.bits))
		if err != nil {
			return bad()
		}
		def.value, def.lit = constant.MakeUint64(n), text
	case readFloat:
		n, err := strconv.ParseFloat(text, r.bits)
		if err != nil {
			return bad()
		}
		switch {
		case math.IsNaN(n):
			def.mathCall = "NaN()"
		case math.IsInf(n, 1):
			def.mathCall = "Inf(1)"
		case math.IsInf(n, -1):
			def.mathCall = "Inf(-1)"
		default:
			def.value, def.lit = constant.MakeFloat64(n), strconv.FormatFloat(n, 'g', -1, r.bits)
		}
	case readDuration:
		d, err := time.ParseDuration(text)
		if err != nil {
			return bad()
		}
		def.value, def.dur = constant.MakeInt64(int64(d)), d
	case readEnum:
		named, _ := types.Unalias(r.typ).(*types.Named)
		var values []string
		for _, c := range enumtype.Constants(named, fset) {
			v := constant.StringVal(c.Val())
			if v == text {
				def.value, def.enum = constant.MakeString(text), c
				return def, ""
			}
			values = append(values, v)
		}
		msg := "the default " + strconv.Quote(text) + " is none of the constants of " + typeString(r.typ, s)
		if c, ok := suggest.Closest(text, values); ok {
			msg += "; did you mean " + strconv.Quote(c) + "?"
		}
		return nil, msg
	case readValue, readUnmarshal:
	}
	return def, ""
}

// bitsOr returns bits, or 64 for an int or a uint, whose size strconv cannot
// tell: a default that fits 64 bits fits each platform the generated code
// assigns it on only if its literal does, which the compiler checks.
func bitsOr(bits int) int {
	if bits == 0 {
		return 64
	}
	return bits
}

// collisions reports two fields of a struct read from one setting, and a field
// whose variable is the one naming the file of another's, NAME_FILE.
func collisions(fields []field) []diag.Diag {
	var diags []diag.Diag
	seen := map[string]string{}
	vars := map[string]string{}
	for _, f := range fields {
		for _, name := range []string{
			"variable " + f.key.env, "key " + strings.Join(f.key.file, "."), "flag -" + f.key.flag,
		} {
			if other, ok := seen[name]; ok {
				diags = append(diags, fieldError(f.Field, "%s reads the %s that %s reads too", f.Name, name, other))
				continue
			}
			seen[name] = f.Name
		}
		vars[f.key.env] = f.Name
	}
	for _, f := range fields {
		base, ok := strings.CutSuffix(f.key.env, "_FILE")
		if other, found := vars[base]; ok && found {
			diags = append(diags, fieldError(f.Field,
				"%s reads the variable %s, which names the file of the variable %s that %s reads",
				f.Name, f.key.env, base, other).
				WithHints(`give one of them another name, with config:"…" or env:"…"`))
		}
	}
	return diags
}

// fieldError reports a problem of f, naming the field after it, as in
// `kanna-config cannot read []string (field Hosts)`.
func fieldError(f scan.Field, format string, args ...any) diag.Diag {
	args = append(args, f.Name)
	return diag.Errorf(f.Pos, format+" (field %s)", args...)
}

// typeString spells t as the file declaring s does: a type of another package
// by the package's name, such as slog.Level.
func typeString(t types.Type, s scan.Struct) string {
	own := s.Named.Obj().Pkg()
	return types.TypeString(t, func(p *types.Package) string {
		if p == own {
			return ""
		}
		return p.Name()
	})
}

// article puts a or an before what, which names a type.
func article(what string) string {
	if strings.ContainsRune("aeiou", rune(what[0])) {
		return "an " + what
	}
	return "a " + what
}
