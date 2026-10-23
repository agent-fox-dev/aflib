// Package aflib is the root of the module github.com/agent-fox-dev/aflib,
// which holds the Go packages shared by the agent-fox programs. The root
// package declares nothing; the packages are:
//
//   - envelope: the JSON object the agent-fox commands write to stdout, as Go
//     types, with the result of the issue command.
//   - issuex: one Client interface over the GitHub and GitLab APIs for
//     repositories, issues, labels, comments and pull or merge requests.
//   - jobqueue: a durable SQLite job queue with retry policies, per-key and
//     group serialization, progress tracking and crash recovery.
//
// The module has one command, cmd/issue, which puts issuex on the command
// line for an agent or a person, and writes an envelope.
package aflib
