package main

import (
	"fmt"
	"io"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/agent-fox-dev/aflib/envelope"
)

// The build identity, set with -ldflags "-X main.version=… -X main.commit=…
// -X main.buildTime=…" (make install).
var (
	version   = "dev"
	commit    = ""
	buildTime = ""
)

// identity is the build identity --version prints and the envelope carries:
// the version, then the commit and build time when the build set them. A
// build that left the version at "dev" reports the module version go install
// recorded, when there is one.
func identity() string {
	v := version
	if v == "dev" {
		if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
			v = info.Main.Version
		}
	}
	var details []string
	if commit != "" {
		details = append(details, "commit "+commit)
	}
	if buildTime != "" {
		details = append(details, "built "+buildTime)
	}
	if len(details) == 0 {
		return v
	}
	return v + " (" + strings.Join(details, ", ") + ")"
}

// run gathers what a call reports beside its result: when it started, its
// warnings and its writes.
type run struct {
	started  time.Time
	warnings []envelope.Warning
	effects  []envelope.SideEffect
}

func newRun() *run { return &run{started: time.Now()} }

func (r *run) warn(code envelope.WarnCode, severity, stage, format string, args ...any) {
	r.warnings = append(r.warnings, envelope.Warning{Code: code, Severity: severity, Stage: stage, Message: fmt.Sprintf(format, args...)})
}

func (r *run) record(e envelope.SideEffect) { r.effects = append(r.effects, e) }

// summaryMax is the longest summary, in characters.
const summaryMax = 200

// emit writes the envelope of the call and returns the exit code. c is nil
// for a call refused before it ran. A usage error carries no result and no
// next: nothing was done.
func (r *run) emit(stdout io.Writer, c *call, failure *envelope.ErrorInfo) int {
	code := envelope.ExitOK
	if failure != nil {
		code = envelope.ExitFailed
		if failure.Category == envelope.CategoryUsage {
			code = envelope.ExitUsage
		}
		failure.Retryable = envelope.RetryableFor(failure.Category)
	}
	env := envelope.IssueEnvelope{
		Tool:        tool,
		Version:     identity(),
		OK:          failure == nil,
		Status:      envelope.StatusFor(code),
		ExitCode:    code,
		Error:       failure,
		Warnings:    r.warnings,
		SideEffects: r.effects,
		DurationMS:  time.Since(r.started).Milliseconds(),
		StartedAt:   r.started.UTC().Format(time.RFC3339),
	}
	var summary string
	if c != nil && code != envelope.ExitUsage {
		env.Result = c.result
		if failure == nil {
			env.Next = c.next
			summary = c.summary
		}
	}
	if summary == "" {
		if failure != nil {
			summary = failure.Message
		} else {
			summary = tool + ": done"
		}
	}
	env.Summary = summarize(summary, failure == nil, r.warnings)

	b, err := envelope.Encode(env)
	if err != nil {
		msg := "the result could not be encoded as JSON: " + err.Error()
		b, _ = envelope.Encode(envelope.IssueEnvelope{
			Tool: tool, Version: identity(), Status: envelope.StatusFor(envelope.ExitFailed), ExitCode: envelope.ExitFailed,
			Summary: msg, Error: &envelope.ErrorInfo{Stage: "emit", Category: envelope.CategoryInternal, Message: msg},
			DurationMS: env.DurationMS, StartedAt: env.StartedAt,
		})
		code = envelope.ExitFailed
	}
	_, _ = stdout.Write(b)
	return code
}

// summarize caps the sentence at summaryMax characters, with a clause that
// counts the high-severity warnings of a success.
func summarize(summary string, ok bool, warnings []envelope.Warning) string {
	high := 0
	for _, w := range warnings {
		if w.Severity == envelope.SeverityHigh {
			high++
		}
	}
	clause := ""
	if ok && high > 0 {
		clause = fmt.Sprintf("; %d high-severity %s", high, plural(high, "warning", "warnings"))
	}
	if room := summaryMax - utf8.RuneCountInString(clause); utf8.RuneCountInString(summary) > room {
		summary = string([]rune(summary)[:room])
	}
	return summary + clause
}
