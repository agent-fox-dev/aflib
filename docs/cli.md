# The `issue` command

```
issue [flags] <ref>                         an issue or pull request → its thread
issue [flags] <repo>                        a repository → its metadata
issue [<repo>] --list [filters]             the issues of a repository
issue [<repo>] --create --title T [body]    open an issue
issue <ref> --comment <body>                comment on a thread
issue <ref> --edit [changes]                change title, body, assignees, labels
issue <ref> --close [body] | --reopen       close (after a comment) or reopen
issue [<repo>] --create-label NAME          create a label
issue [<repo>] --create-pr --head H --base B --title T [body]
issue <pr> --review <body>                  post a review comment
issue <pr> --merge [--method M]             merge a pull request
```

`issue` is aflib's one command (`cmd/issue`). It reads and writes the issues,
pull requests and repositories of GitHub and GitLab through `issuex`, with one
call shape for both forges. It is built for an agent to call, and follows the
interface of the agent-fox commands (`spec`, `specr`, `speca`): **at most one
positional input**, the forms chosen by **flags, not subcommands**, **exactly
one JSON object** on stdout on every path a program reads, the help on stderr,
and one exit-code table. A call with no form flag only reads. The decision is
[ADR 01](adr/01-ship-a-command-for-the-forge-client.md); the requirements are
[PRD 02](prd/02-ship-an-issue-command.md).

```sh
issue https://github.com/acme/widgets/issues/42      # the thread: the issue and its comments
issue 42                                             # the same, in the origin remote's repository
issue acme/widgets!7 --checks --reviews --files      # a pull request with its CI, reviews and files
issue acme/widgets                                   # the repository
issue --list --label bug --limit 10                  # open issues labelled bug
issue --create --title "Crash on start" --body-file report.md
issue 42 --comment --body-file - < reply.md
issue 42 --edit --add-label triaged --remove-label new
issue 42 --close --body "Fixed in #43" --dry-run     # what it would write
issue --create-pr --head fix-crash --base main --title "Fix the crash"
issue 7 --merge --method squash
issue --schema                                       # the interface, as JSON Schema
```

Install it with `make install` (into `$GOBIN`, stamped with `git describe`) or
`go install github.com/agent-fox-dev/aflib/cmd/issue@latest`.

Two paths are for a person and print text: `--version` prints the build
identity (`issue`, the version, then the commit and build time when the build
set them), and `-h`/`--help` or a bare invocation print the help to stderr.
`--help` exits 0 and writes nothing to stdout. A bare invocation (no input and
no form flag) exits 2, and when stdout is not a terminal it also writes a usage
envelope, so a program still gets one object.

## The input

The one positional input names a thread (an issue, a pull request or a merge
request) or a repository:

| Input | Names |
|---|---|
| `https://github.com/o/r/issues/42`, `…/pull/7` | a GitHub issue or pull request, on the URL's host |
| `https://gitlab.com/g/s/p/-/issues/3`, `…/-/merge_requests/5` | a GitLab issue or merge request |
| `o/r#42` | issue 42; on GitHub also pull request 42 |
| `o/r!7`, `g/s/p!7` | pull or merge request 7 |
| `42`, `#42`, `!7` | the same, in the repository of the origin remote of `--dir` |
| `o/r`, `g/s/p` | a repository; a GitLab owner may hold subgroups |
| `https://github.com/o/r`, `git@gitlab.com:g/p.git` | a repository, on its host |
| none | the origin remote's repository, for `--list`, `--create`, `--create-label` and `--create-pr` |

GitHub numbers issues and pull requests together, so `o/r#7` may be a pull
request: the read says so (`ref` comes back as `o/r!7`). GitLab numbers them
apart, so `#N` is always an issue there and a form that needs a merge request
refuses it (`on GitLab g/s/p#5 is an issue: a merge request is g/s/p!5`).

A URL or remote names its host. A host is classified the way `issuex` does it
([configuration.md](configuration.md)): `github` or `gitlab` in its name, or
the host of `GITHUB_API_URL` or `GITLAB_API_URL`. A host neither names is
refused, and nothing is sent to it. An input without a host (`o/r#42`) goes to
the forge the environment configures, else to the origin remote's.

`-`, an input with spaces, two inputs, and a reference that names nothing are
usage errors.

## The forms

At most one form flag per call. A flag that belongs to another form is a usage
error that names it (`--state does not apply to the read`).

| Form | Input | Flags | `issuex` calls | `result` |
|---|---|---|---|---|
| read | a thread | `--checks`, `--reviews`, `--files` (a pull request only) | `ReadIssue`; for a pull request `ReadPullRequest`, and `GetCIChecks`, `GetPRReviews`, `ReadChangedFiles` | `ref`, `issue`, `comments`, `comments_truncated`, `pull_request`, `checks`, `reviews`, `files` |
| read | a repository | | `GetRepository` | `repository` |
| `--list` | a repository | `--state`, `--label`, `--assignee`, `--sort`, `--direction`, `--limit` | `ListIssues` | `issues`, `incomplete` |
| `--create` | a repository | `--title` (required), `--body`/`--body-file`, `--label`, `--assignee` | `CreateIssue` | `ref`, `issue` |
| `--comment` | a thread | `--body`/`--body-file` (required) | `AddComment` | `ref`, `issue`, `comment_url` |
| `--edit` | a thread | `--title`, `--body`/`--body-file`, `--assignee`, `--add-label`, `--remove-label` (one at least) | `UpdateIssue` (title, body, assignees), `AddLabels`, `RemoveLabel` for each | `ref`, `issue` read back |
| `--close` | a thread | `--body`/`--body-file` | `AddComment` when a body is given, then `CloseIssue` or, for a pull request, `ClosePullRequest` | `ref`, `issue` read back |
| `--reopen` | a thread | | `UpdateIssue` with state `open` | `ref`, `issue` read back |
| `--create-label NAME` | a repository | `--color`, `--description` | `CreateLabel` | `label` |
| `--create-pr` | a repository | `--head`, `--base`, `--title` (required), `--body`/`--body-file`, `--draft` | `CreatePullRequest` | `ref`, `pull_request` |
| `--review` | a pull request | `--body`/`--body-file` (required) | `PostReviewComment` | `ref`, `issue` |
| `--merge` | a pull request | `--method`, `--sha`, `--commit-title`, `--commit-message` | `MergePullRequest` | `ref`, `issue`, `merge` |

Every form's `result` also has `forge` (`github` or `gitlab`), `repo`
(`owner/name`) and `host` when the input or the origin remote named one.

**The read** writes `comments` as `[]` when the thread has none. `issuex` reads
at most 500 comments; a longer thread has `comments_truncated` and a low
`comments_truncated` warning. When the issue is read and its comments are not,
the result has the issue, no `comments`, and a high `comments_unreadable`
warning. `--checks`, `--reviews` and `--files` on a thread that turns out to be
an issue are a usage error.

**The list** leaves pull requests out (GitHub's issue list has them). `--label`
narrows to issues with every label given; `--assignee` takes one login.
`--sort` is `created` or `updated`, `--direction` `asc` or `desc`. When more
issues match than `--limit` (default 30), `incomplete` is true and `next` is
the same call with the limit doubled. `issues` is `[]` when none matches.

**The write forms** need a token, and a call without one is refused before
anything is read (category `auth`). The thread forms read the thread first, to
know whether it is a pull request and in which state it is.

- **`--close` and `--reopen`** of a thread already in that state change nothing:
  `no_change` is true, the summary says `is already closed; nothing written`,
  and the call succeeds. A merged pull request is not open, so `--close` leaves
  it alone.
- **`--edit`** sends title, body and assignees in one `UpdateIssue`, then adds
  the labels, then removes each label. `--assignee` replaces the assignees.
  `issuex` sends no empty field, so a body cannot be emptied and the last label
  cannot be removed by `UpdateIssue`; `--remove-label` removes labels one by
  one, and one the thread does not have is not an error.
- **`--create-label`** of a label that exists already succeeds (`issuex` treats
  it so). `--color` is six hex digits, with or without `#`; GitHub picks a
  colour when none is given, and for GitLab, which requires one, it is `ededed`.
- **`--merge`** without `--method` uses the forge's default: on GitHub, the
  first method the repository allows (merge, squash, rebase). A merge the forge
  refuses, or answers without merging, is a `conflict`.
- **`--assignee` on GitLab** is not sent: the GitLab client of `issuex` does not
  send assignees. The call goes ahead without them and raises a high
  `assignees_ignored` warning.

**A body** is `--body TEXT` or `--body-file PATH`, where `PATH` `-` reads stdin.
An empty body is a usage error. A body longer than the forge takes
(`issuex.MaxBodyLength`: 65,536 characters on GitHub, 1,048,576 on GitLab) is
refused before anything is sent.

**`--dry-run`**, on a write form, makes the reads (the thread, or for a
repository form the repository, must exist; `--close` and `--reopen` read its
state) and refuses as the real call would, but writes nothing. `dry_run` is
true and `planned` lists the writes the call would make, in order, each with
its `action`, `kind`, `target` and a `detail` in words. `next` is the same call
without `--dry-run`. A dry run of a call that would change nothing says so with
`no_change`, as the real one does.

## The output

A read of an issue:

```jsonc
{
  "tool": "issue",
  "version": "v0.1.0 (commit 94f3890, built 2026-10-23T08:00:00Z)",
  "ok": true,
  "status": "done",
  "exit_code": 0,
  "summary": "issue: acme/widgets#42 open: \"Crash on start\" (1 comment)",
  "result": {
    "forge": "github",
    "repo": "acme/widgets",
    "ref": "acme/widgets#42",
    "issue": { "number": 42, "title": "Crash on start", "body": "…", "state": "open",
               "html_url": "https://github.com/acme/widgets/issues/42", "author": { "login": "alice" },
               "labels": ["bug"], "created_at": "…", "updated_at": "…", "is_pr": false },
    "comments": [ { "id": 9, "body": "Seen on 1.4 too.", "user": { "login": "bob" }, "created_at": "…" } ]
  },
  "duration_ms": 412,
  "started_at": "2026-10-23T09:00:00Z"
}
```

A close with a comment; every write is in `side_effects`:

```jsonc
{
  "ok": true,
  "summary": "issue: closed acme/widgets#42, with a comment",
  "result": { "forge": "github", "repo": "acme/widgets", "ref": "acme/widgets#42",
              "issue": { "number": 42, "state": "closed", … },
              "comment_url": "https://github.com/acme/widgets/issues/42#issuecomment-1" },
  "side_effects": [
    { "action": "comment", "kind": "issue", "target": "acme/widgets#42", "url": "https://github.com/acme/widgets/issues/42#issuecomment-1", "ok": true },
    { "action": "close", "kind": "issue", "target": "acme/widgets#42", "ok": true }
  ]
}
```

The same call under `--dry-run`:

```jsonc
{
  "ok": true,
  "summary": "issue: would close acme/widgets#42 (comment a body of 12 characters; close)",
  "result": { "ref": "acme/widgets#42", "issue": { … }, "dry_run": true,
              "planned": [ { "action": "comment", "kind": "issue", "target": "acme/widgets#42", "detail": "a body of 12 characters" },
                           { "action": "close", "kind": "issue", "target": "acme/widgets#42" } ] },
  "next": [ { "tool": "issue", "input": "42", "flags": ["--close", "--body", "Fixed in #43"],
              "why": "the same call without --dry-run, which makes these writes" } ]
}
```

A merge the forge refuses; the write that failed is in `side_effects`:

```jsonc
{
  "ok": false,
  "status": "failed",
  "exit_code": 1,
  "summary": "merge acme/widgets!7: conflict: PUT /repos/acme/widgets/pulls/7/merge: 405 Pull Request is not mergeable",
  "error": { "stage": "write", "category": "conflict", "retryable": false, "resumable": false,
             "message": "merge acme/widgets!7: conflict: PUT /repos/acme/widgets/pulls/7/merge: 405 Pull Request is not mergeable" },
  "result": { "ref": "acme/widgets!7", "issue": { … } },
  "side_effects": [ { "action": "merge", "kind": "pull_request", "target": "acme/widgets!7", "ok": false } ]
}
```

| Field | Meaning |
|---|---|
| `tool`, `version` | `issue` and its build identity |
| `ok` | true only when the call did all it was asked; never beside a non-zero `exit_code` or an `error` |
| `status`, `exit_code` | `done` 0, `failed` 1, `usage` 2 |
| `summary` | one sentence, at most 200 characters; on a success that raised a high warning it ends `; 1 high-severity warning` |
| `error` | `stage`, `category`, `message`, `retryable`, `resumable` (always false: no form resumes), `fix_hint`; present exactly when `ok` is false |
| `warnings` | `code`, `severity`, `stage`, `message` |
| `result` | the form's result, above. Absent on a usage error. A failure after the thread was read keeps what was read |
| `side_effects` | every write to the forge, in order: `action` (`create_issue`, `comment`, `edit`, `add_labels`, `remove_label`, `close`, `reopen`, `create_label`, `create_pull_request`, `review`, `merge`), `kind` (`issue`, `pull_request`, `repository`), `target`, `url` when the forge returned one, `ok` |
| `next` | the calls a caller would plausibly make next; none after a failure |
| `duration_ms`, `started_at` | the wall-clock time of the call, and when it started |

### `summary`

| Form | Shape |
|---|---|
| read of an issue | `issue: <ref> <state>: "<title>" (<n> comments)` |
| read of a pull request | `issue: <ref> <open, draft, closed or merged> pull request <head> → <base>: "<title>" (<n> comments)`, then `; checks: 2 success, 1 failure`, `; <n> reviews`, `; <n> files changed` for what was asked |
| read of a repository | `issue: <repo> on <forge>, default branch <branch>` |
| `--list` | `issue: <n> <state> issues in <repo>`, then `; more match than --limit <n>` |
| a write | `issue: opened <ref>: "<title>"`, `commented on <ref>`, `edited <ref> (<writes>)`, `closed <ref>` (`, with a comment`), `reopened <ref>`, `label "<name>" in <repo>`, `opened <ref>, <head> → <base>: "<title>"`, `reviewed <ref>`, `merged <ref> as <sha>` |
| a write under `--dry-run` | `issue: would <verb> <target> (<writes>)` |
| nothing to change | `issue: <ref> is already <state>; nothing written` |
| a failure | the error's message |

Titles longer than 60 characters are cut with `…`.

### `next`

- after a dry run: the same call without `--dry-run` (when the body came from
  stdin, `why` says to give it again);
- after `--list` with `incomplete`: the same call with `--limit` doubled;
- after `--create-pr`: the read of the new pull request with `--checks
  --reviews`;
- otherwise, and after any failure: nothing.

### `error`

| `stage` | `category` | When |
|---|---|---|
| `usage` | `usage` | two forms; a flag of another form; a value outside its set (`--forge`, `--state`, `--sort`, `--direction`, `--method`; `fix_hint.valid` lists the set); a required flag missing; an empty flag; `--body` with `--body-file`; a body file that cannot be read or is empty; a body over the forge's limit; an input of the wrong kind for the form, or that names nothing; no input and no origin remote (`fix_hint.flag` `--dir`); `--forge` that disagrees with the input's host; on GitLab, `#N` for a merge request form; after the read, a pull request form on an issue. Nothing was written, and the message is also written to stderr |
| `forge` | `input` | the forge cannot be told: a host neither variable names, or both forges configured and no origin remote (`fix_hint`: `--forge`, and the variables `GITHUB_API_URL`, `GITLAB_API_URL`) |
| `read`, `write` | `auth` | no token for a write form (`fix_hint.env` names the variables), or the forge answered 401 or 403 |
| `read`, `write` | `input` | the forge does not have the thread or repository (404), or refused the request (400, 422) |
| `write` | `conflict` | the forge refused the write for the state it is in: a merge that cannot be made (405, 409), or a merge answered without merging |
| `read`, `write` | `api` | a rate limit `issuex` did not wait out, a 5xx, a network failure. **Retryable** |
| `read`, `write` | `aborted` | the call outlived `--timeout` (`fix_hint.flag` `--timeout`). **Retryable** |
| `schema` | `internal` | the `--schema` document could not be encoded |

### Exit codes

| Code | `status` | Meaning |
|---|---|---|
| `0` | `done` | read; or written; or nothing needed to change |
| `1` | `failed` | the forge, a read or a write failed; `side_effects` lists the writes made before it |
| `2` | `usage` | usage error; nothing was written |

### Warning codes

| Code | Severity | Stage | When |
|---|---|---|---|
| `comments_unreadable` | high | read | the thread was read and its comments were not |
| `comments_truncated` | low | read | the thread has more comments than the 500 `issuex` reads |
| `assignees_ignored` | high | write | `--assignee` with `--create` or `--edit` on GitLab, whose client does not send assignees |

### Reading it from Go

`envelope.DecodeIssue` reads the object, whichever form it was, into an
`envelope.IssueEnvelope`, whose `Result` is an `*envelope.IssueResult`; the
fields that were absent are zero. See [api.md](api.md#envelope).

```go
env, err := envelope.DecodeIssue(stdout)
if err != nil {
    return err
}
if !env.OK {
    return env.Error // an *envelope.ErrorInfo
}
for _, c := range env.Result.Comments {
    fmt.Println(c.User.Login, c.Body)
}
```

## Flags

| Flag | Default | Effect |
|---|---|---|
| `--list` | off | the list form |
| `--create` | off | the create form |
| `--comment` | off | the comment form |
| `--edit` | off | the edit form |
| `--close` | off | the close form |
| `--reopen` | off | the reopen form |
| `--create-label` | | the create-label form: the label's name |
| `--create-pr` | off | the create-pr form |
| `--review` | off | the review form |
| `--merge` | off | the merge form |
| `--checks`, `--reviews`, `--files` | off | with the read of a pull request, also its CI checks, reviews, changed files |
| `--title` | | the title of the issue or pull request to open; the new title with `--edit` |
| `--body` | | the body; the comment `--close` posts first |
| `--body-file` | | read the body from a file; `-` is stdin |
| `--label` | | a label of the issue to open, or a `--list` filter; repeated or comma-separated |
| `--assignee` | | an assignee of the issue to open, the new assignees with `--edit`, or the one `--list` filter; repeated or comma-separated |
| `--add-label`, `--remove-label` | | with `--edit`, labels to add and to remove; repeated or comma-separated |
| `--state` | `open` | with `--list`: `open`, `closed` or `all` |
| `--sort` | `created` | with `--list`: `created` or `updated` |
| `--direction` | `desc` | with `--list`: `asc` or `desc` |
| `--limit` | `30` | with `--list`, the most issues to list |
| `--head`, `--base` | | with `--create-pr`, the branches |
| `--draft` | off | with `--create-pr`, open a draft |
| `--method` | the forge's | with `--merge`: `merge`, `squash` or `rebase` |
| `--sha` | | with `--merge`, the head commit the pull request must be at |
| `--commit-title`, `--commit-message` | | with `--merge`, of the merge commit |
| `--color`, `--description` | | with `--create-label` |
| `--dir` | `.` | the working tree whose origin remote names the repository when the input does not |
| `--forge` | | `github` or `gitlab`, for a repository with no host when the environment configures both forges and there is no origin remote. It must agree with a host the input names |
| `--timeout` | `2m` | the longest the whole call may take |
| `--dry-run` | off | with a write form: make the reads, none of the writes |
| `--schema` | | print the self-description and exit 0 |
| `--version` | | print the build identity and exit 0 |

Flags may come before or after the input. There is no `--token`: a token is
read from the environment only ([configuration.md](configuration.md)).

## Self-description (`--schema`)

`issue --schema` prints one JSON object and exits 0. It reads nothing (no
directory, no environment, no forge), so it runs anywhere:

- `tool`, `description`;
- `input`: what the input may be, with examples;
- `flags`: a JSON Schema (2020-12) document of the flags, each with its `type`,
  `default` and `description`; the flags with a closed set have an `enum`, and
  the repeatable ones are arrays. `--schema` and `--version` are left out;
- `result`: a JSON Schema document of the envelope, generated from
  `envelope.IssueEnvelope`;
- `exit_codes`.

The document is checked in at `cmd/issue/testdata/schema.golden.json`, and a
test fails when a flag or a result field changes without it;
`UPDATE_GOLDEN=1 go test -run TestSchemaGolden ./cmd/issue` rewrites it.

## What it leaves out

`issuex`'s limits are the command's: no body can be emptied and no last label
removed through `UpdateIssue`, GitLab assignees are not sent, at most 500
comments are read. It has no milestones, projects, line comments on a diff,
forks or releases, and no forge besides GitHub and GitLab. It takes no lock: two
callers that close one thread at once both see it open. It never prompts, opens
an editor or pages.
