package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/agent-fox-dev/aflib/envelope"
)

// request is one request a fake forge received.
type request struct {
	Method string
	Path   string // escaped, without the query
	Query  string
	Body   map[string]any
}

func (r request) String() string { return r.Method + " " + r.Path }

// reply is what a route answers: a status and a JSON body.
type reply struct {
	status int
	body   any
}

// fakeForge is an httptest stand-in for a forge's API. Routes are keyed by
// "METHOD /escaped/path"; a request no route matches is answered 404.
type fakeForge struct {
	srv    *httptest.Server
	mu     sync.Mutex
	routes map[string]func(request) reply
	reqs   []request
}

func newFakeForge(t *testing.T) *fakeForge {
	t.Helper()
	f := &fakeForge{routes: map[string]func(request) reply{}}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req := request{Method: r.Method, Path: r.URL.EscapedPath(), Query: r.URL.RawQuery}
		if len(b) > 0 {
			_ = json.Unmarshal(b, &req.Body)
		}
		f.mu.Lock()
		f.reqs = append(f.reqs, req)
		h := f.routes[req.String()]
		f.mu.Unlock()
		rep := reply{status: http.StatusNotFound, body: map[string]any{"message": "Not Found"}}
		if h != nil {
			rep = h(req)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(rep.status)
		if rep.body != nil {
			_ = json.NewEncoder(w).Encode(rep.body)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// on answers method path with status and body.
func (f *fakeForge) on(method, path string, status int, body any) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = func(request) reply { return reply{status, body} }
}

// onFunc answers method path with what h returns.
func (f *fakeForge) onFunc(method, path string, h func(request) reply) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.routes[method+" "+path] = h
}

// requests returns what the forge received, as "METHOD path" strings.
func (f *fakeForge) requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.reqs))
	for i, r := range f.reqs {
		out[i] = r.String()
	}
	return out
}

// find returns the first request to method path.
func (f *fakeForge) find(t *testing.T, method, path string) request {
	t.Helper()
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, r := range f.reqs {
		if r.Method == method && r.Path == path {
			return r
		}
	}
	t.Fatalf("no request %s %s; got %v", method, path, f.reqs)
	return request{}
}

// writes returns the requests that were not reads.
func (f *fakeForge) writes() []string {
	var out []string
	for _, r := range f.requests() {
		if !strings.HasPrefix(r, "GET ") {
			out = append(out, r)
		}
	}
	return out
}

// githubEnv points the GitHub client at the fake forge, with a token, and
// clears every other forge variable.
func githubEnv(t *testing.T, f *fakeForge, token string) {
	t.Helper()
	t.Setenv("GITHUB_API_URL", f.srv.URL)
	t.Setenv("GITHUB_TOKEN", token)
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITLAB_API_URL", "")
	t.Setenv("GITLAB_TOKEN", "")
}

// gitlabEnv points the GitLab client at the fake forge, with a token, and
// clears every other forge variable.
func gitlabEnv(t *testing.T, f *fakeForge, token string) {
	t.Helper()
	t.Setenv("GITLAB_API_URL", f.srv.URL+"/api/v4")
	t.Setenv("GITLAB_TOKEN", token)
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITHUB_TOKEN", "")
	t.Setenv("GH_TOKEN", "")
}

// outcome is what one run of the command produced.
type outcome struct {
	code   int
	env    envelope.IssueEnvelope
	stdout string
	stderr string
}

// runIssue runs the command with args in a directory that is no git working
// tree, unless args set --dir, and decodes its one envelope.
func runIssue(t *testing.T, stdin string, args ...string) outcome {
	t.Helper()
	if !hasFlag(args, "dir") {
		args = append(args, "--dir", t.TempDir())
	}
	var stdout, stderr bytes.Buffer
	code := Main(args, strings.NewReader(stdin), &stdout, &stderr)
	o := outcome{code: code, stdout: stdout.String(), stderr: stderr.String()}
	env, err := envelope.DecodeIssue(stdout.Bytes())
	if err != nil {
		t.Fatalf("stdout is not one envelope (%v):\n%s", err, stdout.String())
	}
	o.env = env
	if env.ExitCode != code {
		t.Errorf("exit_code %d, Main returned %d", env.ExitCode, code)
	}
	if env.OK != (code == 0) || (env.Error == nil) != (code == 0) {
		t.Errorf("ok %v and error %v beside exit code %d", env.OK, env.Error, code)
	}
	return o
}

func hasFlag(args []string, name string) bool {
	for _, a := range args {
		if a == "--"+name || strings.HasPrefix(a, "--"+name+"=") {
			return true
		}
	}
	return false
}

// gitRepo makes a git working tree whose origin remote is remote.
func gitRepo(t *testing.T, remote string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	dir := t.TempDir()
	for _, args := range [][]string{{"init", "-q"}, {"remote", "add", "origin", remote}} {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	return dir
}

// ghIssue is a GitHub issue as the API writes it.
func ghIssue(number int, state, title string, pr bool) map[string]any {
	m := map[string]any{
		"number": number, "title": title, "body": "the body", "state": state,
		"html_url":   "https://github.com/o/r/issues/" + itoa(number),
		"user":       map[string]any{"login": "alice"},
		"labels":     []any{map[string]any{"name": "bug"}},
		"created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-02T10:00:00Z",
	}
	if pr {
		m["pull_request"] = map[string]any{"url": "x"}
		m["html_url"] = "https://github.com/o/r/pull/" + itoa(number)
	}
	return m
}

// ghPull is a GitHub pull request as the API writes it.
func ghPull(number int) map[string]any {
	return map[string]any{
		"number": number, "title": "Fix the crash", "body": "b", "state": "open",
		"html_url":   "https://github.com/o/r/pull/" + itoa(number),
		"user":       map[string]any{"login": "bob"},
		"head":       map[string]any{"ref": "fix", "sha": "abc1234def"},
		"base":       map[string]any{"ref": "main", "sha": "000"},
		"created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-02T10:00:00Z",
	}
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}
