# Configuration

aflib has no configuration file and no flags. Only `issuex` reads the
environment; `jobqueue` is configured by the options passed to `jobqueue.New`
and reads **no environment** variable.

## Environment read by issuex

`issuex` reads five variables. The constructors and forge detection read one only
for an `Options` field left empty (see *Precedence*); `ParseRemote` also reads
the two API URL variables, to recognise the host of a self-hosted forge.

| Variable | Meaning | Read in |
|---|---|---|
| `GITHUB_API_URL` | API base URL of GitHub or of a GitHub Enterprise server. Default `https://api.github.com`. | `NewGitHub` (`issuex/github.go`), forge detection (`issuex/detect.go`), `ParseRemote` (`issuex/parse.go`) |
| `GITHUB_TOKEN` | Token for GitHub. | `NewGitHub`, `envToken` (`issuex/detect.go`) |
| `GH_TOKEN` | Token for GitHub, read only when `GITHUB_TOKEN` is empty. It is the variable the `gh` CLI uses; nothing here runs `gh`. | `NewGitHub`, `envToken` |
| `GITLAB_API_URL` | API base URL of GitLab or of a self-hosted GitLab. Default `https://gitlab.com/api/v4`; `/api/v4` is appended when it is missing. | `NewGitLab` (`issuex/gitlab.go`), forge detection, `ParseRemote` |
| `GITLAB_TOKEN` | Token for GitLab. | `NewGitLab`, `envToken` |

## Precedence

An explicit `Options.BaseURL` and `Options.Token` win over the environment: the
variables are read only for a field left empty. `NewGitHub` and `NewGitLab` apply
this to the URL and the token, and forge detection leaves an explicit `BaseURL`
alone.

When `issuex` derives the API address from a host (a remote URL, a repository
or the origin remote), the environment's token is sent only to the host it is
configured for: to the host named by `GITHUB_API_URL` or `GITLAB_API_URL`, or to
the public forge when no API URL is configured. Any other host gets its own API
address and no token (`hostEndpoint` in `issuex/detect.go`). An explicit
`Options.BaseURL` is the caller's choice and is not checked: `NewGitHub` and
`NewGitLab` still read the token variable when `Options.Token` is empty and send
it to that URL, so a caller that sets `BaseURL` and wants no environment token
sets `Options.Token`.

## How the forge is decided

`NewWithOptions` and `DetectForge` decide, in this order (`detectForge` in
`issuex/detect.go`):

1. `Options.BaseURL`: a URL that contains `github` or `gitlab` (not both) names
   the forge; otherwise `Options.RemoteURL` or `Options.Repo.Host` is used.
2. `Options.RemoteURL`.
3. `Options.Repo.Host`. A host is classified by `GITHUB_API_URL` or
   `GITLAB_API_URL` when one names it, then by `github` or `gitlab` in the host
   name.
4. The environment: only GitHub variables set means GitHub, only GitLab
   variables set means GitLab. With both or neither set, the next step decides.
5. The origin remote of `Options.Dir`, or of the working directory when `Dir` is
   empty.

A host that names neither forge, and is named by neither API URL variable, is
`ErrAmbiguousForge`, and no request is sent to it. A self-hosted GitLab or GitHub
is named with `GITLAB_API_URL` or `GITHUB_API_URL`.

## Fallbacks

- **`git remote get-url`**. `DetectRepo` (`issuex/parse.go`) runs
  `git -C <dir> remote get-url origin` and parses the output. It is the only
  subprocess `issuex` starts. When `git` is missing, the directory is not a
  repository or there is no `origin`, detection reports no repository, and the
  result is `ErrAmbiguousForge`.
- **No `gh` probe.** There is no `gh` CLI probe: nothing runs `gh`, and
  `GH_TOKEN` is read as a variable only.

## Hub sets everything explicitly

According to the PRD of this import
([`docs/prd/01-import-issuex-and-jobqueue.md`](prd/01-import-issuex-and-jobqueue.md)),
hub's forge API is built on `issuex` and always passes the token and the remote
URL explicitly (`Options.Token`, `Options.RemoteURL`), relying on neither the
environment nor a working directory; `af --local` uses the environment and the
fallback. Hub at the commit this package was copied from does not import `issuex`
yet, so that is the intended use, not something this repository can show. With a
remote URL that names a forge, detection stops at step 2 and never starts `git`.
