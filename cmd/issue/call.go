package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/agent-fox-dev/aflib/envelope"
	"github.com/agent-fox-dev/aflib/issuex"
)

// call is one invocation after its flags were checked: what it was given,
// the forge it talks to once connected, and what it has to report.
type call struct {
	opts     *options
	run      *run
	input    string
	flagArgs []string
	stdin    io.Reader

	ctx    context.Context
	repo   issuex.Repo
	forge  issuex.ForgeType
	client issuex.Client

	result  *envelope.IssueResult
	summary string
	next    []envelope.Next
}

// step is one write a form makes: named as its side effect, and done by do,
// which returns the web URL of what it made, if any.
type step struct {
	envelope.PlannedWrite
	do func() (string, error)
}

// execute runs the form the flags name. It returns the failure, if any; the
// result, the summary and next are left on c.
func (c *call) execute() *envelope.ErrorInfo {
	t, err := parseRef(c.input)
	if err != nil {
		return usageInfo(err, nil) // check has refused this already
	}
	f := c.opts.form()
	body, failure := c.readBody()
	if failure != nil {
		return failure
	}

	ctx, cancel := context.WithTimeout(context.Background(), c.opts.timeout)
	defer cancel()
	c.ctx = ctx
	if failure := c.connect(t); failure != nil {
		return failure
	}
	defer c.client.Close()
	c.result = &envelope.IssueResult{Forge: string(c.forge), Repo: c.repo.String(), Host: c.repo.Host}

	if limit := issuex.MaxBodyLength(c.client); utf8.RuneCountInString(body) > limit {
		return usageInfo(fmt.Errorf("the body has %d characters, and %s takes at most %d",
			utf8.RuneCountInString(body), forgeName(c.forge), limit), nil)
	}

	ref := issuex.IssueRef{Repo: c.repo, Number: t.number, IsPullRequest: t.pr}
	if c.forge == issuex.ForgeTypeGitLab && t.number != 0 && !t.pr && needsPR(f, c.opts) {
		return usageInfo(fmt.Errorf("%s works on a merge request, and on GitLab %s is an issue: a merge request is %s",
			formOrFlags(f), refString(c.repo, t.number, false), refString(c.repo, t.number, true)), nil)
	}

	switch f {
	case formRead:
		if t.number == 0 {
			return c.readRepository()
		}
		return c.readThread(ref)
	case formList:
		return c.list()
	case formCreate:
		return c.createIssue(body)
	case formCreateLabel:
		return c.createLabel()
	case formCreatePR:
		return c.createPR(body)
	}
	return c.writeThread(f, ref, body)
}

// needsPR reports whether a call works on a pull request only.
func needsPR(f form, o *options) bool {
	return f == formReview || f == formMerge || (f == formRead && (o.checks || o.reviews || o.files))
}

func formOrFlags(f form) string {
	if f == formRead {
		return "--checks, --reviews and --files"
	}
	return f.flagName()
}

// readBody reads the body from --body or --body-file, before anything is sent.
func (c *call) readBody() (string, *envelope.ErrorInfo) {
	o := c.opts
	if !o.given["body-file"] {
		return o.body, nil
	}
	var b []byte
	var err error
	if o.bodyFile == "-" {
		if c.stdin == nil {
			return "", usageInfo(errors.New("--body-file -: there is no stdin to read"), nil)
		}
		b, err = io.ReadAll(c.stdin)
	} else {
		b, err = os.ReadFile(o.bodyFile)
	}
	if err != nil {
		return "", usageInfo(fmt.Errorf("--body-file: %w", err), &envelope.FixHint{Flag: "--body-file"})
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", usageInfo(fmt.Errorf("--body-file %s is empty", o.bodyFile), &envelope.FixHint{Flag: "--body-file"})
	}
	return string(b), nil
}

// readRepository is the read of a repository.
func (c *call) readRepository() *envelope.ErrorInfo {
	repo, err := c.client.GetRepository(c.ctx, c.repo)
	if err != nil {
		return c.classify("read", "reading "+c.repo.String(), err)
	}
	c.result.Repository = &repo
	c.summary = fmt.Sprintf("issue: %s on %s", c.repo, c.forge)
	if repo.DefaultBranch != "" {
		c.summary += ", default branch " + repo.DefaultBranch
	}
	return nil
}

// readThread is the read of an issue or pull request: the thread, and for a
// pull request its state and what the read flags ask for.
func (c *call) readThread(ref issuex.IssueRef) *envelope.ErrorInfo {
	th, isPR, failure := c.fetchThread(ref)
	if failure != nil {
		return failure
	}
	ref.IsPullRequest = isPR
	res := c.result
	res.Comments = th.Comments
	if res.Comments == nil && th.CommentsErr == nil {
		res.Comments = []issuex.Comment{}
	}
	if th.CommentsErr != nil {
		c.run.warn(envelope.WarnCommentsUnreadable, envelope.SeverityHigh, "read",
			"the comments of %s could not be read (%v); the result has none", res.Ref, th.CommentsErr)
	}
	if th.Truncated {
		res.CommentsTruncated = true
		c.run.warn(envelope.WarnCommentsTruncated, envelope.SeverityLow, "read",
			"%s has more comments than the %d read", res.Ref, len(th.Comments))
	}
	o := c.opts
	if !isPR {
		if o.checks || o.reviews || o.files {
			c.result = nil
			return usageInfo(fmt.Errorf("%s is an issue, not a pull request: --checks, --reviews and --files read a pull request", res.Ref), nil)
		}
		c.summary = fmt.Sprintf("issue: %s %s: %s%s", res.Ref, th.Issue.State, quote(th.Issue.Title), commentCount(res.Comments, th.CommentsErr))
		return nil
	}

	pr, err := c.client.ReadPullRequest(c.ctx, ref)
	if err != nil {
		return c.classify("read", "reading the pull request "+res.Ref, err)
	}
	res.PullRequest = &pr
	var extra []string
	if o.checks {
		checks, err := c.client.GetCIChecks(c.ctx, ref)
		if err != nil {
			return c.classify("read", "reading the CI checks of "+res.Ref, err)
		}
		res.Checks = nonNil(checks)
		extra = append(extra, "checks: "+checkCounts(checks))
	}
	if o.reviews {
		reviews, err := c.client.GetPRReviews(c.ctx, ref)
		if err != nil {
			return c.classify("read", "reading the reviews of "+res.Ref, err)
		}
		res.Reviews = nonNil(reviews)
		extra = append(extra, fmt.Sprintf("%d %s", len(reviews), plural(len(reviews), "review", "reviews")))
	}
	if o.files {
		files, err := c.client.ReadChangedFiles(c.ctx, ref)
		if err != nil {
			return c.classify("read", "reading the changed files of "+res.Ref, err)
		}
		res.Files = nonNil(files)
		extra = append(extra, fmt.Sprintf("%d %s changed", len(files), plural(len(files), "file", "files")))
	}
	c.summary = fmt.Sprintf("issue: %s %s pull request %s → %s: %s%s", res.Ref, prState(pr), pr.HeadBranch, pr.BaseBranch,
		quote(pr.Title), commentCount(res.Comments, th.CommentsErr))
	if len(extra) > 0 {
		c.summary += "; " + strings.Join(extra, "; ")
	}
	return nil
}

// fetchThread reads a thread and tells whether it is a pull request. It sets
// the result's Ref and Issue.
func (c *call) fetchThread(ref issuex.IssueRef) (issuex.IssueThread, bool, *envelope.ErrorInfo) {
	th, err := c.client.ReadIssue(c.ctx, ref)
	if err != nil {
		return th, false, c.classify("read", "reading "+refString(ref.Repo, ref.Number, ref.IsPullRequest), err)
	}
	// GitHub says which of its threads are pull requests; GitLab numbers
	// them apart, so the input said.
	isPR := th.Issue.IsPR
	if c.forge == issuex.ForgeTypeGitLab {
		isPR = ref.IsPullRequest
	}
	issue := th.Issue
	c.result.Ref = refString(ref.Repo, ref.Number, isPR)
	c.result.Issue = &issue
	return th, isPR, nil
}

// list is the list form.
func (c *call) list() *envelope.ErrorInfo {
	o := c.opts
	filter := issuex.IssueFilter{State: o.state, Labels: o.labels, Sort: o.sort, Direction: o.direction, Limit: o.limit}
	if len(o.assignees) == 1 {
		filter.Assignee = o.assignees[0]
	}
	list, err := c.client.ListIssues(c.ctx, c.repo, filter)
	if err != nil {
		return c.classify("read", "listing the issues of "+c.repo.String(), err)
	}
	c.result.Issues = nonNil(list.Issues)
	c.result.Incomplete = list.Incomplete
	state := o.state + " "
	if o.state == "all" {
		state = ""
	}
	n := len(list.Issues)
	c.summary = fmt.Sprintf("issue: %d %s%s in %s", n, state, plural(n, "issue", "issues"), c.repo)
	if list.Incomplete {
		c.summary += fmt.Sprintf("; more match than --limit %d", o.limit)
		c.next = append(c.next, envelope.Next{Tool: tool, Input: c.input,
			Flags: withValue(c.flagArgs, "limit", strconv.Itoa(o.limit*2)),
			Why:   "more issues match than were listed; the same call with a higher --limit lists them"})
	}
	return nil
}

// requireToken refuses a write form without a token before anything is read,
// as the write itself would, so a dry run is refused the same way.
func (c *call) requireToken() *envelope.ErrorInfo {
	if c.client.Authenticated() {
		return nil
	}
	return c.classify("write", "writing to "+c.repo.String(), issuex.ErrNoToken)
}

// checkRepository reads the repository under --dry-run, so that a dry run
// of a form that writes to a repository says whether it exists.
func (c *call) checkRepository() *envelope.ErrorInfo {
	if !c.opts.dryRun {
		return nil
	}
	if _, err := c.client.GetRepository(c.ctx, c.repo); err != nil {
		return c.classify("read", "reading "+c.repo.String(), err)
	}
	return nil
}

// createIssue is the create form.
func (c *call) createIssue(body string) *envelope.ErrorInfo {
	if f := c.requireToken(); f != nil {
		return f
	}
	if f := c.checkRepository(); f != nil {
		return f
	}
	o := c.opts
	assignees := c.assigneesFor("--create")
	s := step{PlannedWrite: envelope.PlannedWrite{Action: "create_issue", Kind: "issue", Target: c.repo.String(),
		Detail: detail("title "+quote(o.title), withBody(body), labelsDetail(o.labels), assigneesDetail(assignees))}}
	s.do = func() (string, error) {
		iss, err := c.client.CreateIssue(c.ctx, c.repo, issuex.CreateIssueRequest{Title: o.title, Body: body, Labels: o.labels, Assignees: assignees})
		if err != nil {
			return "", err
		}
		c.result.Issue = &iss
		c.result.Ref = refString(c.repo, iss.Number, false)
		return iss.HTMLURL, nil
	}
	if f := c.runSteps([]step{s}); f != nil {
		return f
	}
	if o.dryRun {
		c.summary = fmt.Sprintf("issue: would open an issue in %s: %s", c.repo, quote(o.title))
	} else {
		c.summary = fmt.Sprintf("issue: opened %s: %s", c.result.Ref, quote(o.title))
	}
	return nil
}

// createLabel is the create-label form.
func (c *call) createLabel() *envelope.ErrorInfo {
	if f := c.requireToken(); f != nil {
		return f
	}
	if f := c.checkRepository(); f != nil {
		return f
	}
	o := c.opts
	label := issuex.Label{Name: strings.TrimSpace(o.createLabel), Color: strings.TrimPrefix(o.color, "#"), Description: o.description}
	if label.Color == "" && c.forge == issuex.ForgeTypeGitLab {
		label.Color = "ededed" // GitLab requires a colour
	}
	s := step{PlannedWrite: envelope.PlannedWrite{Action: "create_label", Kind: "repository", Target: c.repo.String(),
		Detail: detail("label "+quote(label.Name), prefixed("color ", label.Color), prefixed("description ", quoteIf(label.Description)))}}
	s.do = func() (string, error) { return "", c.client.CreateLabel(c.ctx, c.repo, label) }
	if f := c.runSteps([]step{s}); f != nil {
		return f
	}
	c.result.Label = &label
	c.summary = fmt.Sprintf("issue: label %s in %s", quote(label.Name), c.repo)
	if o.dryRun {
		c.summary = fmt.Sprintf("issue: would create the label %s in %s", quote(label.Name), c.repo)
	}
	return nil
}

// createPR is the create-pr form.
func (c *call) createPR(body string) *envelope.ErrorInfo {
	if f := c.requireToken(); f != nil {
		return f
	}
	if f := c.checkRepository(); f != nil {
		return f
	}
	o := c.opts
	req := issuex.CreatePullRequestRequest{Title: o.title, Body: body, Head: o.head, Base: o.base, Draft: o.draft}
	draft := ""
	if o.draft {
		draft = "draft"
	}
	s := step{PlannedWrite: envelope.PlannedWrite{Action: "create_pull_request", Kind: "pull_request", Target: c.repo.String(),
		Detail: detail(o.head+" → "+o.base, "title "+quote(o.title), withBody(body), draft)}}
	s.do = func() (string, error) {
		pr, err := c.client.CreatePullRequest(c.ctx, c.repo, req)
		if err != nil {
			return "", err
		}
		c.result.PullRequest = &pr
		c.result.Ref = refString(c.repo, pr.Number, true)
		return pr.HTMLURL, nil
	}
	if f := c.runSteps([]step{s}); f != nil {
		return f
	}
	if o.dryRun {
		c.summary = fmt.Sprintf("issue: would open a pull request in %s, %s → %s: %s", c.repo, o.head, o.base, quote(o.title))
		return nil
	}
	c.summary = fmt.Sprintf("issue: opened %s, %s → %s: %s", c.result.Ref, o.head, o.base, quote(o.title))
	input := c.result.Ref
	if c.result.PullRequest.HTMLURL != "" {
		input = c.result.PullRequest.HTMLURL
	}
	c.next = append(c.next, envelope.Next{Tool: tool, Input: input, Flags: []string{"--checks", "--reviews"},
		Why: "read the pull request with its CI checks and reviews"})
	return nil
}

// writeThread runs the forms that write to an existing thread: comment, edit,
// close, reopen, review and merge. Each reads the thread first, so that it
// knows whether the thread is a pull request and in which state it is.
func (c *call) writeThread(f form, ref issuex.IssueRef, body string) *envelope.ErrorInfo {
	if failure := c.requireToken(); failure != nil {
		return failure
	}
	th, isPR, failure := c.fetchThread(ref)
	if failure != nil {
		return failure
	}
	ref.IsPullRequest = isPR
	name := c.result.Ref
	if (f == formReview || f == formMerge) && !isPR {
		c.result = nil
		return usageInfo(fmt.Errorf("%s is an issue, not a pull request: %s works on a pull request", name, f.flagName()), nil)
	}
	kind := "issue"
	if isPR {
		kind = "pull_request"
	}
	o := c.opts
	planned := func(action, detail string, do func() (string, error)) step {
		return step{PlannedWrite: envelope.PlannedWrite{Action: action, Kind: kind, Target: name, Detail: detail}, do: do}
	}
	comment := func() step {
		return planned("comment", withBody(body), func() (string, error) {
			url, err := c.client.AddComment(c.ctx, ref, body)
			if err == nil {
				c.result.CommentURL = url
			}
			return url, err
		})
	}

	var steps []step
	var verb string
	readBack := true
	switch f {
	case formComment:
		steps, verb, readBack = []step{comment()}, "commented on", false
	case formEdit:
		steps = c.editSteps(ref, planned, body)
		verb = "edited"
	case formClose:
		if th.Issue.State != "open" {
			return c.noChange(name, th.Issue.State)
		}
		if body != "" {
			steps = append(steps, comment())
		}
		steps = append(steps, planned("close", "", func() (string, error) {
			if isPR {
				return "", c.client.ClosePullRequest(c.ctx, ref)
			}
			return "", c.client.CloseIssue(c.ctx, ref, "")
		}))
		verb = "closed"
	case formReopen:
		if th.Issue.State == "open" {
			return c.noChange(name, "open")
		}
		steps = []step{planned("reopen", "", func() (string, error) {
			_, err := c.client.UpdateIssue(c.ctx, ref, issuex.UpdateIssueRequest{State: "open"})
			return "", err
		})}
		verb = "reopened"
	case formReview:
		steps, verb, readBack = []step{planned("review", withBody(body), func() (string, error) {
			return "", c.client.PostReviewComment(c.ctx, ref, body)
		})}, "reviewed", false
	case formMerge:
		steps, verb, readBack = []step{c.mergeStep(ref, planned)}, "merged", false
	}

	if len(steps) == 0 {
		// An edit whose only change the client cannot send.
		c.result.NoChange = true
		c.summary = fmt.Sprintf("issue: %s not changed: nothing the client can send", name)
		return nil
	}
	if failure := c.runSteps(steps); failure != nil {
		return failure
	}
	if o.dryRun {
		c.summary = fmt.Sprintf("issue: would %s %s (%s)", presentTense(verb), name, actions(steps))
		return nil
	}
	c.summary = fmt.Sprintf("issue: %s %s", verb, name)
	switch {
	case f == formEdit:
		c.summary += " (" + actions(steps) + ")"
	case f == formClose && body != "":
		c.summary += ", with a comment"
	case f == formMerge && c.result.Merge != nil && c.result.Merge.SHA != "":
		c.summary += " as " + shortSHA(c.result.Merge.SHA)
	}
	if readBack {
		after, err := c.client.ReadIssue(c.ctx, ref)
		if err != nil {
			return c.classify("read", "reading "+name+" back after the write", err)
		}
		c.result.Issue = &after.Issue
	}
	return nil
}

// editSteps are the writes of --edit, in the order they are made: the fields
// UpdateIssue sends, the labels added, each label removed.
func (c *call) editSteps(ref issuex.IssueRef, planned func(string, string, func() (string, error)) step, body string) []step {
	o := c.opts
	var steps []step
	req := issuex.UpdateIssueRequest{Title: o.title, Body: body, Assignees: c.assigneesFor("--edit")}
	var fields []string
	if req.Title != "" {
		fields = append(fields, "title "+quote(req.Title))
	}
	if req.Body != "" {
		fields = append(fields, withBody(req.Body))
	}
	if len(req.Assignees) > 0 {
		fields = append(fields, assigneesDetail(req.Assignees))
	}
	if len(fields) > 0 {
		steps = append(steps, planned("edit", strings.Join(fields, ", "), func() (string, error) {
			_, err := c.client.UpdateIssue(c.ctx, ref, req)
			return "", err
		}))
	}
	if len(o.addLabels) > 0 {
		steps = append(steps, planned("add_labels", strings.Join(o.addLabels, ", "), func() (string, error) {
			return "", c.client.AddLabels(c.ctx, ref, o.addLabels)
		}))
	}
	for _, l := range o.rmLabels {
		steps = append(steps, planned("remove_label", l, func() (string, error) {
			return "", c.client.RemoveLabel(c.ctx, ref, l)
		}))
	}
	return steps
}

// mergeStep is the write of --merge. A merge the forge answers without
// merging is a conflict.
func (c *call) mergeStep(ref issuex.IssueRef, planned func(string, string, func() (string, error)) step) step {
	o := c.opts
	opts := issuex.MergeOptions{Method: issuex.MergeMethod(o.method), SHA: o.sha, CommitTitle: o.commitTitle, CommitMessage: o.commitMessage}
	method := o.method
	if method == "" {
		method = "the forge's default method"
	}
	return planned("merge", detail(method, prefixed("at ", o.sha)), func() (string, error) {
		res, err := c.client.MergePullRequest(c.ctx, ref, opts)
		if err != nil {
			return "", err
		}
		c.result.Merge = &res
		if !res.Merged {
			msg := res.Message
			if msg == "" {
				msg = "no reason given"
			}
			return "", fmt.Errorf("%w: the forge did not merge it: %s", issuex.ErrConflict, msg)
		}
		return "", nil
	})
}

// runSteps makes the writes in order, recording each as a side effect, and
// stops at the first that fails. Under --dry-run it makes none and lists
// them in the result instead.
func (c *call) runSteps(steps []step) *envelope.ErrorInfo {
	if c.opts.dryRun {
		c.result.DryRun = true
		for _, s := range steps {
			c.result.Planned = append(c.result.Planned, s.PlannedWrite)
		}
		c.next = append(c.next, envelope.Next{Tool: tool, Input: c.input, Flags: withoutFlag(c.flagArgs, "dry-run"),
			Why: dryRunWhy(c.opts)})
		return nil
	}
	for _, s := range steps {
		url, err := s.do()
		c.run.record(envelope.SideEffect{Action: s.Action, Kind: s.Kind, Target: s.Target, URL: url, OK: err == nil})
		if err != nil {
			return c.classify("write", strings.ReplaceAll(s.Action, "_", " ")+" "+s.Target, err)
		}
	}
	return nil
}

func dryRunWhy(o *options) string {
	why := "the same call without --dry-run, which makes these writes"
	if o.bodyFile == "-" {
		why += "; give the body on stdin again"
	}
	return why
}

// noChange reports a close or reopen of a thread that is already in that
// state: nothing is written.
func (c *call) noChange(name, state string) *envelope.ErrorInfo {
	c.result.NoChange = true
	c.summary = fmt.Sprintf("issue: %s is already %s; nothing written", name, state)
	return nil
}

// assigneesFor is --assignee for a form that sends it, with the warning the
// GitLab client calls for: it does not send assignees.
func (c *call) assigneesFor(flag string) []string {
	a := c.opts.assignees
	if len(a) > 0 && c.forge == issuex.ForgeTypeGitLab {
		c.run.warn(envelope.WarnAssigneesIgnored, envelope.SeverityHigh, "write",
			"%s --assignee %s: the GitLab client does not send assignees, so none is set", flag, strings.Join(a, ","))
		return nil
	}
	return a
}

// --- wording -------------------------------------------------------------

func quote(s string) string {
	const max = 60
	if utf8.RuneCountInString(s) > max {
		s = string([]rune(s)[:max-1]) + "…"
	}
	return strconv.Quote(s)
}

func quoteIf(s string) string {
	if s == "" {
		return ""
	}
	return quote(s)
}

func prefixed(prefix, s string) string {
	if s == "" {
		return ""
	}
	return prefix + s
}

func withBody(body string) string {
	if body == "" {
		return ""
	}
	n := utf8.RuneCountInString(body)
	return fmt.Sprintf("a body of %d %s", n, plural(n, "character", "characters"))
}

func labelsDetail(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	return plural(len(labels), "label ", "labels ") + strings.Join(labels, ", ")
}

func assigneesDetail(a []string) string {
	if len(a) == 0 {
		return ""
	}
	return plural(len(a), "assignee ", "assignees ") + strings.Join(a, ", ")
}

// detail joins the non-empty parts.
func detail(parts ...string) string {
	return strings.Join(slices.DeleteFunc(parts, func(s string) bool { return s == "" }), ", ")
}

// actions names the writes of a call, as its summary says them.
func actions(steps []step) string {
	names := make([]string, len(steps))
	for i, s := range steps {
		names[i] = strings.ReplaceAll(s.Action, "_", " ")
		if s.Detail != "" {
			names[i] += " " + s.Detail
		}
	}
	return strings.Join(names, "; ")
}

func presentTense(verb string) string {
	switch verb {
	case "commented on":
		return "comment on"
	case "edited":
		return "edit"
	case "closed":
		return "close"
	case "reopened":
		return "reopen"
	case "reviewed":
		return "review"
	case "merged":
		return "merge"
	}
	return verb
}

func commentCount(comments []issuex.Comment, err error) string {
	if err != nil {
		return " (comments unreadable)"
	}
	return fmt.Sprintf(" (%d %s)", len(comments), plural(len(comments), "comment", "comments"))
}

func prState(pr issuex.PullRequest) string {
	switch {
	case pr.Merged:
		return "merged"
	case pr.Draft && pr.State == "open":
		return "draft"
	}
	return pr.State
}

// checkCounts counts the checks by conclusion, or by status for one that has
// not concluded, in the order each first appears.
func checkCounts(checks []issuex.CheckRun) string {
	if len(checks) == 0 {
		return "none"
	}
	var order []string
	counts := map[string]int{}
	for _, ch := range checks {
		k := ch.Conclusion
		if k == "" {
			k = ch.Status
		}
		if k == "" {
			k = "unknown"
		}
		if counts[k] == 0 {
			order = append(order, k)
		}
		counts[k]++
	}
	parts := make([]string, len(order))
	for i, k := range order {
		parts[i] = fmt.Sprintf("%d %s", counts[k], k)
	}
	return strings.Join(parts, ", ")
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

// nonNil is s, or an empty slice for nil, so a list with nothing in it is
// written as [].
func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// withValue returns args with every occurrence of the flag name replaced by
// --name value at the end.
func withValue(args []string, name, value string) []string {
	var out []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "-"+name || a == "--"+name {
			i++ // its value
			continue
		}
		if strings.HasPrefix(a, "--"+name+"=") || strings.HasPrefix(a, "-"+name+"=") {
			continue
		}
		out = append(out, a)
	}
	return append(out, "--"+name, value)
}
