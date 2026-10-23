// Package envelope defines the JSON object the agent-fox commands write to
// stdout, as Go types a program can decode it into, and the result of the
// issue command (cmd/issue) that this module ships.
//
// A command writes exactly one JSON object on every program-driven path,
// including the failing ones: an Envelope. Its result property holds the
// command's own result, whose type depends on the command:
//
//	issue  an IssueResult  a thread, a repository, a list of issues, or what a write did
//
// Decode a command's output with the function for its result:
//
//	env, err := envelope.DecodeIssue(stdout)
//	if err != nil {
//		return err // not an envelope at all
//	}
//	if !env.OK {
//		return env.Error // an *ErrorInfo: stage, category, message, fix hint
//	}
//	fmt.Println(env.Result.Issue.Title)
//
// Decode is the same for any other result type, and Envelope[any] reads a
// result of a shape the caller does not know.
//
// The JSON is that of the envelope the spec repository's commands (spec,
// specr, speca) write, without the properties only a model-driven command
// writes (artifacts, input, model, usage). A decoder of either reads the
// other: the properties one does not know are ignored.
//
// The package imports the standard library and issuex, whose value types
// IssueResult reuses.
//
// # Compatibility
//
// The types follow the commands' output. Fields are added over time and a
// decoder ignores the ones it does not know; a field's name and meaning do
// not change. Code, category and stage names are open sets: a value this
// package has no constant for is not an error.
package envelope
