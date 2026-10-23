package main

import (
	"errors"
	"flag"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/agent-fox-dev/aflib/envelope"
)

// form is the operation a call names.
type form int

const (
	formRead form = iota
	formList
	formCreate
	formComment
	formEdit
	formClose
	formReopen
	formCreateLabel
	formCreatePR
	formReview
	formMerge
)

// formFlag is the flag that names each form, in the order the help lists them.
var formFlag = []struct {
	form form
	flag string
}{
	{formList, "list"},
	{formCreate, "create"},
	{formComment, "comment"},
	{formEdit, "edit"},
	{formClose, "close"},
	{formReopen, "reopen"},
	{formCreateLabel, "create-label"},
	{formCreatePR, "create-pr"},
	{formReview, "review"},
	{formMerge, "merge"},
}

func (f form) flagName() string {
	for _, ff := range formFlag {
		if ff.form == f {
			return "--" + ff.flag
		}
	}
	return "the read"
}

// commonFlags belong to every form.
var commonFlags = []string{"dir", "forge", "timeout", "schema", "version"}

// formFlags are the flags each form takes beside the common ones and its own.
var formFlags = map[form][]string{
	formRead:        {"checks", "reviews", "files"},
	formList:        {"state", "label", "assignee", "sort", "direction", "limit"},
	formCreate:      {"title", "body", "body-file", "label", "assignee", "dry-run"},
	formComment:     {"body", "body-file", "dry-run"},
	formEdit:        {"title", "body", "body-file", "add-label", "remove-label", "assignee", "dry-run"},
	formClose:       {"body", "body-file", "dry-run"},
	formReopen:      {"dry-run"},
	formCreateLabel: {"color", "description", "dry-run"},
	formCreatePR:    {"head", "base", "title", "body", "body-file", "draft", "dry-run"},
	formReview:      {"body", "body-file", "dry-run"},
	formMerge:       {"method", "sha", "commit-title", "commit-message", "dry-run"},
}

// takesRepo reports whether a form works on a repository rather than on a
// thread; such a form uses the origin remote's repository when no input
// names one.
func (f form) takesRepo() bool {
	switch f {
	case formList, formCreate, formCreateLabel, formCreatePR:
		return true
	}
	return false
}

// The closed sets of values of the flags that have one.
var (
	forges     = []string{"github", "gitlab"}
	states     = []string{"open", "closed", "all"}
	sorts      = []string{"created", "updated"}
	directions = []string{"asc", "desc"}
	methods    = []string{"merge", "squash", "rebase"}
)

// options are issue's flags.
type options struct {
	dir     string
	forge   string
	timeout time.Duration
	dryRun  bool
	schema  bool
	version bool

	list, create, comment, edit, close, reopen bool
	createPR, review, merge                    bool
	createLabel                                string

	checks, reviews, files bool

	title, body, bodyFile string
	labels, assignees     listFlag
	addLabels, rmLabels   listFlag

	state, sort, direction string
	limit                  int

	head, base string
	draft      bool

	method, sha, commitTitle, commitMessage string
	color, description                      string

	// given holds the names of the flags the call set.
	given map[string]bool
}

func (o *options) register(fs *flag.FlagSet) {
	o.given = map[string]bool{}
	fs.BoolVar(&o.list, "list", false, "the list form: the issues of the repository, pull requests left out, filtered by --state, --label and --assignee, ordered by --sort and --direction, at most --limit")
	fs.BoolVar(&o.create, "create", false, "the create form: open an issue in the repository with --title (required), --body or --body-file, --label and --assignee")
	fs.BoolVar(&o.comment, "comment", false, "the comment form: comment on the thread; the body is --body or --body-file")
	fs.BoolVar(&o.edit, "edit", false, "the edit form: change the thread's --title, --body, --assignee, and --add-label and --remove-label; a body cannot be emptied")
	fs.BoolVar(&o.close, "close", false, "the close form: close the thread, after a comment when --body or --body-file is given; a closed thread is left as it is")
	fs.BoolVar(&o.reopen, "reopen", false, "the reopen form: reopen the thread; an open thread is left as it is")
	fs.StringVar(&o.createLabel, "create-label", "", "the create-label form: create the label of this name in the repository, with --color and --description; one that exists already is not an error")
	fs.BoolVar(&o.createPR, "create-pr", false, "the create-pr form: open a pull or merge request from --head into --base with --title, --body or --body-file, and --draft")
	fs.BoolVar(&o.review, "review", false, "the review form: post a review comment on the pull request; the body is --body or --body-file")
	fs.BoolVar(&o.merge, "merge", false, "the merge form: merge the pull request with --method, at --sha, with --commit-title and --commit-message")

	fs.BoolVar(&o.checks, "checks", false, "with the read of a pull request, also read the CI checks of its head commit")
	fs.BoolVar(&o.reviews, "reviews", false, "with the read of a pull request, also read its reviews")
	fs.BoolVar(&o.files, "files", false, "with the read of a pull request, also read the files it changes")

	fs.StringVar(&o.title, "title", "", "the title: of the issue or pull request to open, or the thread's new title with --edit")
	fs.StringVar(&o.body, "body", "", "the body: of the issue, comment, pull request or review, the thread's new body with --edit, or the comment --close posts first")
	fs.StringVar(&o.bodyFile, "body-file", "", "read the body from this file; - is stdin. Not with --body")
	fs.Var(&o.labels, "label", "a label: of the issue to open, or a filter of --list (an issue must have every one); repeated or comma-separated")
	fs.Var(&o.assignees, "assignee", "an assignee: of the issue to open, or the thread's new assignees with --edit; repeated or comma-separated. With --list, the one assignee to filter by. The GitLab client does not send assignees")
	fs.Var(&o.addLabels, "add-label", "with --edit, a label to add; repeated or comma-separated")
	fs.Var(&o.rmLabels, "remove-label", "with --edit, a label to remove; repeated or comma-separated. One the thread does not have is not an error")

	fs.StringVar(&o.state, "state", "open", "with --list, the issues in this state: open, closed or all")
	fs.StringVar(&o.sort, "sort", "created", "with --list, order by: created or updated")
	fs.StringVar(&o.direction, "direction", "desc", "with --list, the order: asc or desc")
	fs.IntVar(&o.limit, "limit", 30, "with --list, the most issues to list; incomplete is true when more match")

	fs.StringVar(&o.head, "head", "", "with --create-pr, the branch to merge from")
	fs.StringVar(&o.base, "base", "", "with --create-pr, the branch to merge into")
	fs.BoolVar(&o.draft, "draft", false, "with --create-pr, open it as a draft")

	fs.StringVar(&o.method, "method", "", "with --merge, the merge method: merge, squash or rebase; default the forge's (on GitHub, the first the repository allows)")
	fs.StringVar(&o.sha, "sha", "", "with --merge, the head commit the pull request must be at, or the merge is refused")
	fs.StringVar(&o.commitTitle, "commit-title", "", "with --merge, the title of the merge commit")
	fs.StringVar(&o.commitMessage, "commit-message", "", "with --merge, the message of the merge commit")

	fs.StringVar(&o.color, "color", "", "with --create-label, the label's colour as six hex digits (ededed); default the forge's on GitHub, ededed on GitLab")
	fs.StringVar(&o.description, "description", "", "with --create-label, the label's description")

	fs.StringVar(&o.dir, "dir", ".", "the git working tree whose origin remote names the repository when the input does not")
	fs.StringVar(&o.forge, "forge", "", "the forge, github or gitlab, for when it cannot be told from the input, the environment or the origin remote")
	fs.DurationVar(&o.timeout, "timeout", defaultTimeout, "the longest the whole call may take")
	fs.BoolVar(&o.dryRun, "dry-run", false, "with a write form, make its reads and none of its writes, and list the writes in result.planned")
	fs.BoolVar(&o.schema, "schema", false, "print the flags, the input and the output's JSON Schema as one JSON object, and exit")
	fs.BoolVar(&o.version, "version", false, "print the build identity and exit")
}

// form is the form the flags name: the first form flag given, or the read.
// check refuses a call that names more than one.
func (o *options) form() form {
	for _, f := range o.forms() {
		return f
	}
	return formRead
}

// forms are the form flags the call set, in help order.
func (o *options) forms() []form {
	var out []form
	for _, ff := range formFlag {
		if o.given[ff.flag] {
			out = append(out, ff.form)
		}
	}
	return out
}

// check refuses, as a usage error and before anything is read or sent, the
// calls that cannot hold: two forms, a flag of another form, a flag value
// outside its set, a missing required flag, and an input of the wrong kind
// for the form. ref is the one positional input, or empty.
func (o *options) check(ref string) *envelope.ErrorInfo {
	var errs []error
	var hint *envelope.FixHint
	add := func(h *envelope.FixHint, format string, args ...any) {
		errs = append(errs, fmt.Errorf(format, args...))
		if h != nil && hint == nil {
			hint = h
		}
	}

	forms := o.forms()
	if len(forms) > 1 {
		names := make([]string, len(forms))
		for i, f := range forms {
			names[i] = f.flagName()
		}
		add(nil, "%s are %d operations: give one", strings.Join(names, " and "), len(forms))
	}
	f := o.form()
	allowed := append(slices.Clone(commonFlags), formFlags[f]...)
	for _, ff := range formFlag {
		allowed = append(allowed, ff.flag)
	}
	var stray []string
	for name := range o.given {
		if !slices.Contains(allowed, name) {
			stray = append(stray, "--"+name)
		}
	}
	slices.Sort(stray)
	if len(stray) > 0 {
		add(nil, "%s %s not apply to %s", strings.Join(stray, ", "), plural(len(stray), "does", "do"), f.flagName())
	}

	o.checkValues(f, add)

	if ref != "" {
		t, err := parseRef(ref)
		switch {
		case err != nil:
			add(nil, "%v", err)
		case t.number == 0 && !f.takesRepo() && f != formRead:
			add(nil, "%s works on an issue or pull request, and %q names a repository: give owner/repo#N, owner/repo!N or a URL", f.flagName(), ref)
		case t.number != 0 && f.takesRepo():
			add(nil, "%s works on a repository, and %q names an issue or pull request: give owner/repo, or no input for the origin remote's", f.flagName(), ref)
		case t.number == 0 && f == formRead && (o.checks || o.reviews || o.files):
			add(nil, "--checks, --reviews and --files read a pull request, and %q names a repository", ref)
		}
	} else if !f.takesRepo() {
		add(nil, "%s works on an issue or pull request: give one (owner/repo#N, owner/repo!N, N or a URL)", f.flagName())
	}

	if len(errs) == 0 {
		return nil
	}
	return usageInfo(errors.Join(errs...), hint)
}

// checkValues refuses the flag values outside their sets and the required
// flags the form is missing.
func (o *options) checkValues(f form, add func(*envelope.FixHint, string, ...any)) {
	oneOf := func(name, value string, valid []string) {
		if o.given[name] && !slices.Contains(valid, value) {
			add(&envelope.FixHint{Flag: "--" + name, Valid: slices.Clone(valid)},
				"--%s %q is not one of %s", name, value, strings.Join(valid, ", "))
		}
	}
	oneOf("forge", o.forge, forges)
	oneOf("state", o.state, states)
	oneOf("sort", o.sort, sorts)
	oneOf("direction", o.direction, directions)
	oneOf("method", o.method, methods)
	if o.timeout <= 0 {
		add(&envelope.FixHint{Flag: "--timeout"}, "--timeout %s is not a positive duration", o.timeout)
	}
	if o.limit < 1 {
		add(&envelope.FixHint{Flag: "--limit"}, "--limit %d: list at least 1 issue", o.limit)
	}
	if o.given["body"] && o.given["body-file"] {
		add(nil, "--body and --body-file are two bodies: give one")
	}
	for _, name := range []string{"body", "title", "body-file", "head", "base", "sha", "commit-title", "commit-message", "color", "description"} {
		if v := o.flagString(name); o.given[name] && strings.TrimSpace(v) == "" {
			add(nil, "--%s is empty: leave it out instead", name)
		}
	}
	for name, l := range map[string]listFlag{"label": o.labels, "assignee": o.assignees, "add-label": o.addLabels, "remove-label": o.rmLabels} {
		if o.given[name] && len(l) == 0 {
			add(nil, "--%s names nothing", name)
		}
	}
	if f == formList && len(o.assignees) > 1 {
		add(nil, "--list filters by one --assignee, got %d", len(o.assignees))
	}
	if o.given["color"] {
		c := strings.TrimPrefix(o.color, "#")
		if len(c) != 6 || strings.Trim(strings.ToLower(c), "0123456789abcdef") != "" {
			add(&envelope.FixHint{Flag: "--color"}, "--color %q is not six hex digits, such as ededed", o.color)
		}
	}
	hasBody := o.given["body"] || o.given["body-file"]
	switch f {
	case formCreate:
		if !o.given["title"] {
			add(&envelope.FixHint{Flag: "--title"}, "--create needs --title")
		}
	case formComment, formReview:
		if !hasBody {
			add(&envelope.FixHint{Flag: "--body"}, "%s needs a body: --body or --body-file", f.flagName())
		}
	case formEdit:
		if !o.given["title"] && !hasBody && len(o.addLabels) == 0 && len(o.rmLabels) == 0 && len(o.assignees) == 0 {
			add(nil, "--edit needs a change: --title, --body, --body-file, --add-label, --remove-label or --assignee")
		}
		for _, l := range o.addLabels {
			if slices.Contains(o.rmLabels, l) {
				add(nil, "--add-label and --remove-label both name %q", l)
			}
		}
	case formCreateLabel:
		if strings.TrimSpace(o.createLabel) == "" {
			add(nil, "--create-label needs the label's name")
		}
	case formCreatePR:
		for _, name := range []string{"head", "base", "title"} {
			if !o.given[name] {
				add(&envelope.FixHint{Flag: "--" + name}, "--create-pr needs --%s", name)
			}
		}
	}
}

// flagString is the value of a string flag, by name.
func (o *options) flagString(name string) string {
	switch name {
	case "body":
		return o.body
	case "title":
		return o.title
	case "body-file":
		return o.bodyFile
	case "head":
		return o.head
	case "base":
		return o.base
	case "sha":
		return o.sha
	case "commit-title":
		return o.commitTitle
	case "commit-message":
		return o.commitMessage
	case "color":
		return o.color
	case "description":
		return o.description
	}
	return ""
}

// listFlag is a flag given once or repeated, each value a comma-separated
// list. Blank items are dropped and a repeated item is kept once.
type listFlag []string

func (l *listFlag) String() string { return strings.Join(*l, ",") }

func (l *listFlag) Set(v string) error {
	for _, item := range strings.Split(v, ",") {
		item = strings.TrimSpace(item)
		if item != "" && !slices.Contains(*l, item) {
			*l = append(*l, item)
		}
	}
	return nil
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
