package report

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"runtime"
	"testing"
)

// The 48 and 40 in the book hold for 64-bit targets. Ask go/types
// for each target's gc layout without cross-compiling.
func TestSizesPerArch(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "posting.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][2]int64{ // Posting, PostingPacked
		"amd64": {48, 40}, "arm64": {48, 40},
		"ppc64le": {48, 40}, "riscv64": {48, 40}, "s390x": {48, 40},
		"386": {32, 28}, "arm": {32, 28}, // int64 aligns to 4
	}
	for arch, w := range want {
		conf := types.Config{Sizes: types.SizesFor("gc", arch)}
		pkg, err := conf.Check("report", fset, []*ast.File{f}, nil)
		if err != nil {
			t.Fatal(err)
		}
		var got [2]int64
		for i, name := range []string{"Posting", "PostingPacked"} {
			got[i] = conf.Sizes.Sizeof(pkg.Scope().Lookup(name).Type())
		}
		if got != w {
			t.Errorf("%s: sizes %v, want %v", arch, got, w)
		}
	}
}

// Alone on the heap, a 40-byte PostingPacked takes a 48-byte size
// class: the saving shows up only side by side.
func TestLoneStructUsesSizeClass(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	const n = 10000
	keep := make([]*PostingPacked, n)
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	for i := range keep {
		keep[i] = new(PostingPacked)
	}
	runtime.ReadMemStats(&after)
	per := (after.TotalAlloc - before.TotalAlloc) / n
	if per != 48 {
		t.Fatalf("lone PostingPacked takes %d bytes, want 48", per)
	}
	runtime.KeepAlive(keep)
}
