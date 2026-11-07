package gen

import (
	"fmt"
	"go/token"

	"github.com/go-kanna/internal/packages"
)

// Emit renders the file the structs marked //kanna:config in pkgs, which make
// one package, are generated into. declared are the top-level names the
// package declares besides.
func Emit(fset *token.FileSet, pkgs []*packages.Package, declared []string) ([]byte, error) {
	found, diags := targets(fset, pkgs)
	if len(diags) > 0 {
		return nil, fmt.Errorf("%v", diags)
	}
	return emit(pkgs[0].PkgPath, pkgs[0].Name, found, declared).Bytes() //nolint:wrapcheck // the test shows it as it is
}

// Targets finds the structs marked //kanna:config in pkgs, naming each, and
// spelling each diagnostic as diag.Diag.String does.
func Targets(fset *token.FileSet, pkgs []*packages.Package) ([]string, []string) {
	found, diags := targets(fset, pkgs)
	names := make([]string, len(found))
	for i, t := range found {
		names[i] = t.Name
	}
	msgs := make([]string, len(diags))
	for i, d := range diags {
		msgs[i] = d.String()
	}
	return names, msgs
}
