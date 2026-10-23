# Architecture

aflib is one Go module, `github.com/agent-fox-dev/aflib`, that holds the packages
more than one agent-fox program needs. It has four public packages and one
command, `issue`. [`api.md`](api.md) maps the packages' exported identifiers and
[`cli.md`](cli.md) is the command's reference.

```
github.com/agent-fox-dev/aflib
├── doc.go          the root package aflib: a doc comment, no declarations
├── envelope/       the JSON envelope of the agent-fox commands, its tests
├── issuex/         the forge client, its tests
├── jobqueue/       the job queue, its tests
├── cmd/issue/      the issue command, its tests and its --schema golden
├── api_test.go     root guard test: the exported-API golden
├── deps_test.go    root guard tests: the dependency guard and the layout
├── testdata/api/   the exported-API goldens, one per public package
├── testdata/fixtures/   fixtures of the guard tests
├── docs/           reference docs, generated Go docs, ADRs, errata, PRDs
├── .specs/         the spec packages
└── Makefile        the quality gate
```

## Packages and where they come from

`issuex` and `jobqueue` were copied, not rewritten: the non-test files are byte-identical
to their source, and the test files differ only where an import path had to
change. Each arrived in one commit that names the source repository and commit.
[`errata/01_import_issuex_and_jobqueue.md`](errata/01_import_issuex_and_jobqueue.md)
records the sources, the findings of the import and where the sources differed
from what the spec assumed.

| Package | Source repository | Directory | Commit |
|---|---|---|---|
| `issuex` | https://github.com/agent-fox-dev/agent-fox | `issuex/` | `84fb9b4785ba80a82559e8955aba25c433d6f89a` |
| `jobqueue` | https://github.com/agent-fox-dev/hub | `internal/jobqueue/` | `ce8e1b873b826b0ac319b28f16a09e30acb014b0` |

The requirement IDs in the copied tests and comments (`10-REQ-6.E3`,
`01-REQ-4.8`, `TS_03_29`, ...) refer to the spec archives of those repositories,
not to the specs of this one. The erratum says which archive each prefix belongs
to.

`envelope` and `cmd/issue` were written here, from
[PRD 02](prd/02-ship-an-issue-command.md), with no spec package;
[ADR 01](adr/01-ship-a-command-for-the-forge-client.md) is the decision to give
the module a command.

## Import graph

- `issuex` imports the **standard library only** (`net/http`, `net/url`,
  `encoding/json`, and `os/exec` for the `git remote get-url` fallback).
- `jobqueue` imports the standard library and `github.com/google/uuid`.
- **Neither package imports the other**, and neither imports anything from hub,
  agent-fox or AgentKit.
- `envelope` imports the standard library and `issuex`, whose value types its
  `IssueResult` reuses.
- `cmd/issue` imports the standard library, `envelope` and `issuex`. It adds no
  module.
- The tests of `jobqueue` also import `modernc.org/sqlite`, the pure-Go SQLite
  driver, which they open as the `database/sql` driver `sqlite`. No non-test file
  imports it.
- The root guard tests import the standard library only.

## Dependency allowlist

`go.mod` requires two modules directly, both at the versions hub pinned at
`ce8e1b8`:

| Module | Allowed in | Used by |
|---|---|---|
| `github.com/google/uuid` | non-test code, tests | `jobqueue/jobqueue.go` |
| `modernc.org/sqlite` | tests and `internal/` packages only | `jobqueue/testhelpers_test.go`, `jobqueue/recovery_test.go` |

Non-test code may import the standard library, `github.com/google/uuid` and the
packages of this module. Test files and `internal/` packages may also import
`modernc.org/sqlite`. There is no `replace` directive and no `go.work` is
committed.

`TestDependencies` (`deps_test.go`) enforces this inside `make check`: it fails
on a `replace` directive, on a direct requirement outside the allowlist, and on
an import outside it, naming the file and the directive or import.

**A new module needs a ruling here first.** Add a section under *Rulings* that
names the module, why it is needed and whether it is test-only; then add it to
`go.mod` and to `ruledIn` in `deps_test.go`. There are no rulings yet: no module
beyond the two above has been needed.

## Tests

- **Tests live next to the code**: `issuex/*_test.go`, `jobqueue/*_test.go`,
  `envelope/*_test.go` and `cmd/issue/*_test.go`. They need no network, token,
  forge or sibling repository. The `issuex` and `cmd/issue` tests use `httptest`
  servers on the loopback interface (the command's tests point `GITHUB_API_URL` or
  `GITLAB_API_URL` at a fake forge that records every request, and run `git` to
  make an origin remote where one is needed); the `jobqueue` tests use an
  in-memory SQLite database. `cmd/issue/testdata/schema.golden.json` pins
  `issue --schema`; `UPDATE_GOLDEN=1 go test -run TestSchemaGolden ./cmd/issue`
  rewrites it. No test checks `-short`, so `make test-short` and
  `make test` run the same tests.
- **Root guard tests**, package `aflib_test`:
  - `api_test.go`: `TestExportedAPI` renders the exported declarations of every
    public package (`go/build`, `go/parser`, `go/doc`, `go/format`, no `go doc`;
    a `package main` is a command, not a public package, and has no golden)
    and compares them with `testdata/api/<name>.golden`, where `<name>` is the
    package's directory with `/` replaced by `_` and `aflib` for the root. A
    mismatch, a package without a golden and a golden without a package all fail
    by name. `make api-golden` (`go test -run TestExportedAPI . -update`)
    rewrites the goldens on purpose.
  - `deps_test.go`: `TestDependencies` and `TestPackageLayout_TS_01_2`, which
    pins the public packages (`aflib`, `envelope`, `issuex`, `jobqueue`) and the
    commands under `cmd/` (`issue`).
- **Fixtures** of the guard tests are in `testdata/fixtures/`.
- Go files under `testdata/` are ignored by the go tool, and `make fmt` formats
  them like any tracked Go file.

## The issue command

`cmd/issue` is `issuex` with a shell around it, in the interface of the
agent-fox commands ([`cli.md`](cli.md)). A call goes through these steps, in
`cmd/issue`:

1. **Flags** (`options.go`). The flags are parsed wherever they stand beside the
   one input; the form is the one form flag given; `check` refuses, before
   anything is read, two forms, a flag of another form, a value outside its set,
   a missing required flag and an input of the wrong kind. A refusal is a usage
   envelope, exit 2.
2. **Reference** (`ref.go`). The input becomes a target: a repository, and a
   number for a thread, with whether it is known to be a pull request. A URL is
   read with `issuex.ParseIssueURL` or `ParseRemote`, `owner/repo` with
   `ParseRepo`.
3. **Forge** (`forge.go`). A target with no repository takes the origin
   remote's (`issuex.DetectRepo`); `issuex.DetectForge` and `NewWithOptions`
   choose the forge and build the client, so the token goes only where
   `issuex` sends it. `--forge` builds `NewGitHub` or `NewGitLab` directly, for a
   repository with no host only.
4. **The form** (`call.go`). A read calls the client and fills the result. A
   write form checks for a token, reads the thread (to know whether it is a pull
   request and its state) or, under `--dry-run`, the repository, then makes its
   writes in order, recording each as a side effect, and stops at the first
   that fails. `--dry-run` lists them instead. `--edit`, `--close` and
   `--reopen` read the thread back.
5. **The envelope** (`run.go`). Every path a program reads ends in one
   `envelope.IssueEnvelope` on stdout; `classify` maps `issuex`'s errors to a
   stage, a category and a fix hint.

The command's version is set by `make install` with `-ldflags` (`main.version`,
`main.commit`, `main.buildTime`); a plain `go install` reports the module
version Go recorded.

## jobqueue caller contract

`jobqueue.New` takes a `*sql.DB` the **caller owns**; the queue never closes it
and registers no SQL driver. The caller:

- configures `PRAGMA journal_mode=WAL` and `PRAGMA busy_timeout` on the
  database **before** calling `InitSchema`. `InitSchema` does not set WAL mode,
  and concurrent workers may meet `SQLITE_BUSY` under load without it
  (`jobqueue/schema.go`);
- writes **idempotent handlers**: the crash recovery `Queue.Start` performs may
  dispatch a job whose handler had already partly run (the doc of `HandlerFunc`
  in `jobqueue/jobqueue.go`).

A consumer on another SQLite driver is unaffected, because only the tests name a
driver.

## Versioning

The module is versioned with `vX.Y.Z` tags. There are none yet. The first,
`v0.1.0`, is cut by a human, as a release step, once hub and agent-fox have both
switched to this module and dropped their copies. Before 1.0, a breaking change
bumps the minor version and is recorded in the errata under `docs/errata/`.

## internal/ helpers

There is no `internal/` package: no copied test needed a helper from its source
repository. A helper copied from one under 01-REQ-4.3 of the first spec would
live in `internal/<helper>/`, be left out of the goldens and of `make doc`, and
be recorded here and in the erratum.

## Rulings

None.
