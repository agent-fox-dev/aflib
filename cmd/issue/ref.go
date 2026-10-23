package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/agent-fox-dev/aflib/issuex"
)

// target is what the one positional input names: a thread when number is set,
// a repository otherwise. A target whose repository is not valid takes the
// repository of the origin remote of --dir.
type target struct {
	repo   issuex.Repo
	number int
	// pr is true when the input says it is a pull or merge request: a URL of
	// one, or the marker !. On GitHub a #N may be one too; the forge tells.
	pr bool
}

// parseRef reads the positional input. It sends nothing and runs nothing: a
// short reference (N, #N, !N) is completed with the origin remote later.
func parseRef(s string) (target, error) {
	s = strings.TrimSpace(s)
	switch {
	case s == "":
		return target{}, nil
	case s == "-":
		return target{}, fmt.Errorf("- is not an input: the input is a reference; --body-file - reads a body from stdin")
	case strings.ContainsAny(s, " \t\r\n"):
		return target{}, fmt.Errorf("%q is not a reference: a reference has no spaces", s)
	}

	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.Contains(s, "://") || isSCP(s) {
		if ref, ok := issuex.ParseIssueURL(s); ok {
			return target{repo: ref.Repo, number: ref.Number, pr: ref.IsPullRequest}, nil
		}
		if repo, ok := issuex.ParseRemote(s); ok {
			return target{repo: repo}, nil
		}
		return target{}, fmt.Errorf("%q is not the URL of an issue, a pull or merge request or a repository on a forge "+
			"this command can tell: a self-hosted forge is named with GITHUB_API_URL or GITLAB_API_URL", s)
	}

	if i := strings.LastIndexAny(s, "#!"); i >= 0 {
		n, err := strconv.Atoi(s[i+1:])
		if err != nil || n <= 0 {
			return target{}, fmt.Errorf("%q: %q is not an issue or pull request number", s, s[i+1:])
		}
		t := target{number: n, pr: s[i] == '!'}
		if i == 0 {
			return t, nil
		}
		repo, ok := issuex.ParseRepo(s[:i])
		if !ok {
			return target{}, fmt.Errorf("%q: %q is not a repository (owner/repo)", s, s[:i])
		}
		t.repo = repo
		return t, nil
	}
	if n, err := strconv.Atoi(s); err == nil {
		if n <= 0 {
			return target{}, fmt.Errorf("%q is not an issue or pull request number", s)
		}
		return target{number: n}, nil
	}
	if repo, ok := issuex.ParseRepo(s); ok {
		return target{repo: repo}, nil
	}
	return target{}, fmt.Errorf("%q is not a reference: give a URL, owner/repo, owner/repo#N, owner/repo!N or N", s)
}

// isSCP reports whether s is a git remote in the scp-like form
// [user@]host:path, which has a colon before its first slash.
func isSCP(s string) bool {
	colon := strings.IndexByte(s, ':')
	slash := strings.IndexByte(s, '/')
	return colon > 0 && (slash < 0 || colon < slash)
}

// refString names a thread as the next call takes it: owner/repo#N for an
// issue, owner/repo!N for a pull or merge request.
func refString(repo issuex.Repo, number int, pr bool) string {
	marker := "#"
	if pr {
		marker = "!"
	}
	return repo.String() + marker + strconv.Itoa(number)
}
