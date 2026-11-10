// Package toml reads a file of settings written in TOML, for a source.File of
// github.com/go-kanna/config/source:
//
//	source.File{Path: "config/production.toml", Decode: toml.Decode}
//
// It is a module of its own, so that only a program reading TOML requires the
// library that does.
package toml

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
)

// Decode reads a TOML document into its tables, and each value as text: a
// date or a time in the form of RFC 3339, with no time zone it does not name,
// and a number in decimal, a float with its point, 1.0 and not 1.
// Its error says where the
// document is wrong, but not what the library says of it, which may quote a
// secret written there.
func Decode(data []byte) (map[string]any, error) {
	tree := map[string]any{}
	if err := toml.Unmarshal(data, &tree); err != nil {
		if pe, ok := errors.AsType[toml.ParseError](err); ok {
			return nil, fmt.Errorf("line %d, column %d is not valid TOML", pe.Position.Line, pe.Position.Col)
		}
		return nil, errors.New("not valid TOML")
	}
	for k, v := range tree {
		tree[k] = textOf(v)
	}
	return tree, nil
}

func textOf(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, e := range v {
			v[k] = textOf(e)
		}
		return v
	case []map[string]any:
		l := make([]any, len(v))
		for i, e := range v {
			l[i] = textOf(e)
		}
		return l
	case []any:
		for i, e := range v {
			v[i] = textOf(e)
		}
		return v
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		s := strconv.FormatFloat(v, 'g', -1, 64)
		if !strings.ContainsAny(s, ".eIN") {
			s += ".0"
		}
		return s
	case time.Time:
		return dateTime(v)
	}
	return v
}

// dateTime writes t without the time zone of the machine, which the library
// gives a local date, time, or date and time.
func dateTime(t time.Time) string {
	switch t.Location().String() {
	case "date-local":
		return t.Format(time.DateOnly)
	case "time-local":
		return t.Format("15:04:05.999999999")
	case "datetime-local":
		return t.Format("2006-01-02T15:04:05.999999999")
	}
	return t.Format(time.RFC3339Nano)
}
