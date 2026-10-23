# 01. Ship a command for the forge client

**Status:** accepted
**Date:** 2026-10-23
**Related:** [PRD 02](../prd/02-ship-an-issue-command.md),
[PRD 01](../prd/01-import-issuex-and-jobqueue.md) (spec 01, 01-REQ-1.2)

## Context

aflib was created as a library with no command: spec 01 pinned the module to
the packages `aflib`, `issuex` and `jobqueue`, and `TestPackageLayout_TS_01_2`
failed on a `cmd/` directory. Its purpose was to hold the packages two
agent-fox programs share, and nothing speculative.

`issuex` is a complete forge client, and nothing reaches it from a shell. An
agent that works on an issue (reads it, comments, labels it, opens the pull
request, watches its checks, merges it) drives `gh` on GitHub and `glab` on
GitLab, with two interfaces and two output formats, or has a program written
for it. The agent-fox commands in the spec repository (`spec`, `specr`,
`speca`) have settled what a command an agent drives looks like: one input,
forms chosen by flags, one JSON envelope on stdout, three exit codes,
`--schema`.

The command could live in its own repository, in agent-fox, or here.

## Decision

The command lives here, as `cmd/issue`, and the envelope it writes is a public
package, `envelope`.

- **Here, beside `issuex`.** The command is `issuex` with a shell around it:
  every form is one or a few `Client` calls. In the same module, a change to
  the client and the command that exposes it are one change, tested together,
  and the command needs no dependency (`issuex` is standard library only).
  Another repository would pin aflib by version and trail it.
- **The interface is the agent-fox one.** Exactly one positional input (none
  for a form on the origin remote's repository), forms as flags, a bare call
  that only reads, one JSON object on stdout on every path a program reads,
  the help on stderr, exit codes 0, 1 and 2, `--schema` pinned by a golden
  file, `--version`, and `next`. Every write to a forge is in `side_effects`,
  a failed one included, and `--dry-run` makes the reads and none of the
  writes.
- **The envelope is a public package.** A Go program that runs `issue`
  decodes its output with `envelope.DecodeIssue` instead of declaring its own
  structs. The JSON is the spec repository's envelope without the properties
  only a model-driven command writes, so either decoder reads the other's
  output; a later change may move the spec repository's commands onto this
  package.
- **No credential on the command line.** The command reads the environment
  `issuex` reads and adds nothing to it.
- **Commands are not public packages.** The exported-API goldens and the
  package list of the layout test cover importable packages; `package main`
  is skipped by `discover`. The layout test now pins the four public packages
  and the one command.

## Consequences

- 01-REQ-1.2 ("there is no `cmd/`") is amended by PRD 02; the erratum of spec
  01 records it.
- `docs/cli.md` is a command reference, no longer one line, and the steering
  file's repository specifics say aflib has a command.
- `make install` builds the command stamped with `git describe`; `make build`
  builds it with the packages.
- `issuex`'s limits are the command's limits, and are documented as such: an
  `UpdateIssue` that cannot clear a body or remove every label, and a GitLab
  adapter that does not send assignees. Lifting them is a change to `issuex`.
