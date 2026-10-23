package envelope_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/agent-fox-dev/aflib/envelope"
	"github.com/agent-fox-dev/aflib/issuex"
)

// An envelope written by one of the spec repository's commands decodes: the
// properties this package does not know are ignored.
func TestDecodeIgnoresUnknownFields(t *testing.T) {
	data := `{"tool":"speca","version":"v1.4.1","ok":true,"status":"done","exit_code":0,"summary":"s",
	"result":{"spec_dir":".specs/09_x"},"artifacts":[{"kind":"spec_package"}],"model":{"spec":"x"},
	"next":[{"tool":"specr","input":".specs/09_x","flags":["--task","2"],"why":"w"}],"duration_ms":3,"started_at":"2026-10-22T08:28:47Z"}`
	env, err := envelope.Decode[map[string]any]([]byte(data))
	if err != nil {
		t.Fatal(err)
	}
	if env.Tool != "speca" || env.Result["spec_dir"] != ".specs/09_x" || len(env.Next) != 1 || env.Next[0].Flags[1] != "2" {
		t.Errorf("decoded %+v", env)
	}
}

func TestDecodeRefusesTrailingData(t *testing.T) {
	if _, err := envelope.DecodeIssue([]byte(`{"tool":"issue"} {}`)); err == nil {
		t.Error("two objects decoded")
	}
	if _, err := envelope.DecodeIssue([]byte(`not json`)); err == nil {
		t.Error("not JSON decoded")
	}
}

func TestEncodeRoundTrip(t *testing.T) {
	env := envelope.IssueEnvelope{
		Tool: "issue", OK: true, Status: envelope.StatusFor(envelope.ExitOK), Summary: "<a & b>",
		Result: &envelope.IssueResult{Forge: "github", Repo: "o/r", Ref: "o/r#1",
			Issue: &issuex.Issue{Number: 1, Title: "t"}, Comments: []issuex.Comment{}},
		SideEffects: []envelope.SideEffect{{Action: "comment", Target: "o/r#1", OK: true}},
	}
	b, err := envelope.Encode(env)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	if !strings.Contains(s, `"summary": "<a & b>"`) || !strings.Contains(s, `"comments": []`) || !strings.HasSuffix(s, "}\n") {
		t.Errorf("encoded:\n%s", s)
	}
	for _, absent := range []string{`"error"`, `"warnings"`, `"pull_request"`, `"issues"`, `"planned"`, `"dry_run"`} {
		if strings.Contains(s, absent) {
			t.Errorf("%s is written though empty:\n%s", absent, s)
		}
	}
	back, err := envelope.DecodeIssue(b)
	if err != nil {
		t.Fatal(err)
	}
	if back.Result.Issue.Title != "t" || back.Result.Comments == nil || back.SideEffects[0].Action != "comment" {
		t.Errorf("round trip lost data: %+v", back.Result)
	}
}

func TestAbsentResult(t *testing.T) {
	b, err := envelope.Encode(envelope.IssueEnvelope{Tool: "issue"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), `"result"`) {
		t.Errorf("a nil result is written:\n%s", b)
	}
}

func TestStatusAndRetryable(t *testing.T) {
	for code, want := range map[int]string{0: "done", 1: "failed", 2: "usage", 7: "failed"} {
		if got := envelope.StatusFor(code); got != want {
			t.Errorf("StatusFor(%d) = %q, want %q", code, got, want)
		}
	}
	for _, c := range []string{envelope.CategoryAPI, envelope.CategoryAborted} {
		if !envelope.RetryableFor(c) {
			t.Errorf("%s is not retryable", c)
		}
	}
	for _, c := range []string{envelope.CategoryUsage, envelope.CategoryAuth, envelope.CategoryInput, envelope.CategoryConflict} {
		if envelope.RetryableFor(c) {
			t.Errorf("%s is retryable", c)
		}
	}
}

func TestErrorInfoUnwraps(t *testing.T) {
	cause := errors.New("boom")
	e := (&envelope.ErrorInfo{Message: "m"}).WithCause(cause)
	if !errors.Is(e, cause) || e.Error() != "m" {
		t.Errorf("ErrorInfo %v does not wrap its cause", e)
	}
	var nilInfo *envelope.ErrorInfo
	if nilInfo.Error() != "" || nilInfo.Unwrap() != nil {
		t.Error("a nil ErrorInfo is not empty")
	}
}
