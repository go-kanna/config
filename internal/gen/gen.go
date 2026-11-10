// Package gen is kanna-config: it generates, for each struct marked
// //kanna:config, the function reading it from flags, the environment, and
// files, and checking it by its validate tags, and the function defining its
// flags.
package gen

import (
	"flag"
	"go/token"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/go-kanna/internal/cli"
	"github.com/go-kanna/internal/diag"
	"github.com/go-kanna/internal/directive"
	"github.com/go-kanna/internal/output"
	"github.com/go-kanna/internal/packages"
)

// config holds what kanna-config's own flags set.
type config struct {
	tags string
}

// Command returns kanna-config.
func Command() cli.Command[config] {
	return cli.Command[config]{
		Name:    generator,
		Summary: "generate the functions reading the settings of structs marked //kanna:" + directiveKey,
		Args:    "[packages]",
		Help: "The packages default to the one in the current directory, so that\n" +
			"//go:generate go tool " + generator + " needs no argument.",
		Flags: func(fs *flag.FlagSet, cfg *config) {
			cli.Tags(fs, &cfg.tags)
		},
		Run:  run,
		Deps: reportDeps,
	}
}

func run(dir string, cfg config, patterns []string) ([]output.Change, []diag.Diag) {
	set, deferred, stop := load(dir, cfg.tags, patterns)
	if stop != nil {
		return nil, stop
	}
	roots := make(map[string]bool, len(set.Packages))
	for _, pkg := range set.Packages {
		roots[pkg.PkgPath] = true
	}

	changes, diags := generate(set, roots)
	if diag.HasErrors(diags) {
		return nil, stopped(set, roots, deferred, diags)
	}
	if rds := packages.Recheck(set, changes); len(rds) > 0 {
		return nil, slices.Concat(diags, rds, set.SetAsideWarnings())
	}
	if !slices.ContainsFunc(changes, func(c output.Change) bool { return !c.Remove }) {
		in := slices.Sorted(maps.Keys(roots))
		if len(in) == 0 {
			in = patterns // which matched no package
		}
		diags = append(diags, diag.Warningf(token.Position{},
			"no struct is marked //kanna:%s in %s", directiveKey, strings.Join(in, ", ")))
	}
	return changes, diags
}

// load loads the packages matching patterns, and says why the run stops when
// it cannot go on with them. Code calling a function the run writes does not
// compile before the run writes it, so a type error of a package loaded does
// not stop the run: it is deferred, for packages.Recheck to tell, once the
// output is in place, whether it remains.
func load(dir, tags string, patterns []string) (set packages.Set, deferred, stop []diag.Diag) {
	cfg := packages.Config{Dir: dir, Tags: tags, TypesInfo: true, Regenerated: []string{outputFile}}
	set, err := packages.Load(patterns, cfg)
	if err != nil {
		return set, nil, []diag.Diag{diag.Errorf(token.Position{}, "%v", err)}
	}
	blocking, deferred := set.Split()
	if diag.HasErrors(blocking) {
		return set, nil, append(blocking, set.SetAsideWarnings()...)
	}
	return set, deferred, nil
}

// stopped returns what a run that cannot generate reports, leaving out the
// type errors that say only that a function it writes is missing.
func stopped(set packages.Set, roots map[string]bool, deferred, diags []diag.Diag) []diag.Diag {
	found, _ := targets(set.Fset, rootPackages(set, roots))
	written := make(map[string]bool, 2*len(found))
	for _, t := range found {
		written[t.PkgPath+"."+loadName(t)] = true
		written[t.PkgPath+"."+defineName(t)] = true
	}
	return packages.Stopped(deferred, diags, set.Pending(written))
}

func rootPackages(set packages.Set, roots map[string]bool) []*packages.Package {
	return slices.DeleteFunc(slices.Clone(set.Packages), func(p *packages.Package) bool { return !roots[p.PkgPath] })
}

// generate works out the changes for the packages of set that roots names: the
// output of each package with targets, and the removal of what an earlier run
// left in a package without any.
func generate(set packages.Set, roots map[string]bool) ([]output.Change, []diag.Diag) {
	pkgs := rootPackages(set, roots)
	found, diags := targets(set.Fset, pkgs)
	for _, pkg := range pkgs {
		lines := directive.Lines(set.Fset, pkg.Syntax)
		diags = append(diags, directive.Loose(lines, directiveKey)...)
		diags = append(diags, directive.Misplaced(lines, pkg.Types, directive.StructType, directiveKey)...)
	}

	byPkg := make(map[string][]target)
	for _, t := range found {
		byPkg[t.PkgPath] = append(byPkg[t.PkgPath], t)
	}

	var changes []output.Change
	for _, pkg := range pkgs {
		if len(pkg.GoFiles) == 0 {
			continue
		}
		dir := filepath.Dir(pkg.GoFiles[0])
		path := filepath.Join(dir, outputFile)

		pkgTargets := byPkg[pkg.PkgPath]
		if len(pkgTargets) == 0 {
			// A package whose structs have all lost the directive keeps the
			// functions an earlier run wrote unless the file goes as well.
			if packages.GeneratedFiles(pkg)[path] {
				changes = append(changes, output.Change{Path: path, Remove: true})
			}
			continue
		}

		names := output.DeclaredNames(dir, outputFile, pkg.Name)
		for _, t := range pkgTargets {
			for _, fn := range []string{loadName(t), defineName(t)} {
				if pos, ok := names[fn]; ok {
					diags = append(diags, diag.Errorf(pos, "%s is declared already, which kanna-config writes for %s", fn, t.Name).
						WithHints("rename it, or remove the //kanna:"+directiveKey+" on "+t.Name))
				}
			}
		}
		if diag.HasErrors(diags) {
			continue
		}

		f := emit(pkg.PkgPath, pkg.Name, pkgTargets, slices.Sorted(maps.Keys(names)))
		src, err := f.Bytes()
		if err != nil {
			return nil, append(diags, diag.Errorf(token.Position{Filename: path}, "generated code does not parse: %v", err))
		}
		changes = append(changes, output.Change{Path: path, Data: src})
		diags = append(diags, output.MissingRequires(path, f.ModuleImports())...)
	}
	return changes, diags
}
