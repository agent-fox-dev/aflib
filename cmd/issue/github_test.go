package main

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/agent-fox-dev/aflib/envelope"
)

func TestReadIssue(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "Crash on start", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})

	o := runIssue(t, "", "o/r#42")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	res := o.env.Result
	if res.Forge != "github" || res.Repo != "o/r" || res.Ref != "o/r#42" {
		t.Errorf("forge %q repo %q ref %q", res.Forge, res.Repo, res.Ref)
	}
	if res.Issue == nil || res.Issue.Title != "Crash on start" || !slices.Equal(res.Issue.Labels, []string{"bug"}) {
		t.Errorf("issue = %+v", res.Issue)
	}
	if res.Comments == nil || len(res.Comments) != 0 || !strings.Contains(o.stdout, `"comments": []`) {
		t.Errorf("a thread with no comments is not written as []: %s", o.stdout)
	}
	if res.PullRequest != nil || o.env.SideEffects != nil {
		t.Errorf("a read of an issue has a pull request or side effects: %s", o.stdout)
	}
	if want := `issue: o/r#42 open: "Crash on start" (0 comments)`; o.env.Summary != want {
		t.Errorf("summary %q, want %q", o.env.Summary, want)
	}
}

func TestReadPullRequestWithChecksReviewsFiles(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/7", 200, ghIssue(7, "open", "Fix the crash", true))
	f.on("GET", "/repos/o/r/issues/7/comments", 200, []any{map[string]any{"id": 1, "body": "looks good", "user": map[string]any{"login": "carol"}}})
	f.on("GET", "/repos/o/r/pulls/7", 200, ghPull(7))
	f.on("GET", "/repos/o/r/commits/abc1234def/check-runs", 200, map[string]any{"check_runs": []any{
		map[string]any{"name": "test", "status": "completed", "conclusion": "success"},
		map[string]any{"name": "lint", "status": "completed", "conclusion": "failure"},
	}})
	f.on("GET", "/repos/o/r/pulls/7/reviews", 200, []any{map[string]any{"user": map[string]any{"login": "carol"}, "state": "APPROVED"}})
	f.on("GET", "/repos/o/r/pulls/7/files", 200, []any{map[string]any{"filename": "main.go", "status": "modified", "additions": 3}})

	o := runIssue(t, "", "o/r#7", "--checks", "--reviews", "--files")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	res := o.env.Result
	if res.Ref != "o/r!7" || res.PullRequest == nil || res.PullRequest.HeadBranch != "fix" {
		t.Errorf("ref %q pull request %+v", res.Ref, res.PullRequest)
	}
	if len(res.Checks) != 2 || len(res.Reviews) != 1 || res.Reviews[0].State != "approved" || len(res.Files) != 1 || len(res.Comments) != 1 {
		t.Errorf("checks %v reviews %v files %v comments %v", res.Checks, res.Reviews, res.Files, res.Comments)
	}
	want := `issue: o/r!7 open pull request fix → main: "Fix the crash" (1 comment); checks: 1 success, 1 failure; 1 review; 1 file changed`
	if o.env.Summary != want {
		t.Errorf("summary %q\nwant    %q", o.env.Summary, want)
	}
}

func TestChecksOfAnIssueIsAUsageError(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/5", 200, ghIssue(5, "open", "t", false))
	f.on("GET", "/repos/o/r/issues/5/comments", 200, []any{})
	o := runIssue(t, "", "o/r#5", "--checks")
	if o.code != 2 || !strings.Contains(o.env.Error.Message, "o/r#5 is an issue, not a pull request") || o.env.Result != nil {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestReadRepository(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "")
	f.on("GET", "/repos/o/r", 200, map[string]any{"full_name": "o/r", "default_branch": "main", "allow_squash_merge": true})
	o := runIssue(t, "", "o/r")
	if o.code != 0 || o.env.Result.Repository == nil || !o.env.Result.Repository.AllowSquashMerge {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if want := "issue: o/r on github, default branch main"; o.env.Summary != want {
		t.Errorf("summary %q, want %q", o.env.Summary, want)
	}
}

func TestList(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "")
	f.on("GET", "/repos/o/r/issues", 200, []any{
		ghIssue(3, "open", "three", false), ghIssue(2, "open", "a pull request", true), ghIssue(1, "open", "one", false),
	})
	dir := gitRepo(t, f.srv.URL+"/o/r.git")
	o := runIssue(t, "", "--list", "--label", "bug,ui", "--assignee", "alice", "--limit", "1", "--dir", dir)
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	res := o.env.Result
	if len(res.Issues) != 1 || res.Issues[0].Number != 3 || !res.Incomplete {
		t.Errorf("issues %+v incomplete %v", res.Issues, res.Incomplete)
	}
	q := f.find(t, "GET", "/repos/o/r/issues").Query
	for _, want := range []string{"state=open", "labels=bug%2Cui", "assignee=alice", "sort=created", "direction=desc"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q has no %q", q, want)
		}
	}
	if want := "issue: 1 open issue in o/r; more match than --limit 1"; o.env.Summary != want {
		t.Errorf("summary %q, want %q", o.env.Summary, want)
	}
	if len(o.env.Next) != 1 || !slices.Equal(o.env.Next[0].Flags[len(o.env.Next[0].Flags)-2:], []string{"--limit", "2"}) {
		t.Errorf("next = %+v", o.env.Next)
	}
}

func TestListEmptyIsAnEmptyArray(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "")
	f.on("GET", "/repos/o/r/issues", 200, []any{})
	o := runIssue(t, "", "o/r", "--list", "--state", "closed")
	if o.code != 0 || o.env.Result.Issues == nil || !strings.Contains(o.stdout, `"issues": []`) {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestCreate(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("POST", "/repos/o/r/issues", 201, ghIssue(43, "open", "Crash", false))
	o := runIssue(t, "", "o/r", "--create", "--title", "Crash", "--body", "It crashes.", "--label", "bug", "--assignee", "alice")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	body := f.find(t, "POST", "/repos/o/r/issues").Body
	if body["title"] != "Crash" || body["body"] != "It crashes." {
		t.Errorf("payload %v", body)
	}
	if o.env.Result.Ref != "o/r#43" || o.env.Summary != `issue: opened o/r#43: "Crash"` {
		t.Errorf("ref %q summary %q", o.env.Result.Ref, o.env.Summary)
	}
	want := []envelope.SideEffect{{Action: "create_issue", Kind: "issue", Target: "o/r", URL: "https://github.com/o/r/issues/43", OK: true}}
	if !slices.Equal(o.env.SideEffects, want) {
		t.Errorf("side_effects = %+v", o.env.SideEffects)
	}
}

func TestWriteWithoutTokenIsAuth(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "")
	o := runIssue(t, "", "o/r", "--create", "--title", "x", "--dry-run")
	e := o.env.Error
	if o.code != 1 || e.Category != envelope.CategoryAuth || e.FixHint == nil || !slices.Equal(e.FixHint.Env, []string{"GITHUB_TOKEN", "GH_TOKEN"}) {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if !strings.Contains(e.Message, "no token for GitHub; set GITHUB_TOKEN or GH_TOKEN") {
		t.Errorf("message %q", e.Message)
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
}

func TestCommentFromStdin(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "t", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	f.on("POST", "/repos/o/r/issues/42/comments", 201, map[string]any{"html_url": "https://github.com/o/r/issues/42#issuecomment-9"})
	o := runIssue(t, "Thanks — fixed.\n", "o/r#42", "--comment", "--body-file", "-")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if got := f.find(t, "POST", "/repos/o/r/issues/42/comments").Body["body"]; got != "Thanks — fixed.\n" {
		t.Errorf("body sent %q", got)
	}
	if o.env.Result.CommentURL != "https://github.com/o/r/issues/42#issuecomment-9" || o.env.Summary != "issue: commented on o/r#42" {
		t.Errorf("comment_url %q summary %q", o.env.Result.CommentURL, o.env.Summary)
	}
	if o.env.Result.Comments != nil {
		t.Errorf("a write writes the comments of the thread: %s", o.stdout)
	}
}

func TestEdit(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "New title", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	f.on("PATCH", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "New title", false))
	f.on("POST", "/repos/o/r/issues/42/labels", 200, []any{})
	f.on("DELETE", "/repos/o/r/issues/42/labels/needs%20triage", 200, []any{})
	o := runIssue(t, "", "o/r#42", "--edit", "--title", "New title", "--add-label", "triaged", "--remove-label", "needs triage")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	want := []string{"PATCH /repos/o/r/issues/42", "POST /repos/o/r/issues/42/labels", "DELETE /repos/o/r/issues/42/labels/needs%20triage"}
	if got := f.writes(); !slices.Equal(got, want) {
		t.Errorf("writes %v, want %v", got, want)
	}
	if p := f.find(t, "PATCH", "/repos/o/r/issues/42").Body; p["title"] != "New title" || p["body"] != nil {
		t.Errorf("PATCH payload %v", p)
	}
	var actions []string
	for _, e := range o.env.SideEffects {
		actions = append(actions, e.Action)
	}
	if !slices.Equal(actions, []string{"edit", "add_labels", "remove_label"}) {
		t.Errorf("side_effects %+v", o.env.SideEffects)
	}
	if want := `issue: edited o/r#42 (edit title "New title"; add labels triaged; remove label needs triage)`; o.env.Summary != want {
		t.Errorf("summary %q\nwant    %q", o.env.Summary, want)
	}
	if o.env.Result.Issue == nil || o.env.Result.Issue.Title != "New title" {
		t.Errorf("issue not read back: %+v", o.env.Result.Issue)
	}
}

func TestCloseClosedIsNoChange(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "closed", "t", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	o := runIssue(t, "", "o/r#42", "--close")
	if o.code != 0 || !o.env.Result.NoChange || o.env.Summary != "issue: o/r#42 is already closed; nothing written" {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
	if w := f.writes(); len(w) != 0 {
		t.Errorf("writes %v", w)
	}
}

func TestCloseWithCommentDryRunThenReal(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "t", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	f.on("POST", "/repos/o/r/issues/42/comments", 201, map[string]any{"html_url": "u"})
	f.onFunc("PATCH", "/repos/o/r/issues/42", func(request) reply {
		f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "closed", "t", false))
		return reply{200, ghIssue(42, "closed", "t", false)}
	})

	dry := runIssue(t, "", "o/r#42", "--close", "--body", "Fixed in #43", "--dry-run")
	if dry.code != 0 || !dry.env.Result.DryRun || dry.env.SideEffects != nil {
		t.Fatalf("exit %d: %s", dry.code, dry.stdout)
	}
	var planned []string
	for _, p := range dry.env.Result.Planned {
		planned = append(planned, p.Action+" "+p.Target)
	}
	if !slices.Equal(planned, []string{"comment o/r#42", "close o/r#42"}) {
		t.Errorf("planned %v", planned)
	}
	if w := f.writes(); len(w) != 0 {
		t.Errorf("a dry run wrote %v", w)
	}
	if !strings.HasSuffix(dry.env.Summary, "(comment a body of 12 characters; close)") {
		t.Errorf("summary %q", dry.env.Summary)
	}
	if len(dry.env.Next) != 1 || dry.env.Next[0].Input != "o/r#42" || slices.Contains(dry.env.Next[0].Flags, "--dry-run") {
		t.Fatalf("next = %+v", dry.env.Next)
	}

	real := runIssue(t, "", append([]string{dry.env.Next[0].Input}, dry.env.Next[0].Flags...)...)
	if real.code != 0 || real.env.Summary != "issue: closed o/r#42, with a comment" {
		t.Fatalf("exit %d: %s", real.code, real.stdout)
	}
	if got := f.writes(); !slices.Equal(got, []string{"POST /repos/o/r/issues/42/comments", "PATCH /repos/o/r/issues/42"}) {
		t.Errorf("writes %v", got)
	}
	if real.env.Result.Issue.State != "closed" {
		t.Errorf("state read back %q", real.env.Result.Issue.State)
	}
}

func TestReopen(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "closed", "t", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	f.on("PATCH", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "t", false))
	o := runIssue(t, "", "o/r#42", "--reopen")
	if o.code != 0 || o.env.Summary != "issue: reopened o/r#42" {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "PATCH", "/repos/o/r/issues/42").Body; p["state"] != "open" {
		t.Errorf("payload %v", p)
	}
}

func TestMerge(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/7", 200, ghIssue(7, "open", "t", true))
	f.on("GET", "/repos/o/r/issues/7/comments", 200, []any{})
	f.on("PUT", "/repos/o/r/pulls/7/merge", 200, map[string]any{"merged": true, "sha": "deadbeefcafe", "message": "merged"})
	o := runIssue(t, "", "o/r!7", "--merge", "--method", "squash")
	if o.code != 0 || o.env.Summary != "issue: merged o/r!7 as deadbee" || !o.env.Result.Merge.Merged {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "PUT", "/repos/o/r/pulls/7/merge").Body; p["merge_method"] != "squash" {
		t.Errorf("payload %v", p)
	}
}

func TestMergeRefusedIsConflict(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/7", 200, ghIssue(7, "open", "t", true))
	f.on("GET", "/repos/o/r/issues/7/comments", 200, []any{})
	f.on("PUT", "/repos/o/r/pulls/7/merge", http.StatusMethodNotAllowed, map[string]any{"message": "Pull Request is not mergeable"})
	o := runIssue(t, "", "o/r#7", "--merge", "--method", "merge")
	e := o.env.Error
	if o.code != 1 || e.Stage != "write" || e.Category != envelope.CategoryConflict || e.Retryable {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	want := []envelope.SideEffect{{Action: "merge", Kind: "pull_request", Target: "o/r!7", OK: false}}
	if !slices.Equal(o.env.SideEffects, want) {
		t.Errorf("side_effects %+v", o.env.SideEffects)
	}
	if o.env.Next != nil {
		t.Errorf("a failure has a next: %+v", o.env.Next)
	}
}

func TestReviewOfAnIssueIsAUsageError(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/5", 200, ghIssue(5, "open", "t", false))
	f.on("GET", "/repos/o/r/issues/5/comments", 200, []any{})
	o := runIssue(t, "", "o/r#5", "--review", "--body", "lgtm")
	if o.code != 2 || !strings.Contains(o.env.Error.Message, "o/r#5 is an issue, not a pull request") {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
	if w := f.writes(); len(w) != 0 {
		t.Errorf("writes %v", w)
	}
}

func TestReview(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/7", 200, ghIssue(7, "open", "t", true))
	f.on("GET", "/repos/o/r/issues/7/comments", 200, []any{})
	f.on("POST", "/repos/o/r/pulls/7/reviews", 200, map[string]any{})
	o := runIssue(t, "", "o/r!7", "--review", "--body", "lgtm")
	if o.code != 0 || o.env.Summary != "issue: reviewed o/r!7" {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "POST", "/repos/o/r/pulls/7/reviews").Body; p["event"] != "COMMENT" || p["body"] != "lgtm" {
		t.Errorf("payload %v", p)
	}
}

func TestCreateLabelAndPR(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("POST", "/repos/o/r/labels", 201, map[string]any{"name": "triaged"})
	f.on("POST", "/repos/o/r/pulls", 201, ghPull(44))

	o := runIssue(t, "", "o/r", "--create-label", "triaged", "--color", "#00ff00", "--description", "seen")
	if o.code != 0 || o.env.Result.Label.Color != "00ff00" || o.env.Summary != `issue: label "triaged" in o/r` {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "POST", "/repos/o/r/labels").Body; p["name"] != "triaged" || p["color"] != "00ff00" || p["description"] != "seen" {
		t.Errorf("label payload %v", p)
	}

	o = runIssue(t, "", "o/r", "--create-pr", "--head", "fix", "--base", "main", "--title", "Fix the crash", "--draft")
	if o.code != 0 || o.env.Result.Ref != "o/r!44" || o.env.Result.PullRequest == nil {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "POST", "/repos/o/r/pulls").Body; p["head"] != "fix" || p["base"] != "main" || p["draft"] != true {
		t.Errorf("pull payload %v", p)
	}
	if len(o.env.Next) != 1 || o.env.Next[0].Input != "https://github.com/o/r/pull/44" || !slices.Equal(o.env.Next[0].Flags, []string{"--checks", "--reviews"}) {
		t.Errorf("next %+v", o.env.Next)
	}
}

func TestErrorCategories(t *testing.T) {
	for _, tc := range []struct {
		status    int
		headers   string
		category  string
		retryable bool
	}{
		{http.StatusNotFound, "", envelope.CategoryInput, false},
		{http.StatusUnauthorized, "", envelope.CategoryAuth, false},
		{http.StatusUnprocessableEntity, "", envelope.CategoryInput, false},
		{http.StatusBadGateway, "", envelope.CategoryAPI, true},
	} {
		f := newFakeForge(t)
		githubEnv(t, f, "tok")
		f.on("GET", "/repos/o/r/issues/1", tc.status, map[string]any{"message": "nope"})
		o := runIssue(t, "", "o/r#1")
		e := o.env.Error
		if o.code != 1 || e.Stage != "read" || e.Category != tc.category || e.Retryable != tc.retryable {
			t.Errorf("status %d: exit %d, error %+v", tc.status, o.code, e)
		}
	}
}

func TestRateLimitIsRetryable(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.srv.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "600") // longer than issuex waits
		w.WriteHeader(http.StatusTooManyRequests)
	})
	o := runIssue(t, "", "o/r#1")
	if e := o.env.Error; o.code != 1 || e.Category != envelope.CategoryAPI || !e.Retryable {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestBodyTooLong(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	o := runIssue(t, "", "o/r#1", "--comment", "--body", strings.Repeat("x", 65537))
	if o.code != 2 || !strings.Contains(o.env.Error.Message, "the body has 65537 characters, and GitHub takes at most 65536") {
		t.Errorf("exit %d: %s", o.code, o.env.Summary)
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
}

func TestShortRefUsesOrigin(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	f.on("GET", "/repos/o/r/issues/42", 200, ghIssue(42, "open", "t", false))
	f.on("GET", "/repos/o/r/issues/42/comments", 200, []any{})
	dir := gitRepo(t, f.srv.URL+"/o/r.git")
	o := runIssue(t, "", "42", "--dir", dir)
	if o.code != 0 || o.env.Result.Ref != "o/r#42" || o.env.Result.Host == "" {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestNoOriginIsAUsageError(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	o := runIssue(t, "", "42")
	if o.code != 2 || o.env.Error.FixHint == nil || o.env.Error.FixHint.Flag != "--dir" {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestForgeFlagMustAgreeWithTheHost(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	o := runIssue(t, "", "https://github.com/o/r/issues/1", "--forge", "gitlab")
	if o.code != 2 || !strings.Contains(o.env.Error.Message, "--forge gitlab, but github.com is on github") {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
}

func TestForgeFlagDecidesWhenBothAreConfigured(t *testing.T) {
	f := newFakeForge(t)
	githubEnv(t, f, "tok")
	t.Setenv("GITLAB_TOKEN", "other") // both forges configured: issuex cannot tell
	f.on("GET", "/repos/o/r", 200, map[string]any{"full_name": "o/r", "default_branch": "main"})

	o := runIssue(t, "", "o/r")
	if o.code != 1 || o.env.Error.Stage != "forge" || o.env.Error.FixHint == nil || o.env.Error.FixHint.Flag != "--forge" {
		t.Fatalf("without --forge: exit %d: %s", o.code, o.stdout)
	}
	o = runIssue(t, "", "o/r", "--forge", "github")
	if o.code != 0 || o.env.Result.Forge != "github" {
		t.Errorf("with --forge: exit %d: %s", o.code, o.stdout)
	}
}
