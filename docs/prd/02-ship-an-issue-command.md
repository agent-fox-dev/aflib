# Ship an issue command

Status: **accepted** (2026-10-23). Implemented directly from this document;
no spec package was written for it. The decision to give aflib a command is
[ADR 01](../adr/01-ship-a-command-for-the-forge-client.md).

## Intent

`issuex` gives a Go program one `Client` over GitHub and GitLab: repositories,
issues, labels, comments and pull or merge requests. An agent with a shell, or
a person, cannot reach any of it without writing a program. The forges' own
CLIs (`gh`, `glab`) are two tools with two interfaces, two output formats and
two ways to fail, and an agent that drives both has to know which forge it is
on before it can ask anything.

`issue` is a command over `issuex`, in the shape the agent-fox commands share
(`spec`, `specr`, `speca` in the spec repository): exactly one positional
input, the forms chosen by flags rather than subcommands, exactly one JSON
object on stdout on every path a program reads, the help on stderr, the exit
codes 0, 1 and 2, `--schema` and `--version`. An agent that can drive `speca`
can drive `issue`, and the same call works on either forge.

The object it writes is described by a new public package, `envelope`, so that
a Go program that runs `issue` decodes its output into types instead of
writing its own.

## Goals

1. Every operation of `issuex.Client` is reachable from a shell, on GitHub and
   GitLab alike, with one call shape for both.
2. The output is one JSON envelope with the fields and meanings of the spec
   repository's envelope (`tool`, `version`, `ok`, `status`, `exit_code`,
   `summary`, `error`, `warnings`, `result`, `side_effects`, `next`,
   `duration_ms`, `started_at`), so a caller that reads one agent-fox command
   reads this one.
3. Every write to a forge is listed in `side_effects`, in the order it was
   made, including a write that failed, so a call that fails halfway says
   exactly what landed.
4. `--dry-run` runs every check and read a write form makes and writes nothing.
5. A Go program decodes the output with `envelope.DecodeIssue`.
6. The command adds no dependency: the standard library and `issuex` only.

## Non-goals

- **Credentials on the command line.** There is no `--token`: a token in argv
  is visible in the process list. The command reads the environment
  `issuex` reads and nothing else.
- **Changing `issuex`.** Its limits are the command's limits, and are
  documented, not worked around: `UpdateIssue` cannot clear a body or remove
  every label (empty fields are not sent), and the GitLab adapter drops
  assignees.
- **Interactive use.** There is no prompt, no editor, no pager and no colour.
  A body comes from a flag, a file or stdin.
- **Forges other than GitHub and GitLab**, and API calls `issuex` does not have
  (milestones, projects, line comments on a diff).

## The interface

```
issue [flags] <ref>                         an issue or pull request → its thread
issue [flags] <repo>                        a repository → its metadata
issue [<repo>] --list [filters]             the issues of a repository
issue [<repo>] --create --title T [body]    open an issue
issue <ref> --comment <body>                comment on an issue or pull request
issue <ref> --edit [changes]                change title, body, labels, assignees
issue <ref> --close [body] | --reopen       close (with an optional comment) or reopen
issue [<repo>] --create-label NAME          create a label
issue [<repo>] --create-pr --head H --base B --title T [body]
issue <pr> --review <body>                  post a review comment on a pull request
issue <pr> --merge [--method M]             merge a pull request
```

A body is `--body TEXT` or `--body-file PATH`, where `-` is stdin.

### The reference

The one positional input names an issue, a pull request or a repository:

| Input | Names |
|---|---|
| a web URL of an issue, pull request or merge request | that thread, on the URL's host |
| `owner/repo#N` | issue (or GitHub pull request) N |
| `owner/repo!N` | pull or merge request N |
| `N`, `#N`, `!N` | the same, in the repository of the origin remote of `--dir` |
| `owner/repo`, `group/sub/project` | a repository |
| a repository URL or a git remote URL | a repository, on its host |
| nothing | the repository of the origin remote of `--dir` (only for the forms that take a repository) |

On GitHub an issue and a pull request share their numbers, so `#N` may be a
pull request; the read tells from the forge's answer. On GitLab they do not,
and `!N` names a merge request.

### Forms

At most one form flag per call; none is the read. A flag that does not belong
to the form is a usage error naming it.

| Form | Input | `issuex` calls | `result` |
|---|---|---|---|
| read | issue or pull request | `ReadIssue`; for a pull request also `ReadPullRequest`, and `GetCIChecks`, `GetPRReviews`, `ReadChangedFiles` under `--checks`, `--reviews`, `--files` | `issue`, `comments`, `pull_request`, `checks`, `reviews`, `files` |
| read | repository | `GetRepository` | `repository` |
| `--list` | repository | `ListIssues` with `--state`, `--label`, `--assignee`, `--sort`, `--direction`, `--limit` | `issues`, `incomplete` |
| `--create` | repository | `CreateIssue` | `issue` |
| `--comment` | thread | `AddComment` | `comment_url` |
| `--edit` | thread | `UpdateIssue` for title, body and assignees, `AddLabels`, `RemoveLabel` | `issue`, read back |
| `--close` | thread | `CloseIssue` (or `AddComment` and `ClosePullRequest` for a pull request) | `issue`, read back |
| `--reopen` | thread | `UpdateIssue` with state `open` | `issue`, read back |
| `--create-label` | repository | `CreateLabel` | `label` |
| `--create-pr` | repository | `CreatePullRequest` | `pull_request` |
| `--review` | pull request | `PostReviewComment` | — |
| `--merge` | pull request | `MergePullRequest` | `merge` |

Closing a closed thread and reopening an open one change nothing and succeed.

### Output, errors, exit codes

- `result` also names the `forge`, the `repo` and, for a thread, the `ref`
  (`owner/repo#N` or `owner/repo!N`).
- A write form under `--dry-run` reads what it would read (the thread must
  exist; close and reopen read its state), writes nothing, and reports
  `dry_run: true` and `planned`: the writes it would make. Its `next` is the
  same call without `--dry-run`.
- Exit 0 `done`, 1 `failed`, 2 `usage`. A usage error sends nothing to the
  forge.
- `error.category` follows the shared vocabulary: `usage`, `auth` (no token,
  or 401/403; `fix_hint.env` names the variables), `input` (not found, 400 or
  422, a forge that cannot be told), `conflict` (a merge that cannot be made),
  `api` (rate limit, 5xx, transport; retryable), `aborted` (`--timeout`;
  retryable).
- Warnings: `comments_unreadable` (the thread was read and its comments were
  not), `comments_truncated` (`issuex` reads at most 500), `assignees_ignored`
  (GitLab drops assignees).

### Flags shared by every form

`--dir` (the working tree whose origin remote is used, default `.`), `--forge
github|gitlab` (for when the forge cannot be told otherwise), `--timeout`
(default 2m), `--dry-run`, `--schema`, `--version`.

## The envelope package

`github.com/agent-fox-dev/aflib/envelope` holds the envelope as Go types:
`Envelope[R]`, `ErrorInfo`, `FixHint`, `Warning`, `SideEffect`, `Next`, the
exit codes and categories, `Decode` and `Encode`, and the `issue` command's
result, `IssueResult`, with `IssueEnvelope` and `DecodeIssue`. It imports the
standard library and `issuex` (whose value types the result reuses). Its JSON
is the spec repository's envelope minus the fields only a model-driven command
writes (`artifacts`, `input`, `model`, `usage`): a decoder of either reads the
other.

## Done when

- `cmd/issue` builds with `make build`, `make install` installs it stamped with
  `git describe`, and `make check` passes.
- Every form is tested against `httptest` stand-ins for GitHub and GitLab, with
  no network and no token of the person running the tests.
- `--schema` is pinned by a golden file.
- `docs/cli.md` is the command's reference; `README.md`, `api.md`,
  `architecture.md`, `configuration.md`, the steering file's repository
  specifics and `AGENTS.md` describe the module with its command.
