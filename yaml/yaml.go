// Package yaml reads a file of settings written in YAML, for a source.File of
// github.com/go-kanna/config/source:
//
//	source.File{Path: "config/production.yaml", Decode: yaml.Decode}
//
// It is a module of its own, so that only a program reading YAML requires the
// library that does.
package yaml

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"regexp"

	"go.yaml.in/yaml/v3"
)

// lineOf finds the line an error of the library starts by naming, before what
// it says of the line.
var lineOf = regexp.MustCompile(`^(?:yaml: )?line (\d+):`)

// Decode reads a YAML mapping into its sections, and each value as the text it
// is written as: 0123 is 0123, not 83. An empty document has none,
// and a second document is an error, which would otherwise go unread. Its
// error says where the document is wrong, but not what the library says of
// it, which may quote a secret written there.
func Decode(data []byte) (map[string]any, error) {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	var doc yaml.Node
	if err := dec.Decode(&doc); err != nil {
		if errors.Is(err, io.EOF) {
			return map[string]any{}, nil
		}
		return nil, invalid(err)
	}
	var next yaml.Node
	switch err := dec.Decode(&next); {
	case err == nil:
		return nil, fmt.Errorf("line %d, column %d starts a second YAML document, and a file holds one",
			next.Line, next.Column)
	case !errors.Is(err, io.EOF):
		return nil, invalid(err)
	}
	if len(doc.Content) == 0 {
		return map[string]any{}, nil
	}
	// The values must decode as the library types them, as !!int s3cr3t does not.
	var typed map[string]any
	if err := doc.Decode(&typed); err != nil {
		if root := doc.Content[0]; root.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("line %d, column %d is not a YAML mapping", root.Line, root.Column)
		}
		if n := undecodable(&doc); n != nil {
			return nil, fmt.Errorf("line %d, column %d is not valid YAML", n.Line, n.Column)
		}
		return nil, invalid(err)
	}
	var texts map[string]text
	if err := doc.Decode(&texts); err != nil {
		return nil, invalid(err)
	}
	return sections(texts), nil
}

// text is a value as it is written: a scalar's text, or a mapping or a
// sequence of them. A null is nil.
type text struct{ v any }

func (t *text) UnmarshalYAML(n *yaml.Node) error {
	switch n.Kind {
	case yaml.MappingNode:
		var m map[string]text
		if err := n.Decode(&m); err != nil {
			return err //nolint:wrapcheck // invalid reports it by the line it names
		}
		t.v = sections(m)
	case yaml.SequenceNode:
		var s []text
		if err := n.Decode(&s); err != nil {
			return err //nolint:wrapcheck // invalid reports it by the line it names
		}
		l := make([]any, len(s))
		for i, e := range s {
			l[i] = e.v
		}
		t.v = l
	case yaml.ScalarNode:
		t.v = n.Value
	case yaml.DocumentNode, yaml.AliasNode:
		// Decoding resolves these before it reaches a value.
	}
	return nil
}

func sections(m map[string]text) map[string]any {
	tree := make(map[string]any, len(m))
	for k, t := range m {
		tree[k] = t.v
	}
	return tree
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
