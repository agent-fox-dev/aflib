# Import issuex and jobqueue

Status: **proposed** (2026-10-19). Nothing here is implemented. This is
item 3 of what the agent-fox service PRD asks of hub
([agent-fox `docs/prd/01`](https://github.com/agent-fox-dev/agent-fox/blob/main/docs/prd/01-triage-and-fix-forge-issues-as-a-service.md),
§9, decision D9): the two packages hub and agent-fox will share leave
their repositories and become this module. Next step: this document
becomes the `prd.md` of this repository's first spec package.

## Intent

Two packages exist today that more than one program needs and that
neither can import:

- **`issuex`**, in agent-fox: one `Client` interface over GitHub and
  GitLab for repositories, issues, labels, comments and pull or merge
  requests, with forge detection, a shared request loop (rate limits,
  retries, body limits), a `NoOp` client, and `httptest` coverage for
  both forges. Hub's forge API (hub PRD 02) is built on it, and
  `af --local` keeps using it directly.
- **`jobqueue`**, in hub's `internal/`: a durable SQLite job queue with
  retry policies, per-key and group serialization, progress, crash
  recovery and graceful shutdown. The agent-fox service's queue is
  this, and hub keeps using it for merges, rebuilds and clones.

`aflib` is the module they move into: `github.com/agent-fox-dev/aflib`,
consumed by version by hub, agent-fox and the agent-fox service. The move
is a move, not a rewrite: the exported API of each package is kept as it
is, the tests come along, and the consumers change an import path.

## Goals

1. `go get github.com/agent-fox-dev/aflib@v0.1.0` resolves, and
   `aflib/issuex` and `aflib/jobqueue` build and pass their tests with no
   `replace` and no dependency on hub, agent-fox or AgentKit.
2. The exported identifiers of both packages are unchanged by the move,
   so that hub's and agent-fox's consumers compile after an import-path
   change and nothing else, and the behaviour their tests pin is the
   behaviour aflib's tests pin.
3. The repository has the shape every agent-fox repository has: the
   four reference docs, `docs/generated/`, `make check`, the spec
   workflow, `AGENTS.md` with a layout that describes this repository.
4. hub and agent-fox delete their copies in the same change that
   switches them, so there is one `issuex` and one `jobqueue` anywhere.

## Non-goals

- **Changing what either package does.** A Forgejo backend for `issuex`,
  a Postgres backend for `jobqueue`, or any API change is a later PRD
  here, filed once the consumers are on this module.
- **A shared module for everything agent-fox-shaped.** `aflib` takes the
  packages two programs need and nothing speculative; a third package
  arrives with its second consumer.
- **Keeping the history by `git subtree` or `filter-repo`.** The files
  arrive in one commit that names the source repository and commit; the
  history stays where it was. *Workshop, if wanted:* a filtered import is
  mechanical and can be done instead, at the cost of a history this
  repository did not make.

## Background: what moves

### `issuex` (from `agent-fox-dev/agent-fox`, `issuex/`, at `84fb9b4`)

About 10 700 lines including tests. Standard library only (`net/http`,
`net/url`, `encoding/json`, `os/exec` for the `git remote get-url`
detection fallback); no third-party module. Exported surface, kept as
is:

| Identifier | Role |
|---|---|
| `Client` (alias `ForgeClient`) | the interface: `GetRepository`, `CreateIssue`, `ReadIssue`, `UpdateIssue`, `CloseIssue`, `ListIssues`, `AddComment`, `ListComments`, `AddLabels`, `RemoveLabel`, `CreateLabel`, `CreatePullRequest`, `ReadPullRequest`, `ReadChangedFiles`, `GetPRState`, `GetCIChecks`, `GetPRReviews`, `PostReviewComment`, `MergePullRequest`, `ClosePullRequest`, `Authenticated`, `Close` |
| `New`, `NewWithOptions`, `NewGitHub`, `NewGitLab`, `NewNoOp`, `Options` | construction: explicit `BaseURL`, `Token`, `HTTPClient`, `Repo`, `RemoteURL`, `Dir`, `NoOp` |
| `DetectForge`, `ForgeType`, `RegisterAdapter`, `ParseRemote`, `ParseIssueURL` and the parse helpers | forge detection from a base URL, a remote URL or a directory; the adapter registry |
| `Repo`, `IssueRef`, `Issue`, `IssueThread`, `Comment`, `PullRequest`, `ChangedFile`, `PRState`, `CheckRun`, `Review`, `Label`, `Repository`, `RepoPermissions`, `IssueFilter`, `IssueList`, `CommentList`, the request types, `MergeMethod`, `MergeOptions`, `MergeResult` | the vocabulary |
| the error values (`ErrAmbiguousForge`, `ErrUnsupportedForge`, …) and `GitHubMaxBodyLength`, `GitLabMaxBodyLength` | errors and limits |

Its tests are `httptest` servers for both forges, a GitLab smoke test
gated on an environment variable, and property tests; none needs a
network or a token.

Two behaviours worth naming because hub depends on them: `Options.Token`
and `Options.BaseURL` given explicitly take precedence over the
environment (`GITHUB_TOKEN`, `GITLAB_TOKEN`, `GITLAB_API_URL`), and a
host that names neither forge is `ErrAmbiguousForge` and nothing is sent
to it. Hub always passes the token and the remote URL explicitly and
never relies on the environment or on a working directory.

### `jobqueue` (from `agent-fox-dev/hub`, `internal/jobqueue/`, at `ce8e1b8`)

About 1 360 lines of code and 4 800 of tests. One third-party dependency,
`github.com/google/uuid`; the tests open SQLite through
`modernc.org/sqlite`, which becomes a test-only dependency of this
module. Exported surface, kept as is:

| Identifier | Role |
|---|---|
| `New(db, logger, opts...)`, `WithWorkers`, `WithPollInterval`, `WithGracePeriod` | construction over a caller-owned `*sql.DB` (WAL and `busy_timeout` are the caller's) |
| `InitSchema`, `MigrateGroupKey`, `MigrateProgress` | the `jobs` table and its two migrations |
| `Register(type, HandlerFunc, *RetryPolicy)`, `HandlerFunc`, `RetryPolicy`, `DefaultRetryPolicy` | handlers and backoff |
| `Enqueue(EnqueueParams)`, `EnqueueParams` (type, key, nonce, payload, submitted-by, group, exclude) | idempotent enqueue with per-key and group dedup |
| `Start`, `Shutdown`, `Wait`, `Stop` | lifecycle with crash recovery and a grace period |
| `CancelJob`, `RequeueDeadLetter`, `UpdateProgress`, `GetByID`, `ListByType`, `ListByKey`, `ListByKeyPrefix`, `CountByStatus`, `Job`, `ListOpts`, the `Status*` constants, `ErrNotCancellable` | the queue's API to its callers |
| `JobIDFromContext`, `ContextWithJobID` | the job id on a handler's context |

Hub's consumers: `cmd/af-hub`, `internal/carrypatch`, `internal/merge`,
and `internal/workspace`'s clone queue is a separate in-memory queue that
does not move.

## 1. The module

```
github.com/agent-fox-dev/aflib
├── go.mod              go 1.27; require github.com/google/uuid; modernc.org/sqlite (tests)
├── issuex/             the package, its tests, its testdata
├── jobqueue/           the package and its tests
├── docs/
│   ├── api.md          the map of the public packages (there is no REST API)
│   ├── architecture.md what each package is, what imports what, the test layout
│   ├── cli.md          one line: this repository has no command
│   ├── configuration.md the environment variables issuex reads, and that jobqueue reads none
│   ├── generated/      go doc -all, committed
│   ├── prd/            this document
│   └── errata/
├── .specs/             steering.md (shared), the spec packages
├── Makefile            check (fmt, vet, lint, test-short), check-release, test, doc, tidy, build
├── AGENTS.md           the shared policy with this repository's layout
└── README.md
```

Rules:

- **One module, two packages, no root package.** There is nothing to put
  at the root; a `doc.go` names the module and lists the packages.
- **The dependency allowlist is the two modules above.** A third is a
  ruling in `docs/architecture.md` first, as AgentKit's `DEPS.md` does.
- **The exported API golden.** `make check` runs a test that compares the
  exported identifiers of each package (`go doc -all`, filtered to
  declarations) against a committed golden, so that a later change to
  the API is a deliberate regeneration with a commit that says so. The
  first golden is the one the move produces.
- **Versioning.** Tags `vX.Y.Z` on the module; the first is `v0.1.0`,
  cut when both consumers are switched. Pre-1.0: a breaking change bumps
  the minor and is recorded in the errata.

## 2. The move

1. **Copy.** `issuex/` from agent-fox at `84fb9b4` and `internal/jobqueue/`
   from hub at `ce8e1b8` into this repository, package names unchanged,
   import paths rewritten in the tests (`issuex`'s external test package
   imports itself by path). One commit per package, each naming the
   source repository and commit in its message.
2. **Build it.** `go.mod`, `go.sum`, the Makefile, `make check` green,
   `make doc` committed, the API goldens, the four docs, `AGENTS.md`'s
   layout and `README.md`.
3. **Switch hub.** `internal/jobqueue` deleted; every import becomes
   `github.com/agent-fox-dev/aflib/jobqueue`; `replace
   github.com/agent-fox-dev/aflib => ../aflib` while the tag does not
   exist, as hub does for apikit; `docs/architecture.md` and `AGENTS.md`
   there drop the package from their layout; an erratum there records
   the move. `make check` in hub stays green with no test changed beyond
   the import path.
4. **Switch agent-fox.** `issuex/` deleted; `README.md`, `AGENTS.md` and
   the generated docs there follow; `go.mod` requires aflib. agent-fox
   has no consumer of `issuex` outside its own tests today, so the
   switch is the deletion and the requirement that the service and
   `af --local` will use.
5. **Tag `v0.1.0`**, drop the `replace` in both consumers, and record the
   pin.

Steps 3 and 4 are independent; both wait on 2.

## 3. What stays exactly as it was

- Every test of both packages, with the import path as the only edit.
  A test that cannot pass outside its old repository (none is known)
  is a finding, not a deletion.
- `issuex`'s environment-driven detection (`GITHUB_TOKEN`, `GITLAB_TOKEN`,
  `GITLAB_API_URL`, `GITHUB_API_URL`, the `git remote get-url` fallback
  and the `gh` CLI probe where it exists). Hub will not use it; `af
  --local` does. It is documented in `docs/configuration.md` here.
- `jobqueue`'s requirement that the caller configures WAL and
  `busy_timeout`, and that handlers are idempotent. Documented in
  `docs/architecture.md` here as it is in the package comment.
- The requirement ids in the test names and comments (`10-REQ-6.E3`,
  `01-REQ-4.8`, …). They refer to specs in the source repositories'
  archives; the erratum of step 3 and 4 says so, and new specs here use
  this repository's numbering.

## 4. Acceptance criteria

1. In a clean checkout with no sibling repositories, `make check` passes
   in this module: `gofmt`, `go vet`, `golangci-lint` when installed,
   `go test -short ./...`.
2. `go list -m all` in this module lists `github.com/google/uuid`,
   `modernc.org/sqlite` and their requirements, and nothing from hub,
   agent-fox or AgentKit.
3. The exported-API golden of each package equals the `go doc -all`
   declarations of the source package at the named commit, name for name
   and signature for signature.
4. hub at the switch commit builds and its `make check` passes with
   `internal/jobqueue` absent and no `jobqueue` test changed beyond its
   import path.
5. agent-fox at the switch commit builds with `issuex/` absent and
   `make check` passes.
6. `docs/generated/issuex.txt` and `docs/generated/jobqueue.txt` exist
   and match `make doc`'s output.

## 5. Open questions

- **History.** A plain copy (the recommendation) or a filtered import.
  The plain copy keeps this repository's history its own and the source
  commits reachable by hash; the filtered import makes `git blame` useful
  here at the cost of 300 commits that predate the module.
- **The test-only SQLite driver.** `modernc.org/sqlite` is pure Go and
  the same driver hub uses. A consumer on another driver is unaffected
  (the package takes a `*sql.DB`), and the tests do not need to be
  driver-neutral.
- **`issuex`'s `os/exec` fallbacks.** They exist for a developer's
  shell. A build tag to drop them for servers is possible and not
  proposed: hub passes everything explicitly and never reaches them.

## Documentation

| Change | Update |
|---|---|
| the module and its packages | `README.md`, `docs/api.md`, `docs/architecture.md`, `docs/generated/` |
| the environment `issuex` reads | `docs/configuration.md` |
| the layout | `AGENTS.md` (Repository Layout), `.specs/steering.md` (Repository Specifics) |
| the consumers' switch | hub: `docs/architecture.md`, `AGENTS.md`, an erratum; agent-fox: `README.md`, `AGENTS.md`, an erratum |
