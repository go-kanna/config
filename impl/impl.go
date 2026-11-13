// Package impl is what the code kanna-config generates runs on: the flags it defines and the errors it reports for a
// setting. Only generated code imports it. Its API changes with the generator, which ships in the same module.
package impl

import (
	"errors"
	"flag"
	"fmt"
	"maps"
	"strings"

	"github.com/go-kanna/validator"

	"github.com/go-kanna/config/internal/octal"
)

// DefineFlag defines on fs the flag name, whose text a Source opened with fs
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

// Coded returns err,
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

// UnknownKey returns
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

// Default returns err,
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
