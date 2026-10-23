package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/agent-fox-dev/aflib/envelope"
)

func glIssue(iid int, state string) map[string]any {
	return map[string]any{
		"id": 100 + iid, "iid": iid, "title": "Slow start", "description": "d", "state": state,
		"web_url": "https://gitlab.example/g/s/p/-/issues/" + itoa(iid),
		"author":  map[string]any{"username": "alice"}, "labels": []string{"perf"},
		"created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-02T10:00:00Z",
	}
}

func glMR(iid int, state string) map[string]any {
	return map[string]any{
		"id": 200 + iid, "iid": iid, "title": "Speed up start", "description": "d", "state": state,
		"source_branch": "speed", "target_branch": "main", "sha": "f00d",
		"web_url":    "https://gitlab.example/g/s/p/-/merge_requests/" + itoa(iid),
		"author":     map[string]any{"username": "bob"},
		"created_at": "2026-10-01T10:00:00Z", "updated_at": "2026-10-02T10:00:00Z",
	}
}

const glProject = "/api/v4/projects/g%2Fs%2Fp"

func TestGitLabReadMergeRequest(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "tok")
	f.on("GET", glProject+"/merge_requests/5", 200, glMR(5, "opened"))
	f.on("GET", glProject+"/merge_requests/5/notes", 200, []any{
		map[string]any{"id": 1, "body": "nice", "author": map[string]any{"username": "carol"}},
		map[string]any{"id": 2, "body": "changed the description", "system": true},
	})
	o := runIssue(t, "", "g/s/p!5")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	res := o.env.Result
	if res.Forge != "gitlab" || res.Repo != "g/s/p" || res.Ref != "g/s/p!5" {
		t.Errorf("forge %q repo %q ref %q", res.Forge, res.Repo, res.Ref)
	}
	if res.PullRequest == nil || res.PullRequest.HeadBranch != "speed" || res.Issue.State != "open" || len(res.Comments) != 1 {
		t.Errorf("pull request %+v issue %+v comments %v", res.PullRequest, res.Issue, res.Comments)
	}
}

func TestGitLabIssueIsNotAMergeRequest(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "tok")
	o := runIssue(t, "", "g/s/p#5", "--merge")
	if o.code != 2 || !strings.Contains(o.env.Error.Message, "on GitLab g/s/p#5 is an issue: a merge request is g/s/p!5") {
		t.Errorf("exit %d: %s", o.code, o.stdout)
	}
	if got := f.requests(); len(got) != 0 {
		t.Errorf("sent %v", got)
	}
}

func TestGitLabCreateIgnoresAssignees(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "tok")
	f.on("POST", glProject+"/issues", 201, glIssue(9, "opened"))
	o := runIssue(t, "", "g/s/p", "--create", "--title", "Slow start", "--assignee", "alice")
	if o.code != 0 || o.env.Result.Ref != "g/s/p#9" {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if len(o.env.Warnings) != 1 || o.env.Warnings[0].Code != envelope.WarnAssigneesIgnored || o.env.Warnings[0].Severity != envelope.SeverityHigh {
		t.Errorf("warnings %+v", o.env.Warnings)
	}
	if !strings.HasSuffix(o.env.Summary, "; 1 high-severity warning") {
		t.Errorf("summary %q", o.env.Summary)
	}
	if p := f.find(t, "POST", glProject+"/issues").Body; p["description"] != "" || p["assignees"] != nil {
		t.Errorf("payload %v", p)
	}
}

func TestGitLabCloseMergeRequest(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "tok")
	f.on("GET", glProject+"/merge_requests/5", 200, glMR(5, "opened"))
	f.on("GET", glProject+"/merge_requests/5/notes", 200, []any{})
	f.on("PUT", glProject+"/merge_requests/5", 200, glMR(5, "closed"))
	o := runIssue(t, "", "g/s/p!5", "--close")
	if o.code != 0 || o.env.Summary != "issue: closed g/s/p!5" {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "PUT", glProject+"/merge_requests/5").Body; p["state_event"] != "close" {
		t.Errorf("payload %v", p)
	}
	if o.env.SideEffects[0].Kind != "pull_request" {
		t.Errorf("side_effects %+v", o.env.SideEffects)
	}
}

func TestGitLabCreateLabelDefaultsTheColour(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "tok")
	f.on("POST", glProject+"/labels", 201, map[string]any{"name": "perf"})
	o := runIssue(t, "", "g/s/p", "--create-label", "perf")
	if o.code != 0 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	if p := f.find(t, "POST", glProject+"/labels").Body; p["color"] != "#ededed" {
		t.Errorf("payload %v", p)
	}
}

func TestGitLabList(t *testing.T) {
	f := newFakeForge(t)
	gitlabEnv(t, f, "")
	f.on("GET", glProject+"/issues", 200, []any{glIssue(2, "opened"), glIssue(1, "opened")})
	o := runIssue(t, "", "g/s/p", "--list", "--sort", "updated", "--direction", "asc")
	if o.code != 0 || len(o.env.Result.Issues) != 2 {
		t.Fatalf("exit %d: %s", o.code, o.stdout)
	}
	q := f.find(t, "GET", glProject+"/issues").Query
	for _, want := range []string{"state=opened", "order_by=updated_at", "sort=asc"} {
		if !strings.Contains(q, want) {
			t.Errorf("query %q has no %q", q, want)
		}
	}
	if got := []int{o.env.Result.Issues[0].Number, o.env.Result.Issues[1].Number}; !slices.Equal(got, []int{2, 1}) {
		t.Errorf("issues %v", got)
	}
}
