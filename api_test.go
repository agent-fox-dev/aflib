package aflib_test

import (
	"errors"
	"flag"
	"fmt"
	"go/ast"
	"go/build"
	"go/doc"
	"go/format"
	"go/parser"
	"go/token"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// update rewrites the goldens of TestExportedAPI instead of comparing with
// them: go test -run TestExportedAPI . -update, or make api-golden.
var update = flag.Bool("update", false, "rewrite the exported-API goldens")

// rootName is the golden name of the root package.
const rootName = "aflib"

// parseMode keeps comments, so that go/doc can attach them and the renderer
// can drop them, and skips the deprecated identifier resolution.
const parseMode = parser.ParseComments | parser.SkipObjectResolution

// publicPkg is a public package of a module tree.
type publicPkg struct {
	Name string // golden name: Dir with "/" replaced by "_", the root is rootName
	Dir  string // slash-separated, relative to the module root; "." is the root
}

// skippedDir reports whether a directory is never a public package, and
// nothing below it either.
func skippedDir(name string) bool {
	switch name {
	case "testdata", "docs", "examples", "internal":
		return true
	}
	return strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// goldenName returns the golden name of a package directory.
func goldenName(dir string) string {
	if dir == "." {
		return rootName
	}
	return strings.ReplaceAll(dir, "/", "_")
}

// discover returns the public packages under root, sorted by name. A directory
// is one when it has non-test Go files for the host build context.
func discover(root string) ([]publicPkg, error) {
	var pkgs []publicPkg
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if path != root && skippedDir(d.Name()) {
			return filepath.SkipDir
		}
		p, err := build.Default.ImportDir(path, 0)
		var noGo *build.NoGoError
		if errors.As(err, &noGo) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		if len(p.GoFiles) == 0 {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		dir := filepath.ToSlash(rel)
		pkgs = append(pkgs, publicPkg{Name: goldenName(dir), Dir: dir})
		return nil
	})
	slices.SortFunc(pkgs, func(a, b publicPkg) int { return strings.Compare(a.Name, b.Name) })
	return pkgs, err
}

var (
	posType     = reflect.TypeFor[token.Pos]()
	commentType = reflect.TypeFor[*ast.CommentGroup]()
)

// normalise removes from a declaration everything the printer would take from
// the layout of the source: comments, positions (which decide blank lines, line
// breaks and whether a group keeps its parentheses) and the marker go/printer
// prints for filtered fields. What is left depends on the declaration only.
func normalise(v reflect.Value, seen map[uintptr]bool) {
	switch v.Kind() {
	case reflect.Pointer:
		if v.IsNil() || seen[v.Pointer()] {
			return
		}
		seen[v.Pointer()] = true
		normalise(v.Elem(), seen)
	case reflect.Interface:
		if !v.IsNil() {
			normalise(v.Elem(), seen)
		}
	case reflect.Slice:
		for i := range v.Len() {
			normalise(v.Index(i), seen)
		}
	case reflect.Struct:
		for i := range v.NumField() {
			f := v.Field(i)
			switch {
			case !f.CanSet():
			case f.Type() == posType:
				f.SetInt(int64(token.NoPos))
			case f.Type() == commentType:
				f.SetZero()
			default:
				normalise(f, seen)
			}
		}
		switch n := v.Addr().Interface().(type) {
		case *ast.StructType:
			n.Incomplete = false
			braceEmpty(n.Fields)
		case *ast.InterfaceType:
			n.Incomplete = false
			braceEmpty(n.Methods)
		}
	}
}

// braceEmpty makes an empty field list print as struct{} or interface{}: the
// printer does that only for braces that share a source line, so they get one
// shared position. Non-empty lists keep no positions and print one field per line.
func braceEmpty(fields *ast.FieldList) {
	if fields != nil && len(fields.List) == 0 {
		fields.Opening, fields.Closing = 1, 1
	}
}

// renderFiles returns the exported declarations of the package made of files:
// constants, variables, functions, then the types, each followed by its own
// constants, variables, functions and methods, in go/doc's order. Each
// declaration is formatted by go/format and followed by a blank line.
func renderFiles(fset *token.FileSet, files []*ast.File) (string, error) {
	pkg, err := doc.NewFromFiles(fset, files, "p")
	if err != nil {
		return "", err
	}
	var out strings.Builder
	emit := func(decl ast.Decl) error {
		normalise(reflect.ValueOf(decl), map[uintptr]bool{})
		if err := format.Node(&out, fset, decl); err != nil {
			return err
		}
		out.WriteString("\n\n")
		return nil
	}
	values := func(vs []*doc.Value) error {
		for _, v := range vs {
			if err := emit(v.Decl); err != nil {
				return err
			}
		}
		return nil
	}
	funcs := func(fs []*doc.Func) error {
		for _, f := range fs {
			if err := emit(f.Decl); err != nil {
				return err
			}
		}
		return nil
	}
	steps := []func() error{
		func() error { return values(pkg.Consts) },
		func() error { return values(pkg.Vars) },
		func() error { return funcs(pkg.Funcs) },
	}
	for _, t := range pkg.Types {
		steps = append(steps,
			func() error { return emit(t.Decl) },
			func() error { return values(t.Consts) },
			func() error { return values(t.Vars) },
			func() error { return funcs(t.Funcs) },
			func() error { return funcs(t.Methods) },
		)
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return "", err
		}
	}
	return out.String(), nil
}

// renderDir returns the exported declarations of the package in dir.
func renderDir(dir string) (string, error) {
	p, err := build.Default.ImportDir(dir, 0)
	if err != nil {
		return "", err
	}
	fset := token.NewFileSet()
	var files []*ast.File
	for _, name := range p.GoFiles {
		f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, parseMode)
		if err != nil {
			return "", err
		}
		files = append(files, f)
	}
	return renderFiles(fset, files)
}

// renderSource returns the exported declarations of a single source file.
func renderSource(src string) (string, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "src.go", src, parseMode)
	if err != nil {
		return "", err
	}
	return renderFiles(fset, []*ast.File{f})
}

// compareGolden returns a failure message when got differs from golden, or "".
// The message names the package, the first differing line and the command that
// regenerates the goldens.
func compareGolden(name, golden, got string) string {
	if golden == got {
		return ""
	}
	lines := func(s string) []string {
		if s == "" {
			return nil
		}
		return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
	}
	g, r := lines(golden), lines(got)
	at := func(l []string, i int) string {
		if i < len(l) {
			return fmt.Sprintf("%q", l[i])
		}
		return "<end of file>"
	}
	i := 0
	for i < min(len(g), len(r)) && g[i] == r[i] {
		i++
	}
	return fmt.Sprintf("package %s: the exported API differs from its golden at line %d:\n\tgolden:   %s\n\trendered: %s\n"+
		"If the change is deliberate, run make api-golden and commit the new golden.",
		name, i+1, at(g, i), at(r, i))
}

// checkAPI compares the rendering of every public package under root with
// goldenDir/<name>.golden and returns one message per problem. With update
// set it rewrites the goldens of all packages and removes the orphans instead.
func checkAPI(root, goldenDir string, update bool) ([]string, error) {
	pkgs, err := discover(root)
	if err != nil {
		return nil, err
	}
	var problems []string
	known := map[string]bool{}
	for _, p := range pkgs {
		known[p.Name+".golden"] = true
		got, err := renderDir(filepath.Join(root, filepath.FromSlash(p.Dir)))
		if err != nil {
			return nil, fmt.Errorf("package %s: %w", p.Name, err)
		}
		path := filepath.Join(goldenDir, p.Name+".golden")
		if update {
			if err := os.MkdirAll(goldenDir, 0o755); err != nil {
				return nil, err
			}
			if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
				return nil, err
			}
			continue
		}
		want, err := os.ReadFile(path)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			problems = append(problems, fmt.Sprintf("public package %s has no golden %s; run make api-golden to create it", p.Name, path))
		case err != nil:
			return nil, err
		default:
			if msg := compareGolden(p.Name, string(want), got); msg != "" {
				problems = append(problems, msg)
			}
		}
	}
	entries, err := os.ReadDir(goldenDir)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".golden") || known[e.Name()] {
			continue
		}
		path := filepath.Join(goldenDir, e.Name())
		if !update {
			problems = append(problems, fmt.Sprintf("golden %s matches no public package; delete it, or run make api-golden", path))
		} else if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	return problems, nil
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func names(pkgs []publicPkg) []string {
	var out []string
	for _, p := range pkgs {
		out = append(out, p.Name)
	}
	return out
}

func dirs(pkgs []publicPkg) []string {
	var out []string
	for _, p := range pkgs {
		out = append(out, p.Dir)
	}
	return out
}

// copyTree copies the directory src to a new temporary directory.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(dst, rel), 0o755)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(dst, rel), b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return dst
}

// TestDocGo_TS_01_3 verifies TS-01-3 (01-REQ-1.3): doc.go holds only a package
// doc comment that names the module and its two packages, and the root golden
// is empty.
func TestDocGo_TS_01_3(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "doc.go", nil, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	if f.Name.Name != "aflib" {
		t.Errorf("package clause is %q, want aflib", f.Name.Name)
	}
	if len(f.Decls) != 0 || len(f.Imports) != 0 {
		t.Errorf("doc.go has %d declarations and %d imports, want none", len(f.Decls), len(f.Imports))
	}
	if f.Doc == nil {
		t.Fatal("doc.go has no package doc comment")
	}
	text := strings.Join(strings.Fields(f.Doc.Text()), " ")
	for _, want := range []string{"github.com/agent-fox-dev/aflib", "shared by the agent-fox programs", "issuex", "jobqueue"} {
		if !strings.Contains(text, want) {
			t.Errorf("the doc comment does not mention %q", want)
		}
	}
	if b := read(t, filepath.Join("testdata", "api", "aflib.golden")); b != "" {
		t.Errorf("testdata/api/aflib.golden has %d bytes, want 0", len(b))
	}
}

// TestDiscover_TS_01_25 verifies TS-01-25 (01-REQ-5.1): public packages are
// found, and testdata, dot, underscore, docs, examples and internal
// directories and directories with only test files are skipped.
func TestDiscover_TS_01_25(t *testing.T) {
	got, err := discover(filepath.Join("testdata", "fixtures", "discovery"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b/c"}; !slices.Equal(dirs(got), want) {
		t.Errorf("fixture packages = %v, want %v", dirs(got), want)
	}
	if want := []string{"a", "b_c"}; !slices.Equal(names(got), want) {
		t.Errorf("fixture golden names = %v, want %v", names(got), want)
	}

	got, err = discover(".")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aflib", "issuex", "jobqueue"}; !slices.Equal(names(got), want) {
		t.Errorf("module packages = %v, want %v", names(got), want)
	}
}

// TestRender_TS_01_26 verifies TS-01-26 (01-REQ-5.2): declarations come in the
// order constants, variables, functions, types (each followed by its own
// constants, variables, functions and methods), exported only, without
// comments, package clause or import path.
func TestRender_TS_01_26(t *testing.T) {
	got, err := renderDir(filepath.Join("testdata", "fixtures", "render", "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	if want := read(t, filepath.Join("testdata", "fixtures", "render", "expected.golden")); got != want {
		t.Errorf("rendering differs from expected.golden:\n%s", compareGolden("render", want, got))
	}
	for _, bad := range []string{"//", "/*", "package ", "probe", "hidden", "helper", "secret", "errHidden"} {
		if strings.Contains(got, bad) {
			t.Errorf("rendering contains %q", bad)
		}
	}
}

// TestExportedAPI verifies TS-01-27 (01-REQ-5.3 to 5.7) on the real module:
// every public package has a golden and renders to it. The goldens equal the
// exported declarations of the source packages because the non-test files are
// byte-identical to them. Run with -update, or make api-golden, to rewrite them.
func TestExportedAPI(t *testing.T) {
	problems, err := checkAPI(".", filepath.Join("testdata", "api"), *update)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range problems {
		t.Error(p)
	}
}

// TestCompareGolden_TS_01_28 verifies TS-01-28 (01-REQ-5.4): a rendering that
// differs from its golden fails with the package, the first differing line
// and the regenerate command.
func TestCompareGolden_TS_01_28(t *testing.T) {
	problems, err := checkAPI(filepath.Join("testdata", "fixtures", "api", "mismatch", "src"),
		filepath.Join("testdata", "fixtures", "api", "mismatch", "golden"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 {
		t.Fatalf("got %d problems, want 1: %q", len(problems), problems)
	}
	for _, want := range []string{"p", "line 1", "func F(n int) int", "func F(n int) string", "make api-golden"} {
		if !strings.Contains(problems[0], want) {
			t.Errorf("the message does not contain %q: %s", want, problems[0])
		}
	}

	// the first differing line, also when one side ends early
	msg := compareGolden("q", "a\nb\n", "a\nb\nc\n")
	for _, want := range []string{"q", "line 3", "c", "make api-golden"} {
		if !strings.Contains(msg, want) {
			t.Errorf("a longer rendering: the message does not contain %q: %s", want, msg)
		}
	}
	if msg := compareGolden("q", "a\nb\n", "a\nb\n"); msg != "" {
		t.Errorf("equal texts gave the message %q", msg)
	}
}

// TestMissingGolden_TS_01_29 verifies TS-01-29 (01-REQ-5.5): a public package
// without a golden fails and is named.
func TestMissingGolden_TS_01_29(t *testing.T) {
	problems, err := checkAPI(filepath.Join("testdata", "fixtures", "api", "missing", "src"),
		filepath.Join("testdata", "fixtures", "api", "missing", "golden"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "newpkg") {
		t.Errorf("problems = %q, want one naming newpkg", problems)
	}
}

// TestOrphanGolden_TS_01_30 verifies TS-01-30 (01-REQ-5.6): a golden that
// matches no package fails and is named.
func TestOrphanGolden_TS_01_30(t *testing.T) {
	problems, err := checkAPI(filepath.Join("testdata", "fixtures", "api", "orphan", "src"),
		filepath.Join("testdata", "fixtures", "api", "orphan", "golden"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) != 1 || !strings.Contains(problems[0], "ghost.golden") {
		t.Errorf("problems = %q, want one naming ghost.golden", problems)
	}
}

// TestUpdate_TS_01_31 verifies TS-01-31 (01-REQ-5.7) at the helper level: with
// update set a stale golden is rewritten, a missing one is written and an
// orphan is removed, and the next check passes. The go test -update path is
// exercised on a scratch copy of the module by the end-to-end checks.
func TestUpdate_TS_01_31(t *testing.T) {
	tree := copyTree(t, filepath.Join("testdata", "fixtures", "api", "stale"))
	src, golden := filepath.Join(tree, "src"), filepath.Join(tree, "golden")

	if problems, err := checkAPI(src, golden, false); err != nil || len(problems) != 3 {
		t.Fatalf("before the update: problems = %q, err = %v, want 3 (stale, missing, orphan)", problems, err)
	}
	if problems, err := checkAPI(src, golden, true); err != nil || len(problems) != 0 {
		t.Fatalf("the update reported problems = %q, err = %v", problems, err)
	}
	if problems, err := checkAPI(src, golden, false); err != nil || len(problems) != 0 {
		t.Errorf("after the update: problems = %q, err = %v", problems, err)
	}
	if _, err := os.Stat(filepath.Join(golden, "ghost.golden")); !os.IsNotExist(err) {
		t.Errorf("the orphan golden survived the update: %v", err)
	}
	want, err := renderDir(filepath.Join(src, "q"))
	if err != nil {
		t.Fatal(err)
	}
	if got := read(t, filepath.Join(golden, "q.golden")); got != want || got == "" {
		t.Errorf("q.golden = %q, want the rendering %q", got, want)
	}
}

// item is one declaration of a generated package.
type item struct {
	kind     int
	id       int
	exported bool
}

const itemKinds = 8

// genItems returns a small random list of declarations.
func genItems(r *rand.Rand) []item {
	n := 3 + r.IntN(8)
	items := make([]item, n)
	for i := range items {
		items[i] = item{kind: r.IntN(itemKinds), id: i, exported: r.IntN(3) != 0}
	}
	return items
}

// decor adds what a rendering must ignore: comments of every kind, renamed
// unexported identifiers and extra unexported declarations. A nil *decor adds
// nothing.
type decor struct{ r *rand.Rand }

func (d *decor) pick(options ...string) string {
	if d == nil {
		return ""
	}
	return options[d.r.IntN(len(options))]
}

func (d *decor) doc() string {
	return d.pick("", "// doc comment\n", "/* block doc */\n", "// first\n// second\n")
}

func (d *decor) trail() string { return d.pick("", " // trailing", " /* trailing */") }

// between returns an unexported line to insert between two exported ones, or "".
func (d *decor) between(format string, id int) string {
	if d.pick("", "x") == "" {
		return ""
	}
	return fmt.Sprintf(format, id)
}

func (d *decor) standalone() string { return d.pick("", "\t// standalone\n", "\t/* standalone */\n") }

// unexported returns the name of an unexported identifier.
func (d *decor) unexported(base string, id int) string {
	if d == nil {
		return fmt.Sprintf("%s%d", base, id)
	}
	return fmt.Sprintf("%s%d%s", base, id, d.pick("", "x", "renamed"))
}

// extra returns an unexported declaration to insert, or "".
func (d *decor) extra(id int) string {
	switch d.pick("", "const", "var", "func", "type") {
	case "const":
		return fmt.Sprintf("const extraC%d = %d\n\n", id, id)
	case "var":
		return fmt.Sprintf("var extraV%d = %q\n\n", id, "v")
	case "func":
		return fmt.Sprintf("func extraF%d() {}\n\n", id)
	case "type":
		return fmt.Sprintf("type extraT%d struct{ a int }\n\n", id)
	}
	return ""
}

// writeSource renders items as Go source. With a nil decor it is bare.
func writeSource(items []item, d *decor) string {
	var b strings.Builder
	b.WriteString(d.pick("", "// Package p is generated.\n", "/* Package p. */\n"))
	b.WriteString("package p\n\n")
	for _, it := range items {
		b.WriteString(d.extra(it.id))
		b.WriteString(d.doc())
		name := fmt.Sprintf("E%d", it.id)
		if !it.exported {
			name = d.unexported("u", it.id)
		}
		switch it.kind {
		case 0:
			fmt.Fprintf(&b, "const %s = %d%s\n\n", name, it.id, d.trail())
		case 1:
			fmt.Fprintf(&b, "var %s = %q%s\n\n", name, "v", d.trail())
		case 2:
			fmt.Fprintf(&b, "func %s(a int) string {\n\t// body comment\n\treturn \"\"\n}\n\n", name)
		case 3:
			// The exported fields A and B are neighbours in the bare source, so
			// comments and unexported fields between them change their line gap.
			fmt.Fprintf(&b, "type %s struct {\n%s\tA int%s\n%s%s\tB []byte\n\t%s string\n%s}\n\n",
				name, d.standalone(), d.trail(), d.standalone(), d.between("\tmid%d int\n", it.id), d.unexported("hidden", it.id), d.standalone())
		case 4:
			fmt.Fprintf(&b, "type %s interface {\n%s\tDo() error%s\n%s%s\tStop()\n\t%s()\n}\n\n",
				name, d.standalone(), d.trail(), d.standalone(), d.between("\tmid%d()\n", it.id), d.unexported("done", it.id))
		case 5:
			fmt.Fprintf(&b, "type %s int\n\n%sconst (\n\t%sFirst %s = iota%s\n\t%s\n)\n\n", name, d.doc(), name, name, d.trail(), d.unexported("unexp", it.id))
			fmt.Fprintf(&b, "%sfunc (t %s) Get() int { return int(t) }\n\n", d.doc(), name)
			fmt.Fprintf(&b, "func (t %s) %s() {}\n\n", name, d.unexported("set", it.id))
		case 6:
			// An exported function that names an unexported type would change
			// when that type is renamed, so only exported types get one.
			ctor := fmt.Sprintf("newT%d", it.id)
			if it.exported {
				ctor = fmt.Sprintf("NewT%d", it.id)
			}
			fmt.Fprintf(&b, "type %s struct{ n int }\n\n%sfunc %s() *%s { return &%s{} }\n\n", name, d.doc(), ctor, name, name)
		case 7:
			fmt.Fprintf(&b, "const (\n\t%s = iota\n\tG%da = %d%s\n%s%s\tG%db = %d\n\t%s = 0\n)\n\n",
				d.unexported("lead", it.id), it.id, it.id, d.trail(), d.standalone(), d.between("\tmid%d = 7\n", it.id), it.id, it.id, d.unexported("tail", it.id))
		}
	}
	return b.String()
}

// TestRenderProperties_TS_01_32 verifies TS-01-32 (01-REQ-5.8): for generated
// packages the rendering is deterministic and does not change when only
// comments or unexported identifiers change, but does change with an exported
// one.
func TestRenderProperties_TS_01_32(t *testing.T) {
	nonEmpty := 0
	for seed := uint64(1); seed <= 300; seed++ {
		r := rand.New(rand.NewPCG(seed, seed))
		items := genItems(r)
		bare := writeSource(items, nil)
		first, err := renderSource(bare)
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, bare)
		}
		if again, _ := renderSource(bare); again != first {
			t.Fatalf("seed %d: two renderings of the same source differ", seed)
		}
		decorated := writeSource(items, &decor{r: rand.New(rand.NewPCG(seed, seed+1))})
		got, err := renderSource(decorated)
		if err != nil {
			t.Fatalf("seed %d: %v\n%s", seed, err, decorated)
		}
		if got != first {
			t.Fatalf("seed %d: comments and unexported identifiers changed the rendering\nbare:\n%s\ndecorated:\n%s\n--- bare rendering:\n%s\n--- decorated rendering:\n%s", seed, bare, decorated, first, got)
		}
		if first == "" {
			continue
		}
		nonEmpty++
		changed, err := renderSource(strings.Replace(bare, "E", "X", 1))
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		if changed == first {
			t.Fatalf("seed %d: renaming an exported identifier did not change the rendering", seed)
		}
	}
	if nonEmpty < 100 {
		t.Errorf("only %d of 300 generated packages have exported declarations, the property is barely exercised", nonEmpty)
	}
}
