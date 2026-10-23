package envelope

import "github.com/agent-fox-dev/aflib/issuex"

// IssueEnvelope is what the issue command writes.
type IssueEnvelope = Envelope[*IssueResult]

// DecodeIssue reads the envelope the issue command wrote, whichever of its
// forms it was. Which fields of the result are set tells the forms apart.
func DecodeIssue(data []byte) (IssueEnvelope, error) { return Decode[*IssueResult](data) }

// IssueResult is the result of the issue command. Every form sets Forge and
// Repo, and Ref when it names a thread; the other fields are those of the
// form, and the ones a form does not produce are absent.
type IssueResult struct {
	Forge string `json:"forge" description:"the forge the call went to: github or gitlab"`
	Repo  string `json:"repo" description:"the repository, owner/name; a GitLab owner may hold subgroups"`
	Host  string `json:"host,omitempty" description:"the repository's web host, when the input or the origin remote named one"`
	Ref   string `json:"ref,omitempty" description:"the thread: owner/name#N for an issue, owner/name!N for a pull or merge request; runnable as the input of the next call"`

	// The read of a thread, and the thread after a write to it.
	Issue             *issuex.Issue        `json:"issue,omitempty" description:"the issue, or the issue view of a pull request: title, body, state, labels, author"`
	Comments          []issuex.Comment     `json:"comments,omitzero" description:"the thread's comments, oldest first; [] when it has none. Read forms only"`
	CommentsTruncated bool                 `json:"comments_truncated,omitempty" description:"the thread has more comments than were read (the client reads at most 500)"`
	PullRequest       *issuex.PullRequest  `json:"pull_request,omitempty" description:"the pull or merge request: branches, head commit, draft and merged"`
	Checks            []issuex.CheckRun    `json:"checks,omitzero" description:"with --checks, the CI checks of the head commit"`
	Reviews           []issuex.Review      `json:"reviews,omitzero" description:"with --reviews, the reviews of the pull request"`
	Files             []issuex.ChangedFile `json:"files,omitzero" description:"with --files, the files the pull request changes"`

	// The read of a repository.
	Repository *issuex.Repository `json:"repository,omitempty" description:"the repository: default branch, visibility, the caller's permissions and the merge methods allowed"`

	// --list.
	Issues     []issuex.Issue `json:"issues,omitzero" description:"with --list, the issues that match, without pull requests; [] when none does"`
	Incomplete bool           `json:"incomplete,omitempty" description:"with --list, more issues match than --limit allowed"`

	// The write forms.
	CommentURL string              `json:"comment_url,omitempty" description:"with --comment, the web URL of the comment"`
	Label      *issuex.Label       `json:"label,omitempty" description:"with --create-label, the label asked for; a label that already existed is not an error"`
	Merge      *issuex.MergeResult `json:"merge,omitempty" description:"with --merge, the forge's answer: merged, the merge commit and its message"`
	NoChange   bool                `json:"no_change,omitempty" description:"the thread was already in the state asked for (--close on a closed one, --reopen on an open one); nothing was written"`
	DryRun     bool                `json:"dry_run,omitempty" description:"the call ran under --dry-run: nothing was written, and planned lists what would have been"`
	Planned    []PlannedWrite      `json:"planned,omitzero" description:"under --dry-run, the writes the call would have made, in order"`
}

// PlannedWrite is one write a call under --dry-run would have made, named as
// a SideEffect of the real call would name it.
type PlannedWrite struct {
	Action string `json:"action" description:"what would be written, as SideEffect.Action"`
	Kind   string `json:"kind,omitempty" description:"what it would be written to: issue, pull_request or repository"`
	Target string `json:"target" description:"the thread or the repository it would be written to"`
	Detail string `json:"detail,omitempty" description:"what it would carry, in words: the fields, labels or method"`
}
