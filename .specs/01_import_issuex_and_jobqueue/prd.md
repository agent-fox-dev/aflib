---
spec_id: "01"
spec_name: "import_issuex_and_jobqueue"
title: "Import issuex and jobqueue into aflib"
status: "active"
created_at: "2026-10-19T17:05:52.36554Z"
updated_at: "2026-10-19T17:05:52.36554Z"
intent_hash: "31f5ba012e2ac1428e1deb86af0383d97a1e9d08dcfb6c3e962efeec18647591"
schema_version: 2
source: "docs/prd/01-import-issuex-and-jobqueue.md"
---
## Intent

Make `github.com/agent-fox-dev/aflib` the one home of the Go packages that more than one agent-fox program needs. It starts with the forge client `issuex` (from agent-fox) and the durable SQLite job queue `jobqueue` (from hub). Both move in unchanged, so their consumers can switch by changing an import path. The module also gets the repository shape, the quality gate and the exported-API guard that every agent-fox repository has.

## Goals

1. In a clean checkout of this repository alone (no sibling repositories, no `go.work`, no `replace`), `make check` passes. That means `gofmt`, `go vet`, `golangci-lint` when installed, and `go test -short ./...`, which runs every copied test of both packages.
2. `go list -m all` in this module lists only the module itself, `github.com/google/uuid`, `modernc.org/sqlite` and their transitive requirements. It lists nothing under `github.com/agent-fox-dev/hub`, `github.com/agent-fox-dev/agent-fox` or AgentKit.
3. Every non-test `.go` file under `issuex/` and `jobqueue/` is byte-identical to its source file at the named commit. Every test file differs from its source only in import-path lines. The only exceptions are findings listed in the erratum.
4. Every public package has a committed exported-API golden. A change to an exported declaration fails `make check` until the golden is regenerated on purpose.
5. The repository has the standard agent-fox shape:
   - the four reference docs
   - `docs/generated/{aflib,issuex,jobqueue}.txt`, matching `make doc`
   - a `Makefile` with the targets `AGENTS.md` lists
   - `.golangci.yml`
   - `AGENTS.md`, `CLAUDE.md` and `.specs/steering.md` describing this repository rather than AgentKit
   - a `README.md` that explains the module.

## Non-goals

- **Changing what either package does.** This spec adds no Forgejo backend for `issuex`, no Postgres backend for `jobqueue`, and no API change. Each of those is a later PRD here.
- **Switching hub.** Deleting `internal/jobqueue`, rewriting hub's imports, the temporary `replace` and hub's erratum are work in the hub repository, under a hub spec.
- **Switching agent-fox.** Deleting `issuex/`, requiring aflib and updating agent-fox's docs are work in the agent-fox repository, under an agent-fox spec.
- **Tagging `v0.1.0`** and removing the consumers' `replace` lines. A human does this as a release step once both consumers are switched. This spec only documents the versioning rule.
- **Importing git history** by `git subtree` or `filter-repo`.
- **A build tag that removes `issuex`'s `os/exec` fallbacks** (`git remote get-url`, the `gh` probe).
- **Tidying the copied code.** Doc comments that mention hub or agent-fox, lint findings, and requirement IDs in test names all stay as they are.
- **Further packages.** This includes hub's in-memory workspace clone queue. A third package arrives with its second consumer.
- **Driver-neutral `jobqueue` tests**, and any CI workflow configuration.

## Background

**What is in this repository today.** There is no Go code, no `go.mod`, no `Makefile` and no reference docs. The tracked files are:

- `README.md`: one paragraph announcing the two packages.
- `AGENTS.md`, and `CLAUDE.md` with identical content. This is the shared agent policy. Its Repository Layout section still describes AgentKit (root package `agentkit.Config`, `examples/`, `containers/`), and it fixes the Go version at 1.27 and the `make` target table.
- `.specs/steering.md`: the shared steering. Its Repository Specifics section still describes AgentKit and its nested `MODULES`.
- `.gitignore`: ignores `go.work`, `bin/`, coverage output.
- `LICENSE` (MIT, agent-fox-dev).
- `docs/prd/01-import-issuex-and-jobqueue.md`: the source of this PRD.

There are no tags. The remote is `https://github.com/agent-fox-dev/aflib`.

**Why.** Two existing packages are needed by more than one program, and neither program can import the other's copy:

- `issuex` lives in agent-fox. It is a `Client` interface over GitHub and GitLab with forge detection, a shared request loop, a `NoOp` client and `httptest` coverage. Hub's forge API is built on it, and `af --local` uses it.
- `jobqueue` lives in hub's `internal/`. It is a durable SQLite job queue with retry policies, per-key and group serialization, progress tracking, crash recovery and graceful shutdown. The agent-fox service needs it, and hub uses it for merges, rebuilds and clones.

The agent-fox service PRD (§9, decision D9) asks for both to move here.

**What moves.** None of this can be read from this repository, so all of it is unverified here.

`issuex` comes from `agent-fox-dev/agent-fox`, directory `issuex/`, at `84fb9b4`:
- About 10,700 lines including tests.
- Standard library only. It uses `os/exec` for the `git remote get-url` fallback.
- It reads `GITHUB_TOKEN`, `GITLAB_TOKEN`, `GITLAB_API_URL` and `GITHUB_API_URL`. An explicit `Options.Token` or `Options.BaseURL` overrides the environment.
- A host that names neither forge yields `ErrAmbiguousForge`, and no request is sent to it.
- Its tests are `httptest` servers, property tests, and a GitLab smoke test gated on an environment variable.

`jobqueue` comes from `agent-fox-dev/hub`, directory `internal/jobqueue/`, at `ce8e1b8`:
- About 1,360 lines of code and 4,800 lines of tests.
- Its one runtime dependency is `github.com/google/uuid`. Its tests open SQLite through `modernc.org/sqlite`.
- It works over a caller-owned `*sql.DB`. The caller configures WAL and `busy_timeout`, and handlers must be idempotent.

## Requirements

### R1. The module

- **`go.mod`.** It declares `module github.com/agent-fox-dev/aflib` and `go 1.27`. It requires `github.com/google/uuid` and `modernc.org/sqlite` at the versions hub's `go.mod` requires at `ce8e1b8`; `go mod tidy` may add `// indirect` lines. It has no `replace` directive. `go.sum` is committed and no `go.work` is committed.
- **Packages.** The module has exactly three packages:
  - the root package `aflib`, in `doc.go`, which holds only a package doc comment. The comment names the module, says it holds the packages shared by the agent-fox programs, and lists `issuex` and `jobqueue` with one line each. The root package has no declarations and no imports.
  - `issuex/`
  - `jobqueue/`

  There is no `cmd/`. There is no `internal/` package unless R4 requires one.
- **Network.** `go build ./...` and `go test -short ./...` succeed from a clean checkout. The only network access they need is to fetch the two required modules and their requirements through the Go module proxy. No test needs a forge, a token or a sibling repository.

### R2. Importing `issuex`

- **Source.** Copy `issuex/` from `https://github.com/agent-fox-dev/agent-fox` at commit `84fb9b4`, resolved to its full hash. Extract it from git objects at that commit, for example with `git archive <hash> issuex`. Never copy from a working tree, which may have moved on.
- **If the source is unreachable.** If the commit cannot be fetched, stop. Do not reconstruct the package from memory, from documentation, or from a different commit.
- **What is copied.** Every file in the directory goes to `issuex/` at the same relative path, including `testdata/` and any non-Go files. The package name stays `issuex`, and the external test package stays `issuex_test`.
- **Non-test files** are not edited (see R4 for the only exceptions).
- **Test files.** The only edit is to rewrite imports of the source package's path (`github.com/agent-fox-dev/agent-fox/issuex`, and any sub-path) to `github.com/agent-fox-dev/aflib/issuex`.
- **Commit.** The package arrives in its own commit. That commit contains only the package's files, the import edits and any `go.mod`/`go.sum` changes they need. Its message names the source repository and the full commit hash.
- **Behaviour kept.** The environment-driven detection stays, including the `git remote get-url` fallback and the `gh` CLI probe, and so does the precedence of explicit `Options` over the environment. The GitLab smoke test is skipped unless its environment variable is set, exactly as in the source.

### R3. Importing `jobqueue`

- **Source.** Copy `internal/jobqueue/` from `https://github.com/agent-fox-dev/hub` at commit `ce8e1b8` into `jobqueue/`. The extraction rules of R2 apply, including stopping if the commit is unreachable.
- **Edits.** Non-test files are not edited. The only edit to test files is to rewrite `github.com/agent-fox-dev/hub/internal/jobqueue` to `github.com/agent-fox-dev/aflib/jobqueue`.
- **Commit.** The package arrives in its own commit, which names the source repository and the full hash.
- **The SQLite driver** `modernc.org/sqlite` is imported only from `_test.go` files.
- **Caller contract kept.** The caller owns the `*sql.DB` and configures WAL and `busy_timeout`, and handlers must be idempotent. The package comment states this already, and `docs/architecture.md` repeats it.

### R4. Findings during the import

The import is a move, not a rewrite. When copied code does not work in its new home, the case is handled as follows, and every case is listed in the erratum (R9):

- **No test is deleted, skipped, `t.Skip`-gated or weakened** to make the build pass.
- **A non-test file imports another package of its source module.** This blocks the work. Do not copy that package and do not edit the file. Stop, record the finding, and report it in the handoff.
- **A test file imports a test helper package of its source module.** Copy the files of that helper the tests need into `internal/<helper>/` in this module, so the test files' edits stay import-path-only. Record the copy in `docs/architecture.md`.
- **A copied test needs a module outside the allowlist** (for example, a property-testing library). Add that module as a test-only dependency. Record a ruling naming it in `docs/architecture.md`, and add it to the test allowlist of R6.
- **`gofmt`, `go vet` or `golangci-lint` rejects copied code.**
  - Prefer a configuration-level fix: a path-scoped exclusion in `.golangci.yml`.
  - Where no configuration fix exists (`gofmt` output, `go vet`), make the smallest edit that passes. The edit must not change any exported declaration.
- **A test fails in the new location for an environmental reason**, such as missing testdata. Fix the cause outside the test. If that is not possible, the failure blocks the work.

### R5. Exported-API golden

- **What the test does.** A Go test at the module root, `TestExportedAPI` in package `aflib_test`, finds every public package of the module. For each one, it renders the exported declarations and compares them with a committed golden file.
- **Which directories count as public packages.** A directory counts when it contains non-test Go files for the host build context. It is skipped when it is `testdata`, starts with `.` or `_`, is `docs` or `examples`, or has an `internal` path element.
- **Golden files** live at `testdata/api/<name>.golden`. `<name>` is the package's directory relative to the module root with `/` replaced by `_`, and the root package's name is `aflib`.
- **How declarations are rendered.**
  - Use the standard library: `go/build` to select files, `go/parser`, `go/doc`, and `go/format`. Do not exec `go doc`.
  - Include only exported identifiers, as `go/doc` filters them by default.
  - Order: constants, variables, functions, then types, with each type followed by its associated constants, variables, functions and methods, in the order `go/doc` yields them.
  - Format each declaration with `go/format`, separated by a blank line, with doc comments, field comments and line comments removed.
  - Leave out the package clause and the import path.
  - A package with no exported declarations produces an empty golden. The root package's golden is empty.
- **Failures.**
  - A mismatch fails with the package name, the first differing line, and the command that regenerates the goldens.
  - A discovered package without a golden fails, naming the package.
  - A golden with no matching package fails, naming the file.
- **Regenerating.** Running the test with `-update`, or `make api-golden`, rewrites the goldens. The first goldens are the ones the import produces. Because the non-test files are byte-identical to the source (R2, R3), these goldens equal the source packages' exported declarations at the named commits.

### R6. Dependency guard

A root-level test, `TestDependencies`, fails in each of these cases:

- `go.mod` contains a `replace` directive.
- `go.mod` directly requires (without `// indirect`) a module outside the allowlist {`github.com/google/uuid`, `modernc.org/sqlite`}, plus any module added under R4.
- A non-test file of a public package imports something other than the standard library, `github.com/google/uuid`, or another package of this module. An import counts as standard library when the first element of its path contains no dot.
- A test file, or a file of an `internal/` package, imports something other than the standard library, this module's packages, or the test allowlist {`github.com/google/uuid`, `modernc.org/sqlite`} plus any module added under R4.

Each failure names the file and the offending directive or import. The test does not depend on third-party modules or on the network.

### R7. Makefile

The `Makefile` provides the targets in `AGENTS.md`'s table, plus `test-short` and `api-golden`:

- `help`
- `build`: `go build ./...`
- `test`: `go test ./... -count=1`
- `test-short`: `go test -short ./...`
- `coverage`
- `fmt`: `gofmt` in place over all tracked Go files
- `vet`: `go vet ./...`
- `lint`: runs `golangci-lint run` when it is on `PATH`. Otherwise it prints that it was skipped and succeeds.
- `check`: `fmt`, `vet`, `lint`, `test-short`, in that order, stopping at the first failure
- `check-release`: `fmt`, `vet`, `lint`, `test`
- `doc`: writes `go doc -all <importpath>` for each public package to `docs/generated/<name>.txt`, using the same names as R5
- `tidy`: `go mod tidy`
- `clean`
- `api-golden`: regenerates the R5 goldens

This is a single module, so there is no nested-module loop.

### R8. Lint configuration

`.golangci.yml` is hub's `.golangci.yml` at `ce8e1b8`, minus any rule that names a hub path. `jobqueue` therefore passes it as it did in hub. Any findings in the copied `issuex` code are excluded by path-scoped rules, each with a comment pointing at the erratum, as R4 requires. The copied code is not edited to satisfy the linter.

### R9. Reference docs, generated docs, erratum

- **`docs/api.md`.** Says the repository has no REST API. Maps the public packages (`aflib`, `issuex`, `jobqueue`) with each one's purpose and principal exported identifiers, taken from the copied code, and points to `docs/generated/`.
- **`docs/architecture.md`.** Covers:
  - what each package is and where it came from (repository and full hash)
  - what imports what: neither package imports the other; `issuex` uses only the standard library; `jobqueue` uses the standard library and `uuid`; the tests also use `modernc.org/sqlite`
  - the dependency allowlist, and the rule that adding a module needs a ruling here first
  - the test layout: tests next to the code, root guard tests, goldens in `testdata/api/`
  - `jobqueue`'s caller contract
  - versioning: `vX.Y.Z` tags, the first being `v0.1.0`, cut by a human once both consumers are switched. Before 1.0, a breaking change bumps the minor version and is recorded in the errata.
  - any `internal/` helpers added under R4.
- **`docs/cli.md`.** One line: the repository has no command.
- **`docs/configuration.md`.** The environment variables `issuex` reads and their precedence below explicit `Options`, the `git` and `gh` fallbacks, and the fact that hub sets everything explicitly. States that `jobqueue` reads no environment. The variable names must be checked against the copied code: list every variable it reads, not only the four named above.
- **`docs/generated/`.** `aflib.txt`, `issuex.txt` and `jobqueue.txt`, produced by `make doc` and committed.
- **`docs/errata/01_import_issuex_and_jobqueue.md`.** Always written. It records:
  - the source repository and full hash of each package
  - that the requirement IDs in the copied test names and comments (`10-REQ-6.E3`, `01-REQ-4.8`, …) refer to the source repositories' spec archives. They are not this repository's specs, even where the numbers collide with this spec's own IDs.
  - every R4 finding, or the statement that there were none.
- **`README.md`.** Explains the module, its two packages, how to require it (`go get github.com/agent-fox-dev/aflib`), the quality commands, and links to the docs.

### R10. Agent policy files

- **`AGENTS.md`.** Only the Repository Layout section is rewritten. It now describes this repository: `doc.go` (root package, doc only), `issuex/`, `jobqueue/`, the root guard tests and `testdata/api/`, `docs/` and `docs/generated/`, `.specs/`, and tests living next to the code. The shared sections stay unchanged.
- **`CLAUDE.md`** has the same content as `AGENTS.md` afterwards.
- **`.specs/steering.md`.** Only Repository Specifics is rewritten. It says aflib is a library with no REST API and no command, that `docs/api.md` is the map of its public packages, and that it is a single module with no nested modules. The shared sections stay unchanged.

## Design Decisions

1. **This spec covers only the work inside this repository.** The hub and agent-fox switches change other repositories and belong to their own specs. Tagging `v0.1.0` is a human release step that depends on both switches.
2. **The packages arrive as a plain copy, not a history import.** Each arrives in one commit naming the source repository and full hash. This keeps this repository's history its own, and the source history stays reachable by hash. It resolves the input's first open question.
3. **Files are extracted from git objects at the named commit, never from a working tree.** A sibling checkout may have drifted, and byte identity is only checkable against a fixed commit.
4. **There is a root `doc.go` with a doc-only package `aflib`.** The input asks for "no root package" and also for a `doc.go` that names the module. A comment-only package satisfies both, and its empty golden enforces that nothing is ever declared at the root.
5. **The golden is rendered with `go/doc` and `go/format` in a Go test, not by running `go doc`.** This avoids exec in tests and dependence on the `go doc` output format across toolchains. Stripping comments in the AST is exact, where filtering text would be fragile.
6. **The golden test discovers packages instead of listing them.** A new public package then fails `make check` until it gets a golden, which enforces the rule that a third package is a deliberate decision.
7. **`uuid` and `modernc.org/sqlite` are pinned at hub's versions at `ce8e1b8`.** The copied tests have passed at exactly those versions, and hub's module graph does not move when it switches.
8. **`modernc.org/sqlite` stays test-only by import, but is listed in `go.mod`.** Go modules have no test scope. Consumers are unaffected because `jobqueue` takes a `*sql.DB`. This resolves the input's second open question.
9. **The `os/exec` fallbacks in `issuex` stay, with no build tag.** Hub passes everything explicitly and never reaches them, and `af --local` needs them. This resolves the input's third open question.
10. **A dependency-guard test is added.** The input states an allowlist rule and a no-`replace` rule but gives no way to enforce them. A test in `make check` makes both checkable.
11. **Findings follow fixed rules (R4) instead of being judged case by case.** An unattended run needs a deterministic answer. The rules keep test edits import-path-only and never trade a test away to get a green build.
12. **A test helper from a source repository is copied to `internal/`, not inlined.** This keeps the copied test files' edits import-path-only. `internal/` is not public API and is outside the golden and `make doc`.
13. **Lint failures in copied code are excluded in configuration; code is edited only where no configuration fix exists.** "A move, not a rewrite" outranks lint cleanliness, and the exclusions remain visible as a to-do list for a later spec.
14. **`.golangci.yml` is taken from hub at `ce8e1b8`.** It is a sibling agent-fox configuration that `jobqueue` already passes, so only `issuex` findings can appear.
15. **Doc comments that mention hub or agent-fox are left as they are.** Byte identity of non-test files is what makes the move reviewable and the golden trustworthy.
16. **The requirement IDs in copied tests stay, and the erratum explains them.** Renaming them would change the tests, and this spec's own `01-REQ-*` IDs collide with hub's, so the note is needed.
17. **The erratum is `docs/errata/01_import_issuex_and_jobqueue.md`.** That follows `AGENTS.md`'s `NN_snake_case_topic.md` rule for the first spec.
18. **There is no test of the docs or the `Makefile`.** Steering forbids tests of doc content. Running `make check` and `make doc` verifies the `Makefile`.

## Verified External API

None of the external code below lives in this repository, so none of it could be read. Everything here is **unverified**. New tests must not call `issuex` or `jobqueue` APIs with assumed signatures. The copied tests are the behaviour tests, and R5's golden is generated from the code, not written by hand.

| Package | Symbols assumed | Status |
|---|---|---|
| `issuex` (agent-fox `issuex/` @ `84fb9b4`) | `Client` (alias `ForgeClient`), `New`, `NewWithOptions`, `NewGitHub`, `NewGitLab`, `NewNoOp`, `Options{BaseURL, Token, HTTPClient, Repo, RemoteURL, Dir, NoOp}`, `DetectForge`, `ForgeType`, `RegisterAdapter`, `ParseRemote`, `ParseIssueURL`, `ErrAmbiguousForge`, `ErrUnsupportedForge`, `GitHubMaxBodyLength`, `GitLabMaxBodyLength`, and the vocabulary types listed in the input | unverified (from the input). Copied, not called. |
| `jobqueue` (hub `internal/jobqueue/` @ `ce8e1b8`) | `New(db, logger, opts...)`, `WithWorkers`, `WithPollInterval`, `WithGracePeriod`, `InitSchema`, `MigrateGroupKey`, `MigrateProgress`, `Register`, `HandlerFunc`, `RetryPolicy`, `DefaultRetryPolicy`, `Enqueue(EnqueueParams)`, `Start`, `Shutdown`, `Wait`, `Stop`, `CancelJob`, `RequeueDeadLetter`, `UpdateProgress`, `GetByID`, `ListByType`, `ListByKey`, `ListByKeyPrefix`, `CountByStatus`, `Job`, `ListOpts`, `Status*`, `ErrNotCancellable`, `JobIDFromContext`, `ContextWithJobID` | unverified (from the input). Copied, not called. |
| `github.com/google/uuid` | whatever `jobqueue` calls (job IDs) | unverified. Version is hub's at `ce8e1b8`. |
| `modernc.org/sqlite` | registers the `database/sql` driver `"sqlite"` (blank import in tests) | unverified. Test-only. Version is hub's at `ce8e1b8`. |
| `golangci-lint` (tool) | `golangci-lint run` with `.golangci.yml` | unverified. Version and config format follow hub's config. |
| Standard library (R5, R6) | `build.ImportDir(dir string, mode build.ImportMode) (*build.Package, error)`; `parser.ParseFile(fset *token.FileSet, filename string, src any, mode parser.Mode) (*ast.File, error)`; `doc.NewFromFiles(fset *token.FileSet, files []*ast.File, importPath string, opts ...any) (*doc.Package, error)`; `format.Node(dst io.Writer, fset *token.FileSet, node any) error` | standard library, stable. Not read in the repository. Do not use `parser.ParseDir`/`ast.Package`, which are deprecated. |
