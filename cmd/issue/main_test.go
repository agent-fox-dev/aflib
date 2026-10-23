package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/agent-fox-dev/aflib/envelope"
	"github.com/agent-fox-dev/aflib/issuex"
)

func TestParseRef(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITLAB_API_URL", "")
	repo := func(owner, name, host string) issuex.Repo { return issuex.Repo{Owner: owner, Name: name, Host: host} }
	for _, tc := range []struct {
		in   string
		want target
	}{
		{"https://github.com/acme/widgets/issues/42", target{repo: repo("acme", "widgets", "github.com"), number: 42}},
		{"https://github.com/acme/widgets/pull/7", target{repo: repo("acme", "widgets", "github.com"), number: 7, pr: true}},
		{"https://gitlab.com/g/s/p/-/merge_requests/5", target{repo: repo("g/s", "p", "gitlab.com"), number: 5, pr: true}},
		{"acme/widgets#42", target{repo: repo("acme", "widgets", ""), number: 42}},
		{"g/s/p!5", target{repo: repo("g/s", "p", ""), number: 5, pr: true}},
		{"42", target{number: 42}},
		{"#42", target{number: 42}},
		{"!7", target{number: 7, pr: true}},
		{"acme/widgets", target{repo: repo("acme", "widgets", "")}},
		{"https://github.com/acme/widgets", target{repo: repo("acme", "widgets", "github.com")}},
		{"git@gitlab.com:g/p.git", target{repo: repo("g", "p", "gitlab.com")}},
	} {
		got, err := parseRef(tc.in)
		if err != nil || got != tc.want {
			t.Errorf("parseRef(%q) = %+v, %v; want %+v", tc.in, got, err, tc.want)
		}
	}
	for _, bad := range []string{"-", "acme", "acme/widgets#x", "#0", "0", "a b", "https://example.com/o/r/issues/1", "o/r#-1"} {
		if _, err := parseRef(bad); err == nil {
			t.Errorf("parseRef(%q) succeeded, want an error", bad)
		}
	}
}

// Every usage error is exit 2, sends nothing and names the fault.
func TestUsageErrors(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	for _, tc := range []struct {
		name string
		args []string
		want string
		hint *envelope.FixHint
	}{
		{"two forms", []string{"o/r#1", "--comment", "--close", "--body", "x"}, "--comment and --close are 2 operations", nil},
		{"flag of another form", []string{"o/r#1", "--state", "closed"}, "--state does not apply to the read", nil},
		{"enum", []string{"--list", "--state", "done"}, `--state "done" is not one of open, closed, all`,
			&envelope.FixHint{Flag: "--state", Valid: []string{"open", "closed", "all"}}},
		{"create without title", []string{"o/r", "--create"}, "--create needs --title", &envelope.FixHint{Flag: "--title"}},
		{"thread form without input", []string{"--comment", "--body", "x"}, "--comment works on an issue or pull request", nil},
		{"thread form on a repository", []string{"o/r", "--close"}, `--close works on an issue or pull request, and "o/r" names a repository`, nil},
		{"repository form on a thread", []string{"o/r#3", "--list"}, "--list works on a repository", nil},
		{"empty body", []string{"o/r#1", "--comment", "--body", " "}, "--body is empty", nil},
		{"two bodies", []string{"o/r#1", "--comment", "--body", "x", "--body-file", "f"}, "--body and --body-file are two bodies", nil},
		{"edit without change", []string{"o/r#1", "--edit"}, "--edit needs a change", nil},
		{"add and remove", []string{"o/r#1", "--edit", "--add-label", "a,b", "--remove-label", "b"}, `both name "b"`, nil},
		{"pr flags without all", []string{"o/r", "--create-pr", "--head", "h"}, "--create-pr needs --base", &envelope.FixHint{Flag: "--base"}},
		{"colour", []string{"o/r", "--create-label", "bug", "--color", "red"}, `--color "red" is not six hex digits`, &envelope.FixHint{Flag: "--color"}},
		{"checks on a repository", []string{"o/r", "--checks"}, "--checks, --reviews and --files read a pull request", nil},
		{"two inputs", []string{"o/r#1", "o/r#2"}, "expected at most one input, got 2", nil},
		{"undefined flag", []string{"o/r#1", "--nope"}, "flag provided but not defined: -nope", nil},
		{"dry run of a read", []string{"o/r#1", "--dry-run"}, "--dry-run does not apply to the read", nil},
		{"bad reference", []string{"acme"}, `"acme" is not a reference`, nil},
		{"limit", []string{"--list", "--limit", "0"}, "--limit 0", &envelope.FixHint{Flag: "--limit"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := runIssue(t, "", tc.args...)
			if o.code != envelope.ExitUsage {
				t.Fatalf("exit %d, want 2: %s", o.code, o.stdout)
			}
			e := o.env.Error
			if e.Stage != "usage" || e.Category != envelope.CategoryUsage || e.Retryable {
				t.Errorf("error = %+v, want stage and category usage, not retryable", e)
			}
			if !strings.Contains(e.Message, tc.want) {
				t.Errorf("message %q does not contain %q", e.Message, tc.want)
			}
			if tc.hint != nil && (e.FixHint == nil || e.FixHint.Flag != tc.hint.Flag || !slices.Equal(e.FixHint.Valid, tc.hint.Valid)) {
				t.Errorf("fix_hint = %+v, want %+v", e.FixHint, tc.hint)
			}
			if o.env.Result != nil || o.env.Next != nil {
				t.Errorf("a usage error has a result or a next: %s", o.stdout)
			}
			if !strings.Contains(o.stderr, "issue: ") {
				t.Errorf("stderr does not repeat the message: %q", o.stderr)
			}
		})
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("usage errors sent requests: %v", got)
	}
}

func TestBareInvocation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Main([]string{"--dir", "."}, nil, &stdout, &stderr)
	if code != envelope.ExitUsage {
		t.Errorf("exit %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("the help is not on stderr: %q", stderr.String())
	}
	env, err := envelope.DecodeIssue(stdout.Bytes())
	if err != nil || env.Error == nil || env.Error.Category != envelope.CategoryUsage {
		t.Errorf("a bare invocation piped to a program writes no usage envelope: %v %s", err, stdout.String())
	}
}

func TestHelpAndVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"--help"}, nil, &stdout, &stderr); code != 0 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "Exit codes:") {
		t.Errorf("--help: exit %d, stdout %q, stderr %d bytes", code, stdout.String(), stderr.Len())
	}
	stdout.Reset()
	if code := Main([]string{"--version"}, nil, &stdout, &stderr); code != 0 || !strings.HasPrefix(stdout.String(), "issue ") {
		t.Errorf("--version: exit %d, stdout %q", code, stdout.String())
	}
}

func TestIdentity(t *testing.T) {
	defer func(v, c, b string) { version, commit, buildTime = v, c, b }(version, commit, buildTime)
	version, commit, buildTime = "v0.1.0", "94f3890", "2026-10-23T08:00:00Z"
	if got, want := identity(), "v0.1.0 (commit 94f3890, built 2026-10-23T08:00:00Z)"; got != want {
		t.Errorf("identity() = %q, want %q", got, want)
	}
}

// The --schema document is pinned: a flag or a result field that changes
// without it fails here. UPDATE_GOLDEN=1 rewrites it.
func TestSchemaGolden(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Main([]string{"--schema"}, nil, &stdout, &stderr); code != 0 {
		t.Fatalf("--schema: exit %d: %s", code, stderr.String())
	}
	var doc map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &doc); err != nil {
		t.Fatalf("--schema is not JSON: %v", err)
	}
	for _, key := range []string{"tool", "description", "input", "flags", "result", "exit_codes"} {
		if _, ok := doc[key]; !ok {
			t.Errorf("--schema has no %q", key)
		}
	}
	path := filepath.Join("testdata", "schema.golden.json")
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(path, stdout.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v; UPDATE_GOLDEN=1 go test -run TestSchemaGolden ./cmd/issue writes it", err)
	}
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Errorf("--schema differs from %s; if the change is meant, UPDATE_GOLDEN=1 go test -run TestSchemaGolden ./cmd/issue", path)
	}
}

// Every flag the command defines is described by --schema with a type and a
// description, and every flag belongs to a form or to every form.
func TestFlagsAreDescribed(t *testing.T) {
	var o options
	fs := flag.NewFlagSet(tool, flag.ContinueOnError)
	o.register(fs)
	known := slices.Clone(commonFlags)
	for _, ff := range formFlag {
		known = append(known, ff.flag)
	}
	for _, names := range formFlags {
		known = append(known, names...)
	}
	fs.VisitAll(func(f *flag.Flag) {
		if !slices.Contains(known, f.Name) {
			t.Errorf("--%s belongs to no form", f.Name)
		}
		if f.Usage == "" {
			t.Errorf("--%s has no description", f.Name)
		}
	})
}

func TestWithValueAndWithoutFlag(t *testing.T) {
	if got, want := withValue([]string{"--list", "--limit", "30", "--label=bug"}, "limit", "60"), []string{"--list", "--label=bug", "--limit", "60"}; !slices.Equal(got, want) {
		t.Errorf("withValue = %v, want %v", got, want)
	}
	if got, want := withoutFlag([]string{"--close", "--dry-run", "--body", "x", "--dry-run=true"}, "dry-run"), []string{"--close", "--body", "x"}; !slices.Equal(got, want) {
		t.Errorf("withoutFlag = %v, want %v", got, want)
	}
}

func TestSummaryIsCapped(t *testing.T) {
	long := strings.Repeat("é", 300)
	got := summarize(long, true, []envelope.Warning{{Severity: envelope.SeverityHigh}})
	if n := len([]rune(got)); n != summaryMax || !strings.HasSuffix(got, "; 1 high-severity warning") {
		t.Errorf("summary of %d characters ending %q", n, got[len(got)-30:])
	}
}
