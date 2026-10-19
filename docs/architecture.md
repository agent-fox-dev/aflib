# Architecture

aflib is one Go module, `github.com/agent-fox-dev/aflib`, that holds the packages
more than one agent-fox program needs. It has three public packages and no
command. [`api.md`](api.md) maps their exported identifiers.

```
github.com/agent-fox-dev/aflib
├── doc.go          the root package aflib: a doc comment, no declarations
├── issuex/         the forge client, its tests
├── jobqueue/       the job queue, its tests
├── api_test.go     root guard test: the exported-API golden
├── deps_test.go    root guard tests: the dependency guard and the layout
├── testdata/api/   the exported-API goldens, one per public package
├── testdata/fixtures/   fixtures of the guard tests
├── docs/           reference docs, generated Go docs, errata, PRDs
├── .specs/         the spec packages
└── Makefile        the quality gate
```

## Packages and where they come from

Both packages were copied, not rewritten: the non-test files are byte-identical
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

## Import graph

- `issuex` imports the **standard library only** (`net/http`, `net/url`,
  `encoding/json`, and `os/exec` for the `git remote get-url` fallback).
- `jobqueue` imports the standard library and `github.com/google/uuid`.
- **Neither package imports the other**, and neither imports anything from hub,
  agent-fox or AgentKit.
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

- **Tests live next to the code**: `issuex/*_test.go` and `jobqueue/*_test.go`.
  They need no network, token, forge or sibling repository. The `issuex` tests
  use `httptest` servers on the loopback interface; the `jobqueue` tests use an
  in-memory SQLite database. No test checks `-short`, so `make test-short` and
  `make test` run the same tests.
- **Root guard tests**, package `aflib_test`:
  - `api_test.go`: `TestExportedAPI` renders the exported declarations of every
    public package (`go/build`, `go/parser`, `go/doc`, `go/format`, no `go doc`)
    and compares them with `testdata/api/<name>.golden`, where `<name>` is the
    package's directory with `/` replaced by `_` and `aflib` for the root. A
    mismatch, a package without a golden and a golden without a package all fail
    by name. `make api-golden` (`go test -run TestExportedAPI . -update`)
    rewrites the goldens on purpose.
  - `deps_test.go`: `TestDependencies` and `TestPackageLayout_TS_01_2`.
- **Fixtures** of the guard tests are in `testdata/fixtures/`.
- Go files under `testdata/` are ignored by the go tool, and `make fmt` formats
  them like any tracked Go file.

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
