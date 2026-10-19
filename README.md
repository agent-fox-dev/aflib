# aflib

Shared Go packages of the agent-fox repositories. A package lives here once more
than one agent-fox program needs it.

| Package | What it is |
|---|---|
| [`issuex`](docs/api.md#issuex) | One `Client` interface over GitHub and GitLab for repositories, issues, labels, comments and pull or merge requests, with forge detection. Standard library only. |
| [`jobqueue`](docs/api.md#jobqueue) | A durable, SQLite-backed job queue with retry policies, per-key and group serialization, progress, crash recovery and graceful shutdown. |

Both were moved here unchanged: `issuex` from agent-fox, `jobqueue` from hub. The
exported API of each is as it was, so a consumer switches by changing an import
path. [`docs/architecture.md`](docs/architecture.md) says where each came from.

## Use it

```
go get github.com/agent-fox-dev/aflib
```

Then import `github.com/agent-fox-dev/aflib/issuex` or
`github.com/agent-fox-dev/aflib/jobqueue`. The module needs Go 1.27. It has no
tags yet; the first, `v0.1.0`, is cut once hub and agent-fox have both switched to
it.

## Develop

```
make check        fmt, vet, lint and the short tests: the gate before committing
make doc          regenerate docs/generated/ (go doc -all of each package)
make api-golden   accept a change of an exported declaration
make help         list all targets
```

`make check` fails when an exported declaration of a public package changes, until
`make api-golden` rewrites its golden file in `testdata/api/` and the result is
committed; it also fails on a `replace` directive or an import outside the
allowlist. `golangci-lint` runs when it is installed.

## Documentation

- [`docs/api.md`](docs/api.md): the map of the public packages and their principal
  identifiers; the full Go docs are in [`docs/generated/`](docs/generated/)
- [`docs/architecture.md`](docs/architecture.md): what each package is, what
  imports what, the dependency allowlist, the tests, the `jobqueue` caller
  contract, versioning
- [`docs/configuration.md`](docs/configuration.md): the environment variables
  `issuex` reads
- [`docs/cli.md`](docs/cli.md): there is no command
- [`docs/errata/`](docs/errata/): where the code differs from its specs
- [`docs/prd/`](docs/prd/): the product requirements; specs are in `.specs/`
