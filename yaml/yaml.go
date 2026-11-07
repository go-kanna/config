// Package yaml reads a file of settings written in YAML, for a source.File of
// github.com/go-kanna/config/source:
//
//	source.File{Path: "config/production.yaml", Decode: yaml.Decode}
//
// It is a module of its own, so that only a program reading YAML requires the
// library that does.
package yaml

import (
	"errors"
	"fmt"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// lineOf finds the line an error of the library starts by naming, before what
// it says of the line.
var lineOf = regexp.MustCompile(`^(?:yaml: )?line (\d+):`)

// Decode reads a YAML mapping into its sections. An empty document has none.
// Its error says where the document is wrong, but not what the library says
// of it, which may quote a secret written there.
func Decode(data []byte) (map[string]any, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, invalid(err)
	}
	if len(doc.Content) == 0 {
		return map[string]any{}, nil
	}
	var tree map[string]any
	if err := doc.Decode(&tree); err != nil {
		if root := doc.Content[0]; root.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d, column %d is not a YAML mapping", root.Line, root.Column)
		}
		if n := undecodable(&doc); n != nil {
			return nil, fmt.Errorf("line %d, column %d is not valid YAML", n.Line, n.Column)
		}
		return nil, invalid(err)
	}
	if tree == nil {
		return map[string]any{}, nil
	}
	return tree, nil
}

// undecodable returns the first value under n that does not decode, such as
// a string tagged !!int.
func undecodable(n *yaml.Node) *yaml.Node {
	for _, c := range n.Content {
		if u := undecodable(c); u != nil {
			return u
		}
	}
	var v any
	if n.Kind == yaml.ScalarNode && n.Decode(&v) != nil {
		return n
	}
	return nil
}

// invalid reports err by the line it names, if any.
func invalid(err error) error {
	msg := err.Error()
	if te, ok := errors.AsType[*yaml.TypeError](err); ok && len(te.Errors) > 0 {
		msg = te.Errors[0]
	}
	if m := lineOf.FindStringSubmatch(msg); m != nil {
		return fmt.Errorf("line %s is not valid YAML", m[1])
	}
	return errors.New("not valid YAML")
}
