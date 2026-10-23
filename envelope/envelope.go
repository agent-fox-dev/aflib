package envelope

import (
	"bytes"
	"encoding/json"
	"errors"
)

// Exit codes. Something other than a human reads them.
const (
	// ExitOK means the command did what it was asked.
	ExitOK = 0
	// ExitFailed means a step errored. The failing stage is named in the
	// envelope's error object.
	ExitFailed = 1
	// ExitUsage means the invocation was wrong: an undefined flag, a flag
	// combination that cannot hold, an input that names nothing. Nothing was
	// written.
	ExitUsage = 2
)

// StatusFor maps an exit code to its status string: done, usage or failed.
func StatusFor(code int) string {
	switch code {
	case ExitOK:
		return "done"
	case ExitUsage:
		return "usage"
	default:
		return "failed"
	}
}

// Envelope is the single JSON object a command writes to stdout. R is the
// type of the command's own result, IssueResult for issue. Use a pointer type,
// as IssueEnvelope does, to tell an absent result from an empty one; a failure
// that happened before there was anything to report has none.
type Envelope[R any] struct {
	Tool     string `json:"tool" description:"the command that wrote this envelope"`
	Version  string `json:"version" description:"the command's build identity"`
	OK       bool   `json:"ok" description:"true only when the command did the whole job it was asked to do; never true beside a non-zero exit_code or an error"`
	Status   string `json:"status" description:"the outcome in words, derived from exit_code: done (0), failed (1) or usage (2)"`
	ExitCode int    `json:"exit_code" description:"the process exit code: 0 done, 1 failed, 2 usage error"`
	Summary  string `json:"summary" description:"one sentence on the outcome, at most 200 characters"`

	Error       *ErrorInfo   `json:"error,omitempty" description:"what failed and where; present exactly when ok is false"`
	Warnings    []Warning    `json:"warnings,omitempty" description:"what went differently than intended without stopping the call"`
	Result      R            `json:"result,omitzero" description:"the command's own result"`
	SideEffects []SideEffect `json:"side_effects,omitempty" description:"every write to a forge, in the order it was made, a failed one included"`
	Next        []Next       `json:"next,omitempty" description:"the invocations a caller would plausibly make next"`

	DurationMS int64  `json:"duration_ms" description:"wall-clock time of the call, in milliseconds"`
	StartedAt  string `json:"started_at" description:"when the call started, RFC 3339 in UTC"`
}

// Decode reads the envelope a command wrote. It fails when data is not one
// JSON object, or when a field has the wrong type; unknown fields are
// ignored. A command writes exactly one object, so data after it is an
// error too.
func Decode[R any](data []byte) (Envelope[R], error) {
	var env Envelope[R]
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&env); err != nil {
		return Envelope[R]{}, err
	}
	if dec.More() {
		return Envelope[R]{}, errors.New("envelope: data after the JSON object")
	}
	return env, nil
}

// Encode renders v as indented JSON and a trailing newline, the way a command
// writes its envelope. It does not escape <, > and & as encoding/json does by
// default: the output is read by programs and models, never embedded in HTML,
// and Markdown in a body is full of them.
func Encode(v any) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return b.Bytes(), nil
}

// SideEffect is one write a command made to a forge, in the order it was
// made. A write the forge refused is listed too, with OK false.
type SideEffect struct {
	Action  string   `json:"action" description:"what was written: create_issue, comment, edit, add_labels, remove_label, close, reopen, create_label, create_pull_request, review or merge"`
	Kind    string   `json:"kind,omitempty" description:"what it was written to: issue, pull_request or repository"`
	Target  string   `json:"target" description:"the thread (owner/repo#N, owner/repo!N) or the repository (owner/repo) written to"`
	URL     string   `json:"url,omitempty" description:"the web URL of what the write made, when the forge returned one"`
	OK      bool     `json:"ok" description:"whether the forge accepted the write"`
	Warning WarnCode `json:"warning,omitempty" description:"the warning raised about this write, if any"`
}

// Next is one suggested follow-up invocation: the tool, its single input, the
// flags to add, and why.
type Next struct {
	Tool  string   `json:"tool" description:"the command to run"`
	Input string   `json:"input" description:"its one positional input; empty when the call takes none"`
	Flags []string `json:"flags" description:"the flags to add, each value its own element"`
	Why   string   `json:"why" description:"why a caller would run it"`
}

// Warning is one thing that went differently than intended but did not stop
// the call. Code is stable; Stage is the step that raised it.
type Warning struct {
	Code     WarnCode `json:"code" description:"a stable code naming the cause; an open set"`
	Severity string   `json:"severity" description:"high when the result is not quite what it appears to be, low when informational"`
	Stage    string   `json:"stage" description:"the step that raised it"`
	Message  string   `json:"message" description:"what happened, in words"`
}

// Warning severities.
const (
	// SeverityHigh means the result is not quite what it appears to be.
	SeverityHigh = "high"
	// SeverityLow means the warning is informational.
	SeverityLow = "low"
)

// WarnCode identifies one warning cause. It is an open set: a decoder must
// accept a code it has no constant for.
type WarnCode string

// The warning codes the issue command raises.
const (
	// WarnCommentsUnreadable: the thread was read and its comments were not.
	WarnCommentsUnreadable WarnCode = "comments_unreadable"
	// WarnCommentsTruncated: the thread has more comments than were read.
	WarnCommentsTruncated WarnCode = "comments_truncated"
	// WarnAssigneesIgnored: the forge's client does not send assignees, so
	// the ones asked for were not set.
	WarnAssigneesIgnored WarnCode = "assignees_ignored"
)
