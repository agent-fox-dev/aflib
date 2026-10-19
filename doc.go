// Package aflib is the root of the module github.com/agent-fox-dev/aflib,
// which holds the Go packages shared by the agent-fox programs. The root
// package declares nothing; the packages are:
//
//   - issuex: one Client interface over the GitHub and GitLab APIs for
//     repositories, issues, labels, comments and pull or merge requests.
//   - jobqueue: a durable SQLite job queue with retry policies, per-key and
//     group serialization, progress tracking and crash recovery.
package aflib
