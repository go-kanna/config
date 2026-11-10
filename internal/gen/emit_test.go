package gen_test

import (
	"maps"
	"path/filepath"
	"slices"
	"testing"

	"github.com/go-kanna/internal/output"
	"github.com/go-kanna/internal/packages"
	"github.com/go-kanna/internal/pkgtest"

	"github.com/go-kanna/config/internal/gen"
)

// TestEmit compares the output for testdata/emit with its golden file, which
// TestGenerated compiles and calls. To accept a change in the output:
//
//	go test ./internal/gen -run TestEmit -update
func TestEmit(t *testing.T) {
	t.Parallel()

	const dir = "testdata/emit"
	cfg := packages.Config{TypesInfo: true, Regenerated: []string{"config_gen.go"}}
	set, err := packages.Load([]string{"./" + dir}, cfg)
	if err != nil || len(set.Diags) > 0 {
		t.Fatalf("load: %v %v", err, set.Diags)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	declared := slices.Sorted(maps.Keys(output.DeclaredNames(abs, "config_gen.go", set.Packages[0].Name)))

	got, err := gen.Emit(set.Fset, set.Packages, declared)
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, got)
	}
	pkgtest.Golden(t, filepath.Join(dir, "config_gen.go"), got)
}
