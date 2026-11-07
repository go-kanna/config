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

	"github.com/BurntSushi/toml"
)

// Decode reads a TOML document into its tables. Its error says where the
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
	return tree, nil
}
