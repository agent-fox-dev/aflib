package aflib_test

import (
	"errors"
	"fmt"
	"go/build"
	"go/parser"
	"go/token"
	"io/fs"
	"math/rand/v2"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	uuidModule   = "github.com/google/uuid"
	sqliteModule = "modernc.org/sqlite"
)

// ruledIn lists the modules added as test-only dependencies under 01-REQ-4.4,
// each with a ruling in docs/architecture.md. The import needed none.
var ruledIn []string

// modReq is a requirement of go.mod.
type modReq struct {
	Path     string
	Indirect bool
}

// goMod is what the guard needs of a go.mod file.
type goMod struct {
	Module   string
	Requires []modReq
	Replaces []string // the text of each directive, e.g. "example.com/x => ../x"
}

// isStdlib reports whether an import path is in the standard library: the
// first element of the path has no dot.
func isStdlib(path string) bool {
	first, _, _ := strings.Cut(path, "/")
	return !strings.Contains(first, ".")
}

// inModule reports whether an import path belongs to a module.
func inModule(path, module string) bool {
	return path == module || strings.HasPrefix(path, module+"/")
}

// parseGoMod reads the module line, the requirements (with their indirect
// marks) and the replace directives of a go.mod, in single-line and block form.
// Other directives are ignored.
func parseGoMod(text string) goMod {
	var m goMod
	block := ""
	for line := range strings.Lines(text) {
		code, comment, _ := strings.Cut(line, "//")
		code = strings.TrimSpace(code)
		if code == "" {
			continue
		}
		indirect := strings.HasPrefix(strings.TrimSpace(comment), "indirect")
		if block != "" {
			if code == ")" {
				block = ""
			} else {
				m.add(block, code, indirect)
			}
			continue
		}
		verb, rest, _ := strings.Cut(code, " ")
		rest = strings.TrimSpace(rest)
		switch {
		case verb == "module":
			m.Module = rest
		case (verb == "require" || verb == "replace") && rest == "(":
			block = verb
		case verb == "require" || verb == "replace":
			m.add(verb, rest, indirect)
		}
	}
	return m
}

// add records one require or replace directive.
func (m *goMod) add(verb, spec string, indirect bool) {
	fields := strings.Fields(spec)
	switch {
	case verb == "require" && len(fields) > 0:
		m.Requires = append(m.Requires, modReq{Path: fields[0], Indirect: indirect})
	case verb == "replace":
		m.Replaces = append(m.Replaces, strings.Join(fields, " "))
	}
}

// checkGoMod returns one message per replace directive and per direct
// requirement outside the allowlist: uuid, sqlite and the modules ruled in.
func checkGoMod(text string, ruled []string) []string {
	m := parseGoMod(text)
	var problems []string
	for _, r := range m.Replaces {
		problems = append(problems, fmt.Sprintf("go.mod: replace directive %q is not allowed", r))
	}
	allowed := append([]string{uuidModule, sqliteModule}, ruled...)
	for _, r := range m.Requires {
		if !r.Indirect && !slices.Contains(allowed, r.Path) {
			problems = append(problems, fmt.Sprintf("go.mod: %s is required directly but is not in the allowlist (%s)", r.Path, strings.Join(allowed, ", ")))
		}
	}
	return problems
}

// importAllowed reports whether a file may import path. Every file may import
// the standard library, uuid and the module's own packages; test files and the
// files of internal packages may also import sqlite and the modules ruled in.
func importAllowed(path, module string, test bool, ruled []string) bool {
	if isStdlib(path) || inModule(path, module) || inModule(path, uuidModule) {
		return true
	}
	return test && (inModule(path, sqliteModule) || slices.ContainsFunc(ruled, func(m string) bool { return inModule(path, m) }))
}

// checkTree returns one message per disallowed import in the Go files under
// root. Non-test files of the public packages may import the standard library,
// uuid and packages of the module; test files and the files of internal
// packages may also import sqlite and the modules ruled in.
func checkTree(root, module string, ruled []string) ([]string, error) {
	var problems []string
	err := filepath.WalkDir(root, func(dir string, d fs.DirEntry, err error) error {
		if err != nil || !d.IsDir() {
			return err
		}
		if dir != root && d.Name() != "internal" && skippedDir(d.Name()) {
			return filepath.SkipDir
		}
		p, err := build.Default.ImportDir(dir, 0)
		var noGo *build.NoGoError
		if errors.As(err, &noGo) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("%s: %w", dir, err)
		}
		rel, err := filepath.Rel(root, dir)
		if err != nil {
			return err
		}
		internal := slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "internal")
		check := func(names []string, test bool) error {
			for _, name := range names {
				f, err := parser.ParseFile(token.NewFileSet(), filepath.Join(dir, name), nil, parser.ImportsOnly)
				if err != nil {
					return err
				}
				for _, imp := range f.Imports {
					path, _ := strconv.Unquote(imp.Path.Value)
					if !importAllowed(path, module, test, ruled) {
						problems = append(problems, fmt.Sprintf("%s: import %q is not allowed here (%s)",
							pathpkg.Join(filepath.ToSlash(rel), name), path, importRule(test)))
					}
				}
			}
			return nil
		}
		if err := check(append(slices.Clone(p.GoFiles), p.CgoFiles...), internal); err != nil {
			return err
		}
		return check(append(slices.Clone(p.TestGoFiles), p.XTestGoFiles...), true)
	})
	return problems, err
}

// importRule describes what a file may import, for the messages.
func importRule(test bool) string {
	if test {
		return "test files and internal packages may import the standard library, " + uuidModule + ", " + sqliteModule + ", the modules ruled in and this module"
	}
	return "non-test files may import the standard library, " + uuidModule + " and this module"
}

func fixture(parts ...string) string {
	return filepath.Join(append([]string{"testdata", "fixtures", "deps"}, parts...)...)
}

// TestReplaceDirective_TS_01_33 verifies TS-01-33 (01-REQ-6.1): a replace
// directive, single-line or in a block, fails the check and is named.
func TestReplaceDirective_TS_01_33(t *testing.T) {
	cases := map[string][]string{
		"replace_single.mod": {"example.com/x => ../x"},
		"replace_block.mod":  {"example.com/x => ../x", "example.com/y v1.2.3 => example.com/z v1.0.0"},
	}
	for file, directives := range cases {
		problems := checkGoMod(read(t, fixture("gomod", file)), nil)
		if len(problems) != len(directives) {
			t.Errorf("%s: got %d problems, want %d: %q", file, len(problems), len(directives), problems)
			continue
		}
		for i, want := range directives {
			if !strings.Contains(problems[i], "go.mod") || !strings.Contains(problems[i], "replace") || !strings.Contains(problems[i], want) {
				t.Errorf("%s: the message does not name go.mod, replace and %q: %s", file, want, problems[i])
			}
		}
	}
}

// TestDirectRequire_TS_01_34 verifies TS-01-34 (01-REQ-6.2): a direct
// requirement outside the allowlist fails, an indirect one does not, and a
// module ruled in is allowed.
func TestDirectRequire_TS_01_34(t *testing.T) {
	for _, file := range []string{"direct_single.mod", "direct_block.mod"} {
		problems := checkGoMod(read(t, fixture("gomod", file)), nil)
		if len(problems) != 1 || !strings.Contains(problems[0], "go.mod") || !strings.Contains(problems[0], "example.com/bad") {
			t.Errorf("%s: problems = %q, want one naming go.mod and example.com/bad", file, problems)
		}
		if problems := checkGoMod(read(t, fixture("gomod", file)), []string{"example.com/bad"}); len(problems) != 0 {
			t.Errorf("%s: a module ruled in is still reported: %q", file, problems)
		}
	}
	for _, file := range []string{"indirect.mod", "allowed.mod"} {
		if problems := checkGoMod(read(t, fixture("gomod", file)), nil); len(problems) != 0 {
			t.Errorf("%s: problems = %q, want none", file, problems)
		}
	}
}

// TestPublicImports_TS_01_35 verifies TS-01-35 (01-REQ-6.3): a non-test file of
// a public package may import the standard library, uuid and the module's own
// packages, and nothing else; sqlite is for tests.
func TestPublicImports_TS_01_35(t *testing.T) {
	check := func(scenario string) []string {
		t.Helper()
		problems, err := checkTree(fixture("imports", scenario), "example.com/m", nil)
		if err != nil {
			t.Fatal(err)
		}
		return problems
	}
	problems := check("badpublic")
	if len(problems) != 1 || !strings.Contains(problems[0], "p/p.go") || !strings.Contains(problems[0], "example.com/bad/lib") {
		t.Errorf("badpublic: problems = %q, want one naming p/p.go and example.com/bad/lib", problems)
	}
	if problems := check("okpublic"); len(problems) != 0 {
		t.Errorf("okpublic: problems = %q, want none", problems)
	}
	problems = check("sqlitepublic")
	if len(problems) != 1 || !strings.Contains(problems[0], "p/p.go") || !strings.Contains(problems[0], sqliteModule) {
		t.Errorf("sqlitepublic: problems = %q, want one naming p/p.go and %s", problems, sqliteModule)
	}
	if problems := check("skipped"); len(problems) != 0 {
		t.Errorf("skipped: a testdata directory was checked: %q", problems)
	}
}

// TestTestImports_TS_01_36 verifies TS-01-36 (01-REQ-6.4): test files and the
// files of internal packages may also import sqlite and the modules ruled in,
// but not other modules.
func TestTestImports_TS_01_36(t *testing.T) {
	check := func(scenario string, ruled ...string) []string {
		t.Helper()
		problems, err := checkTree(fixture("imports", scenario), "example.com/m", ruled)
		if err != nil {
			t.Fatal(err)
		}
		return problems
	}
	for _, scenario := range []string{"tests", "sqliteinternal"} {
		if problems := check(scenario); len(problems) != 0 {
			t.Errorf("%s: problems = %q, want none", scenario, problems)
		}
	}
	for scenario, file := range map[string]string{"badtests": "p/p_test.go", "badinternal": "internal/h/h.go"} {
		problems := check(scenario)
		if len(problems) != 1 || !strings.Contains(problems[0], file) || !strings.Contains(problems[0], "example.com/bad/lib") {
			t.Errorf("%s: problems = %q, want one naming %s and example.com/bad/lib", scenario, problems, file)
		}
	}
	if problems := check("ruled"); len(problems) != 1 || !strings.Contains(problems[0], "example.com/ruled/lib") {
		t.Errorf("ruled, not ruled in: problems = %q, want one naming example.com/ruled/lib", problems)
	}
	if problems := check("ruled", "example.com/ruled"); len(problems) != 0 {
		t.Errorf("ruled, ruled in: problems = %q, want none", problems)
	}
}

// TestIsStdlib_TS_01_37 verifies TS-01-37 (01-REQ-6.5): an import path is
// standard library exactly when its first element contains no dot.
func TestIsStdlib_TS_01_37(t *testing.T) {
	alphabet := []byte("abcxyz019.-")
	r := rand.New(rand.NewPCG(37, 37))
	for range 2000 {
		elems := make([]string, 1+r.IntN(4))
		for i := range elems {
			e := make([]byte, 1+r.IntN(6))
			for j := range e {
				e[j] = alphabet[r.IntN(len(alphabet))]
			}
			elems[i] = string(e)
		}
		p := strings.Join(elems, "/")
		firstHasDot := false
		for i := 0; i < len(p) && p[i] != '/'; i++ {
			firstHasDot = firstHasDot || p[i] == '.'
		}
		if got := isStdlib(p); got == firstHasDot {
			t.Fatalf("isStdlib(%q) = %v, but the first element has a dot: %v", p, got, firstHasDot)
		}
	}
	for path, want := range map[string]bool{"fmt": true, "net/http": true, "go/build": true, "github.com/google/uuid": false, "example.com": false, "a.b/c": false} {
		if got := isStdlib(path); got != want {
			t.Errorf("isStdlib(%q) = %v, want %v", path, got, want)
		}
	}
}

// TestParseGoMod_TS_01_38 verifies TS-01-38 (01-REQ-6.6): go.mod is parsed with
// the standard library only, and so are the root guard tests themselves.
func TestParseGoMod_TS_01_38(t *testing.T) {
	got := parseGoMod(read(t, fixture("gomod", "parse.mod")))
	want := goMod{
		Module: "example.com/m",
		Requires: []modReq{
			{"example.com/single", false},
			{"example.com/direct", false},
			{"example.com/quiet", true},
			{"example.com/tail", false},
		},
		Replaces: []string{
			"example.com/a => ../a",
			"example.com/b => ../b",
			"example.com/c v1.0.0 => example.com/d v1.0.1",
		},
	}
	if got.Module != want.Module || !slices.Equal(got.Requires, want.Requires) || !slices.Equal(got.Replaces, want.Replaces) {
		t.Errorf("parseGoMod =\n%+v\nwant\n%+v", got, want)
	}

	module := parseGoMod(read(t, "go.mod")).Module
	for _, file := range []string{"api_test.go", "deps_test.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatal(err)
		}
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			if !isStdlib(p) && p != module && !strings.HasPrefix(p, module+"/") {
				t.Errorf("%s imports %q, which is neither the standard library nor this module", file, p)
			}
		}
	}
}

// TestDependencies verifies TS-01-39 (01-REQ-6.7): the committed tree has no
// replace directive, no direct requirement outside the allowlist and no
// disallowed import. Each message names the file and the directive or import.
func TestDependencies(t *testing.T) {
	text := read(t, "go.mod")
	problems := checkGoMod(text, ruledIn)
	more, err := checkTree(".", parseGoMod(text).Module, ruledIn)
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range append(problems, more...) {
		t.Error(p)
	}
}

// TestPackageLayout_TS_01_2 verifies TS-01-2 (01-REQ-1.2): the module has
// exactly the packages aflib, issuex and jobqueue, and no cmd directory. An
// internal/ helper copied under 01-REQ-4.3 would be recorded in the erratum and
// in docs/architecture.md; steering forbids tests of docs, so that is reviewed.
func TestPackageLayout_TS_01_2(t *testing.T) {
	pkgs, err := discover(".")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"aflib", "issuex", "jobqueue"}; !slices.Equal(names(pkgs), want) {
		t.Errorf("public packages = %v, want %v", names(pkgs), want)
	}
	if matches, _ := filepath.Glob("cmd"); len(matches) != 0 {
		t.Errorf("the module has a cmd directory")
	}
}
