# Erratum: importing issuex and jobqueue (spec 01)

Spec: `.specs/01_import_issuex_and_jobqueue`. This file records where the two
packages come from, what the requirement IDs in the copied tests refer to,
every finding of the import (01-REQ-4), and each place where the sources
differ from what the spec assumed about them.

## Sources

| Package | Repository | Directory | Commit |
|---|---|---|---|
| `issuex` | https://github.com/agent-fox-dev/agent-fox | `issuex/` | `84fb9b4785ba80a82559e8955aba25c433d6f89a` (`84fb9b4`) |
| `jobqueue` | https://github.com/agent-fox-dev/hub | `internal/jobqueue/` | `ce8e1b873b826b0ac319b28f16a09e30acb014b0` (`ce8e1b8`) |

Each package was extracted with `git archive` from the objects of its commit,
never from a working tree, and arrived in one commit that names the repository
and the full hash. All 16 non-test `.go` files of `issuex` and all 3 of
`jobqueue` are byte-identical to the source. Of the 23 `issuex` test files,
the 9 external ones differ by one import line each (E1); none of the 19
`jobqueue` test files differs (E2).

## Requirement IDs in the copied tests

The copied code, tests and comments carry requirement and test IDs such as
`01-REQ-4.8` (`issuex/factory.go:39`), `TS_01_19` (`issuex/registry_test.go:31`)
and `10-REQ-6.E3` (`jobqueue/backoff_test.go:238`). They refer to the spec
archives of the source repositories, not to the specs of this repository:

| IDs | Spec archive |
|---|---|
| `01-…`, `TS_01_…` in `issuex` | agent-fox, `.specs/archive/01_issuex_core` |
| `02-…`, `TS_02_…` in `issuex` | agent-fox, `.specs/archive/02_issuex_github` |
| `03-…`, `TS_03_…` in `issuex` | agent-fox, `.specs/archive/03_issuex_gitlab` |
| `10-…` in `jobqueue` | hub, `.specs/archive/10_durable_job_queue` |
| `12-…` in `jobqueue` | hub, `.specs/archive/12_merge_operations` |

The numbers collide with this repository's own. This repository's first spec
is `01_import_issuex_and_jobqueue` and uses `01-REQ-*` and `TS-01-*`; the
`01-REQ-4.8` above is agent-fox's requirement about an unsupported forge, not a
criterion of this spec. The IDs stay as they are, because renaming them would
change the copied files. New specs here use this repository's numbering.

## Findings (01-REQ-4)

There were no findings. For each rule:

- **01-REQ-4.1, no test removed or weakened.** The names of the `Test*`,
  `Fuzz*` and `Example*` functions are the same sets as at the source commits:
  121 in `issuex`, 96 in `jobqueue`. No test is skipped or gated.
- **01-REQ-4.2, a non-test file importing another package of its source
  module.** None does. No file of either package imports anything from
  `github.com/agent-fox-dev/` other than this module.
- **01-REQ-4.3, a test helper package.** None was needed. No test file imports
  a source-module path, and there is no `internal/` directory.
- **01-REQ-4.4, a module outside the allowlist.** None was needed. The direct
  requirements of `go.mod` are exactly `github.com/google/uuid v1.6.0` and
  `modernc.org/sqlite v1.60.1`, the versions in hub's `go.mod` at `ce8e1b8`.
  `uuid` is imported by `jobqueue/jobqueue.go`; `modernc.org/sqlite` only by
  `jobqueue/testhelpers_test.go` and `jobqueue/recovery_test.go`.
- **01-REQ-4.5, lint findings in copied code.** None, with `.golangci.yml`
  (see below). No exclusion exists.
- **01-REQ-4.6, `gofmt` or `go vet` rejecting copied code.** Neither does, so
  no non-test file was edited.
- **01-REQ-4.7, a test failing in the new location.** None does. The sources
  have no `testdata/` directory and no non-Go file, and no test opens a
  `testdata` path. The short tests pass in a fresh clone with an empty module
  cache and none of the forge variables set.

## Lint configuration

`.golangci.yml` is hub's file at `ce8e1b8`, byte for byte. It names no hub
path, so no rule was removed, and no `issuex` exclusion was added. With it,
`golangci-lint` 2.15.0 reports 0 issues in `issuex` and `jobqueue`. Without a
configuration the same unedited code has 22 issues (18 `errcheck`, 4
`staticcheck`), which is why the file is part of the module from the start.

## Where the sources differ from what the spec assumed

**E1. The agent-fox module path has no hyphen.** The spec names
`github.com/agent-fox-dev/agent-fox/issuex` as the import path to rewrite. At
`84fb9b4` agent-fox's module is `github.com/agent-fox-dev/agentfox`, and the
nine external test files (`package issuex_test`) import
`github.com/agent-fox-dev/agentfox/issuex`, for example
`issuex/client_test.go:7` before the rewrite. That line is the one rewritten, to
`github.com/agent-fox-dev/aflib/issuex`. Strings such as
`agent-fox-dev/agentfox#42` in `issuex/smoke_test.go` are test data, not
imports, and are unchanged. Checks for imports of the source modules cover
every module under `github.com/agent-fox-dev/` other than this one.

**E2. No `jobqueue` test file needed an import edit.** All 22 files are
`package jobqueue`, tests included, and none imports hub's
`internal/jobqueue` path. The rewrite the spec describes has nothing to act on.

**E3. No GitLab smoke test is gated on an environment variable.** The spec
expects a smoke test that is skipped unless a variable is set. The four GitLab
smoke tests (`TestSmoke_GitLab…_TS_03_29` to `_TS_03_32`,
`issuex/gitlab_smoke_test.go:33`, `:274`, `:484`, `:553`) run against
`httptest` servers and pass under `-short`. The only skip in either package is
`t.Skip("git is not installed")` at `issuex/issue222_test.go:112`. A run with
git installed therefore has no skipped test, and the checks assert that, not a
skipped smoke test. No test was edited to make a skip appear.

**E4. There is no `gh` CLI probe.** The spec lists "the `gh` probe" among the
`issuex` fallbacks. The code has none: no `exec.LookPath`, and the only
subprocess is `git -C <dir> remote get-url origin` (`issuex/parse.go:142`).
What `issuex` reads from the environment is `GITHUB_API_URL`, `GITHUB_TOKEN`,
`GH_TOKEN`, `GITLAB_API_URL` and `GITLAB_TOKEN`
(`issuex/detect.go:118-119`, `:123`, `:137`, `:198-199`, `:221-226`, `:246`;
`issuex/github.go:32`, `:41`, `:43`; `issuex/gitlab.go:34`, `:46`;
`issuex/parse.go:46`, `:54`). `GH_TOKEN` is the variable the `gh` CLI uses, and
it is read as a token fallback; nothing runs `gh`. `docs/configuration.md`
describes what the code does.

**E5. Two files arrived earlier than the spec's task order.** `.golangci.yml`
and the root `doc.go` were added with the module skeleton, not later. With the
default lint configuration the unedited copies fail `make check` (22 issues),
and on Go 1.27 `go vet ./...` and `go test ./...` exit 1 on a module with no
package, so the skeleton could not pass its own gate without one. Both files
have the final content the spec fixes for them.

**E6. The `jobqueue` caller contract is not in the package comment.** The spec
says the package comment states that the caller configures WAL and
`busy_timeout` and that handlers are idempotent. The package comment is two
lines (`jobqueue/jobqueue.go:1`). The WAL and `busy_timeout` requirement is
stated in the comments on the `jobs` table DDL and on `InitSchema`
(`jobqueue/schema.go:9-12`, `:49-52`), and handler idempotency in the doc of
`HandlerFunc` (`jobqueue/jobqueue.go:33-37`). `docs/architecture.md` states the
contract and points to those places. `issuex` has no package comment at all, so
`docs/api.md` describes it from the doc of `Client`.

**E7. The golden pins declarations as written, not only their shape.** The
exported-API golden renders each exported declaration with `go/doc` and
`go/format`, after removing comments, source positions and the marker for
filtered fields, so that edits to comments and to unexported identifiers leave it
unchanged (01-REQ-5.8). Two things follow from rendering what is exported. The
initializer of an exported variable or constant is part of its declaration and is
rendered as written, so an edit inside it that names an unexported identifier
changes the golden; 01-REQ-5.8 holds for packages whose exported declarations do
not refer to unexported identifiers, which is what `TestRenderProperties_TS_01_32`
generates. And whether a type declaration is an alias (`type A = B`) is kept,
because `=` changes what the declaration means; the position that carries it is
the one position the renderer does not clear (`normalise` in `api_test.go`).

## Amendments after the spec

**A1. The module has a command and a fourth package (PRD 02).** 01-REQ-1.2 pins
the module to the packages `aflib`, `issuex` and `jobqueue` and says there is no
`cmd/`. [PRD 02](../prd/02-ship-an-issue-command.md) and
[ADR 01](../adr/01-ship-a-command-for-the-forge-client.md) amend it: the module
also has the public package `envelope` and one command, `cmd/issue`.
`TestPackageLayout_TS_01_2` keeps its ID and now pins the four packages and the
one command; `discover` (`api_test.go`) skips a `package main`, which has no
importable API and no golden, and `TestDiscover_TS_01_25` has a fixture for it
(`testdata/fixtures/discovery/cmd/tool`). `TestDocGo_TS_01_3` checks that
`doc.go` names `envelope` and `cmd/issue` as well.
