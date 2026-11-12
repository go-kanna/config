package gen

import (
	"path/filepath"

	"github.com/go-kanna/internal/deps"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/scan"
	"github.com/go-kanna/internal/validation"
)

// reportDeps tells what a run in dir reads and writes. A qualified rule= names a
// package the struct's file imports, by its import path.
//
// It reads the directives and tags alone, as a struct whose field types do not
// resolve yet, before another generator writes them, still has its file and
// functions written.
func reportDeps(dir string, cfg config, patterns []string) (deps.Report, error) {
	set, err := packages.Load(patterns, packages.Config{Dir: dir, Tags: cfg.tags, Regenerated: []string{outputFile}})
	if err != nil {
		return deps.Report{}, err //nolint:wrapcheck // it says what could not be loaded
	}

	r := deps.Report{Packages: set.Paths()}
	for _, s := range scan.Structs(set.Packages) {
		if d, _ := directive.Find(set.Fset, s.Doc, directiveKey); !d.Found {
			continue
		}
		t := target{Struct: s}
		r.Writes = append(r.Writes, filepath.Join(filepath.Dir(s.Pos.Filename), outputFile))
		r.Declares = append(r.Declares, s.PkgPath+"."+loadName(t), s.PkgPath+"."+defineName(t))
		r.Refs = append(r.Refs, validation.RuleRefs(s)...)
	}
	return r, nil
}
