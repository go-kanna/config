package gen

import (
	"fmt"
	"go/types"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-kanna/internal/gofile"
	"github.com/go-kanna/internal/validation"
)

// emit renders the functions of found into a file of the package pkgName at
// pkgPath. declared are the top-level names the package declares outside the
// file, which nothing the file introduces may take.
func emit(pkgPath, pkgName string, found []target, declared []string) *gofile.File {
	f := gofile.New(generator, pkgPath, pkgName)
	f.Reserve(declared...)

	// gofile wants every top-level name the file declares reserved before it
	// hands out any other name.
	taken := append([]string(nil), declared...)
	planned := make([]validation.Planned, len(found))
	for i, t := range found {
		taken = append(taken, defineName(t), loadName(t))
		planned[i] = validation.Planned{Name: t.Name}
		for _, fd := range t.fields {
			if fd.checks != nil {
				planned[i].Fields = append(planned[i].Fields, validation.Field{Field: fd.Field, Value: fd.checks})
			}
		}
	}
	f.Reserve(taken...)
	w := validation.NewWriter(f, planned, taken)

	e := &emitter{f: f, w: w, src: f.Import(sourcePath, "source"), keys: sectionKeys(found)}
	for _, t := range found {
		e.importAll(t)
	}
	for _, t := range found {
		e.define(t)
		e.load(t)
	}
	return f
}

func defineName(t target) string { return "Define" + t.Name + "Flags" }

func loadName(t target) string { return "Load" + t.Name }

// emitter writes the functions of one file.
type emitter struct {
	f *gofile.File
	w *validation.Writer

	// src is the name the file imports package source under.
	src string

	// keys are the keys of each section, of every struct reading it.
	keys map[string][]string
}

// sectionKeys returns the keys of each section of a file that the structs of
// found read, so that a struct sharing its section with another of the
// package takes the keys of both as known.
func sectionKeys(found []target) map[string][]string {
	keys := map[string][]string{}
	for _, t := range found {
		for _, fd := range t.fields {
			if k := fd.key.file[1]; !slices.Contains(keys[t.section], k) {
				keys[t.section] = append(keys[t.section], k)
			}
		}
	}
	for _, k := range keys {
		slices.Sort(k)
	}
	return keys
}

func (e *emitter) printf(format string, args ...any) {
	fmt.Fprintf(e.f, format, args...)
}

// importAll imports every package the functions of t read its fields with,
// ahead of naming any local, so that a parameter or a local yields its name to
// an import, as in fs2 *flag.FlagSet beside package io/fs.
func (e *emitter) importAll(t target) {
	e.f.Import("flag", "flag")
	l := &loader{emitter: e, t: t}
	for _, fd := range t.fields {
		l.read(fd.read, "", "", "", func(string) string { return "" })
		if fd.def != nil && fd.read.staticDefault() {
			l.defaultExpr(fd)
		}
	}
}

// define writes the function defining the flags of t.
func (e *emitter) define(t target) {
	s := e.f.Scope()
	fs := s.Name("fs")
	e.printf("// %s defines on %s the flags that %s reads %s from.\n", defineName(t), fs, loadName(t), t.Name)
	e.printf("func %s(%s *%s.FlagSet) {\n", defineName(t), fs, e.f.Import("flag", "flag"))
	for _, fd := range t.fields {
		def := ""
		if fd.def != nil {
			def = fd.def.text
		}
		usage := fmt.Sprintf("%s.%s (%s)", t.Name, fd.Name, fd.key.env)
		e.printf("%s.DefineFlag(%s, %q, %q, %q, %t)\n", e.src, fs, fd.key.flag, usage, def, fd.read.kind == readBool)
	}
	e.printf("}\n\n")
}

// load writes the function reading t.
func (e *emitter) load(t target) {
	s := e.f.Scope()
	l := &loader{
		emitter: e, t: t,
		in: s.Name("src"), x: s.Name("x"), errs: s.Name("errs"),
		locals: make(map[string]string), names: make(map[string]string),
	}
	for _, base := range []string{"v", "ok", "err", "b", "n", "d", "e", "u", "o", "hook", "fe", "p", "rest", "keys"} {
		l.locals[base] = s.Name(base)
	}
	for _, fd := range t.fields {
		if l.needsName(fd) {
			l.names[fd.Name] = s.Name("name" + fd.Name)
		}
	}
	rt := e.w.Runtime()

	e.printf("// %s reads %s from %s, and checks it by its validate tags.\n", loadName(t), t.Name, l.in)
	e.printf("// It returns the %s.Errors of each setting it cannot read and of each\n", rt)
	e.printf("// key in its section of a file that no field reads, or else of each rule\n")
	e.printf("// the settings break, under the name each goes by where it is set.\n")
	e.printf("func %s(%s %s.Source) (%s, error) {\n", loadName(t), l.in, e.src, t.Name)
	e.printf("var (\n%s %s\n%s %s.Errors\n)\n", l.x, t.Name, l.errs, rt)
	for _, fd := range t.fields {
		e.printf("\n")
		l.field(fd)
	}
	l.unknown()
	e.printf("\nif len(%s) > 0 {\nreturn %s{}, %s\n}\n", l.errs, t.Name, l.errs)
	if t.hook == nil && !slices.ContainsFunc(t.fields, func(fd field) bool { return fd.checks != nil }) {
		e.printf("return %s, nil\n}\n\n", l.x)
		return
	}

	body := e.w.Body(s, l.x, l.errs)
	for _, fd := range t.fields {
		if fd.checks != nil {
			e.printf("\n")
			body.Value(fd.checks, l.x+"."+fd.Name, l.names[fd.Name], l.names[fd.Name])
		}
	}
	if t.hook != nil {
		l.hook()
	}
	e.printf("\nif len(%s) > 0 {\nreturn %s{}, %s\n}\nreturn %s, nil\n}\n\n", l.errs, t.Name, l.errs, l.x)
}

// loader writes the body of one function reading a struct.
type loader struct {
	*emitter

	t target

	// in, x, and errs are the parameter, the struct read, and the errors.
	in, x, errs string

	// locals are the names of the other locals by what they would be called,
	// and names the variable holding the name of each field's setting.
	locals, names map[string]string

	// def is set while writing the reading of a default, whose errors are
	// marked as the default's.
	def bool
}

// fail returns err, an error of reading a setting, marked as the default's
// while a default is read.
func (l *loader) fail(err string) string {
	if l.def {
		return fmt.Sprintf("%s.Default(%s)", l.src, err)
	}
	return err
}

// unknown writes the reporting of what the section of the struct holds besides
// the keys of the structs reading it.
func (l *loader) unknown() {
	keys := make([]string, len(l.keys[l.t.section]))
	for i, k := range l.keys[l.t.section] {
		keys[i] = strconv.Quote(k)
	}
	v, name := l.locals["v"], l.locals["keys"]
	l.printf("\n%s := []string{%s}\n", name, strings.Join(keys, ", "))
	l.printf("for _, %s := range %s.Unknown(%q, %s) {\n%s.Add(%s.Name, %s.UnknownKey(%s.Name, %q, %s))\n}\n",
		v, l.in, l.t.section, name, l.errs, v, l.src, v, l.t.section, name)
}

// needsName reports whether the name of fd's setting is needed beyond where it
// is found: for an error of reading it, of its rules, or of the hook.
func (l *loader) needsName(fd field) bool {
	return fd.read.kind != readText || fd.checks != nil || l.t.hook != nil || fd.def != nil && !fd.read.staticDefault()
}

// field writes the reading of fd: its default, then its setting, where it is.
func (l *loader) field(fd field) {
	v, ok, err := l.locals["v"], l.locals["ok"], l.locals["err"]
	name := l.names[fd.Name]
	if fd.def != nil && fd.read.staticDefault() {
		l.printf("%s.%s = %s\n", l.x, fd.Name, l.defaultExpr(fd))
	}
	if name != "" {
		l.printf("%s := %q\n", name, fd.key.env)
	}
	l.printf("if %s, %s, %s := %s.Lookup(%s); %s != nil {\n%s.Add(%s.Name, %s)\n} else if %s {\n",
		v, ok, err, l.in, l.keyLit(fd.key), err, l.errs, v, err, ok)
	if name != "" {
		l.printf("%s = %s.Name\n", name, v)
	}
	l.printf("%s", l.read(fd.read, v+".Text", name, typeString(fd.Type, l.t.Struct), l.assign(fd)))
	if fd.def != nil && !fd.read.staticDefault() {
		l.def = true
		def := l.read(fd.read, strconv.Quote(fd.def.text), name, typeString(fd.Type, l.t.Struct), l.assign(fd))
		l.def = false
		l.printf("} else {\n%s", def)
	}
	l.printf("}\n")
}

// assign returns what stores a value read into fd.
func (l *loader) assign(fd field) func(string) string {
	return func(value string) string { return fmt.Sprintf("%s.%s = %s\n", l.x, fd.Name, value) }
}

func (l *loader) keyLit(k key) string {
	file := make([]string, len(k.file))
	for i, p := range k.file {
		file[i] = strconv.Quote(p)
	}
	return fmt.Sprintf("%s.Key{Env: %q, File: []string{%s}, Flag: %q}", l.src, k.env, strings.Join(file, ", "), k.flag)
}

// read returns what reads text, a Go expression of type string, as r says,
// recording what cannot be read under name, which reports it as typeName, and
// handing the value to assign.
func (l *loader) read(r reader, text, name, typeName string, assign func(string) string) string {
	invalid := func() string {
		return fmt.Sprintf("%s.Add(%s, %s)\n", l.errs, name,
			l.fail(fmt.Sprintf("%s.FailWith(\"invalid\", map[string]any{\"type\": %q})", l.w.Runtime(), typeName)))
	}
	parse := func(local, call string, convert bool) string {
		value := local
		if convert {
			value = l.convert(r.typ, local)
		}
		next := assign(value)
		if strings.HasPrefix(next, "if ") {
			// A constructor to pass the value to goes on as else if.
			return fmt.Sprintf("if %s, %s := %s; %s != nil {\n%s} else %s",
				local, l.locals["err"], call, l.locals["err"], invalid(), next)
		}
		return fmt.Sprintf("if %s, %s := %s; %s != nil {\n%s} else {\n%s}\n",
			local, l.locals["err"], call, l.locals["err"], invalid(), next)
	}
	strconvPkg := func() string { return l.f.Import("strconv", "strconv") }

	switch r.kind {
	case readText:
		if types.Identical(types.Unalias(r.typ), types.Typ[types.String]) {
			return assign(text)
		}
		return assign(l.convert(r.typ, text))
	case readBool:
		return parse(l.locals["b"], fmt.Sprintf("%s.ParseBool(%s)", strconvPkg(), text), !l.is(r.typ, types.Bool))
	case readInt:
		call := fmt.Sprintf("%s.ParseInt(%s, 0, %d)", strconvPkg(), text, r.bits)
		return parse(l.locals["n"], call, !l.is(r.typ, types.Int64))
	case readUint:
		call := fmt.Sprintf("%s.ParseUint(%s, 0, %d)", strconvPkg(), text, r.bits)
		return parse(l.locals["n"], call, !l.is(r.typ, types.Uint64))
	case readFloat:
		call := fmt.Sprintf("%s.ParseFloat(%s, %d)", strconvPkg(), text, r.bits)
		return parse(l.locals["n"], call, !l.is(r.typ, types.Float64))
	case readDuration:
		return parse(l.locals["d"], fmt.Sprintf("%s.ParseDuration(%s)", l.f.Import("time", "time"), text), false)
	case readEnum:
		return l.construct(l.locals["e"], r.ctor, text, name, typeName, assign)
	case readValue:
		return l.read(*r.held, text, name, typeName, func(held string) string {
			return l.construct(l.locals["o"], r.ctor, held, name, typeName, assign)
		})
	case readUnmarshal:
		u, err := l.locals["u"], l.locals["err"]
		return fmt.Sprintf("var %s %s\n", u, types.TypeString(r.typ, l.f.Qualifier)) +
			fmt.Sprintf("if %s := %s.UnmarshalText([]byte(%s)); %s != nil {\n%s.Add(%s, %s)\n} else {\n%s}\n",
				err, u, text, err, l.errs, name, l.coded(err, typeName), assign(u))
	}
	panic(fmt.Sprintf("emit: unknown reader %d", r.kind))
}

// construct returns what passes value to ctor, the constructor of an enum or a
// value type, recording its error under name.
func (l *loader) construct(
	local string, ctor *types.Func, value, name, typeName string, assign func(string) string,
) string {
	err := l.locals["err"]
	fn := ctor.Name()
	if q := l.f.Qualifier(ctor.Pkg()); q != "" {
		fn = q + "." + fn
	}
	return fmt.Sprintf("if %s, %s := %s(%s); %s != nil {\n%s.Add(%s, %s)\n} else {\n%s}\n",
		local, err, fn, value, err, l.errs, name, l.coded(err, typeName), assign(local))
}

// coded returns what reports err, returned for a value of type typeName.
func (l *loader) coded(err, typeName string) string {
	return l.fail(fmt.Sprintf("%s.Coded(%s, %q)", l.src, err, typeName))
}

// is reports whether t is the predeclared type kind itself, which a value
// read as one is assigned to without converting it.
func (l *loader) is(t types.Type, kind types.BasicKind) bool {
	return types.Identical(types.Unalias(t), types.Typ[kind])
}

func (l *loader) convert(t types.Type, value string) string {
	return types.TypeString(t, l.f.Qualifier) + "(" + value + ")"
}

// defaultExpr returns the default of fd as a Go value.
func (l *loader) defaultExpr(fd field) string {
	switch {
	case fd.def.enum != nil:
		c := fd.def.enum
		if q := l.f.Qualifier(c.Pkg()); q != "" {
			return q + "." + c.Name()
		}
		return c.Name()
	case fd.read.kind == readDuration:
		return durationExpr(fd.def.dur, l.f.Import("time", "time"))
	case fd.def.mathCall != "":
		v := l.f.Import("math", "math") + "." + fd.def.mathCall
		if l.is(fd.read.typ, types.Float64) {
			return v
		}
		return l.convert(fd.read.typ, v)
	}
	return fd.def.lit
}

// durationExpr writes d as a Go expression in the largest unit it is a whole
// number of, such as 5 * time.Second.
func durationExpr(d time.Duration, timePkg string) string {
	if d == 0 {
		return "0"
	}
	for _, u := range []struct {
		d    time.Duration
		name string
	}{
		{time.Hour, "Hour"}, {time.Minute, "Minute"}, {time.Second, "Second"},
		{time.Millisecond, "Millisecond"}, {time.Microsecond, "Microsecond"},
	} {
		if d%u.d == 0 {
			if d/u.d == 1 {
				return timePkg + "." + u.name
			}
			return fmt.Sprintf("%d * %s.%s", d/u.d, timePkg, u.name)
		}
	}
	return strconv.FormatInt(int64(d), 10)
}

// hook writes the call of the hook of the struct, and moves what it reports
// under the name of each field's setting: the hook names a field as
// kanna-validator does, by its JSON name.
func (l *loader) hook() {
	hook, fe, p, rest, ok := l.locals["hook"], l.locals["fe"], l.locals["p"], l.locals["rest"], l.locals["ok"]
	rt := l.w.Runtime()
	l.printf("\nvar %s %s.Errors\n%s.validate(&%s)\n", hook, rt, l.x, hook)
	l.printf("for _, %s := range %s {\nfor _, %s := range []struct{ path, name string }{\n", fe, hook, p)
	for _, fd := range longestFirst(l.t.fields) {
		l.printf("{%q, %s},\n", fd.path, l.names[fd.Name])
	}
	l.printf("} {\n")
	l.printf("if %s, %s := %s.CutPrefix(%s.Field, %s.path); %s && (%s == \"\" || %s[0] == '.' || %s[0] == '[') {\n",
		rest, ok, l.f.Import("strings", "strings"), fe, p, ok, rest, rest, rest)
	l.printf("%s.Field = %s.name + %s\nbreak\n}\n}\n", fe, p, rest)
	l.printf("%s = append(%s, %s)\n}\n", l.errs, l.errs, fe)
}

// longestFirst returns fields in their order, but each ahead of those whose
// path its own begins with, such as json:"a.b" ahead of json:"a": the hook's
// errors go under the first path they begin with.
func longestFirst(fields []field) []field {
	var ordered []field
	for _, fd := range fields {
		i := slices.IndexFunc(ordered, func(o field) bool { return under(fd.path, o.path) })
		if i < 0 {
			ordered = append(ordered, fd)
		} else {
			ordered = slices.Insert(ordered, i, fd)
		}
	}
	return ordered
}

// under reports whether path names something under parent, as the hook's
// error paths do: parent followed by a field or an index.
func under(path, parent string) bool {
	rest, ok := strings.CutPrefix(path, parent)
	return ok && rest != "" && (rest[0] == '.' || rest[0] == '[')
}
