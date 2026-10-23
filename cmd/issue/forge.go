package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/agent-fox-dev/aflib/envelope"
	"github.com/agent-fox-dev/aflib/issuex"
)

// connect resolves the repository the call works on and builds the client
// for its forge. A target that names no repository takes the origin remote's
// of --dir. --forge decides the adapter only where issuex cannot: for a
// repository with no host. A host issuex cannot tell stays an error, since a
// token must not be sent to a host it was not configured for.
func (c *call) connect(t target) *envelope.ErrorInfo {
	dir, err := filepath.Abs(c.opts.dir)
	if err != nil {
		return usageInfo(fmt.Errorf("resolving --dir %q: %w", c.opts.dir, err), nil)
	}
	if info, serr := os.Stat(dir); serr != nil || !info.IsDir() {
		return usageInfo(fmt.Errorf("--dir %s is not a directory", dir), &envelope.FixHint{Flag: "--dir"})
	}
	repo := t.repo
	if !repo.Valid() {
		r, ok := issuex.DetectRepo(dir)
		if !ok {
			return usageInfo(fmt.Errorf("no repository: the input names none, and %s has no origin remote on a forge "+
				"this command can tell (a self-hosted one is named with GITHUB_API_URL or GITLAB_API_URL); "+
				"give owner/repo or a URL", dir), &envelope.FixHint{Flag: "--dir"})
		}
		repo = r
	}
	c.repo = repo

	opts := issuex.Options{Repo: repo, Dir: dir, UserAgent: tool + "/" + version}
	forge := issuex.ForgeType(c.opts.forge)
	switch {
	case forge != "" && repo.Host == "":
		if forge == issuex.ForgeTypeGitLab {
			c.client, err = issuex.NewGitLab(opts)
		} else {
			c.client, err = issuex.NewGitHub(opts)
		}
	default:
		var detected issuex.ForgeType
		detected, err = issuex.DetectForge(opts)
		if err == nil && forge != "" && detected != forge {
			return usageInfo(fmt.Errorf("--forge %s, but %s is on %s", forge, repo.Host, detected),
				&envelope.FixHint{Flag: "--forge", Valid: []string{string(detected)}})
		}
		forge = detected
		if err == nil {
			c.client, err = issuex.NewWithOptions(opts)
		}
	}
	if err != nil {
		return c.classify("forge", fmt.Sprintf("choosing the forge of %s", repo), err)
	}
	c.forge = forge
	return nil
}

// tokenEnv names the variables a forge's token is read from.
func tokenEnv(forge issuex.ForgeType) []string {
	switch forge {
	case issuex.ForgeTypeGitHub:
		return []string{"GITHUB_TOKEN", "GH_TOKEN"}
	case issuex.ForgeTypeGitLab:
		return []string{"GITLAB_TOKEN"}
	}
	return []string{"GITHUB_TOKEN", "GH_TOKEN", "GITLAB_TOKEN"}
}

// classify turns an error of issuex into the envelope's error: the stage the
// call was at, a category that says whether running it again could help, and
// a fix hint where one is mechanical. what says what the call was doing.
func (c *call) classify(stage, what string, err error) *envelope.ErrorInfo {
	info := (&envelope.ErrorInfo{Stage: stage, Message: what + ": " + err.Error()}).WithCause(err)
	var he *issuex.HTTPError
	hasHTTP := errors.As(err, &he)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		info.Category = envelope.CategoryAborted
		info.Message = fmt.Sprintf("%s: the call did not finish within --timeout %s", what, c.opts.timeout)
		info.FixHint = &envelope.FixHint{Flag: "--timeout"}
	case errors.Is(err, context.Canceled):
		info.Category = envelope.CategoryAborted
	case errors.Is(err, issuex.ErrNoToken):
		info.Category = envelope.CategoryAuth
		info.Message = fmt.Sprintf("%s: no token for %s; set %s", what, forgeName(c.forge), orList(tokenEnv(c.forge)))
		info.FixHint = &envelope.FixHint{Env: tokenEnv(c.forge)}
	case errors.Is(err, issuex.ErrAmbiguousForge), errors.Is(err, issuex.ErrUnsupportedForge):
		info.Category = envelope.CategoryInput
		info.FixHint = &envelope.FixHint{Flag: "--forge", Valid: forges, Env: []string{"GITHUB_API_URL", "GITLAB_API_URL"}}
	case issuex.IsRateLimited(err):
		info.Category = envelope.CategoryAPI
	case issuex.IsNotFound(err):
		info.Category = envelope.CategoryInput
	case issuex.IsConflict(err):
		info.Category = envelope.CategoryConflict
	case hasHTTP && (he.Status == http.StatusUnauthorized || he.Status == http.StatusForbidden):
		info.Category = envelope.CategoryAuth
		info.FixHint = &envelope.FixHint{Env: tokenEnv(c.forge)}
	case hasHTTP && he.Status >= 500:
		info.Category = envelope.CategoryAPI
	case hasHTTP:
		info.Category = envelope.CategoryInput
	default:
		// A transport failure, or a response that could not be decoded.
		info.Category = envelope.CategoryAPI
	}
	return info
}

func forgeName(f issuex.ForgeType) string {
	switch f {
	case issuex.ForgeTypeGitHub:
		return "GitHub"
	case issuex.ForgeTypeGitLab:
		return "GitLab"
	}
	return "the forge"
}

// orList joins items as "a, b or c".
func orList(items []string) string {
	switch len(items) {
	case 0:
		return ""
	case 1:
		return items[0]
	}
	out := items[0]
	for _, it := range items[1 : len(items)-1] {
		out += ", " + it
	}
	return out + " or " + items[len(items)-1]
}
