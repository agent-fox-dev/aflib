// Command issue reads and writes the issues, pull requests and repositories of
// GitHub and GitLab through the issuex client, for an agent or a person with a
// shell.
//
//	issue [flags] <ref>                      an issue or pull request: its thread
//	issue [flags] <repo>                     a repository: its metadata
//	issue [<repo>] --list [filters]          the issues of a repository
//	issue [<repo>] --create --title T        open an issue
//	issue <ref> --comment --body B           comment on a thread
//	issue <ref> --edit [changes]             change title, body, labels, assignees
//	issue <ref> --close | --reopen           close or reopen a thread
//	issue [<repo>] --create-label NAME       create a label
//	issue [<repo>] --create-pr --head H --base B --title T
//	issue <pr> --review --body B             post a review comment
//	issue <pr> --merge [--method M]          merge a pull request
//
// It takes at most one input, a reference to a thread or a repository, and
// writes exactly one JSON object to stdout, the envelope of package
// github.com/agent-fox-dev/aflib/envelope, whose result is an
// envelope.IssueResult. The forms are flags, not subcommands; without a form
// flag the call reads and changes nothing. Every write to the forge is listed
// in the envelope's side_effects. The token comes from the environment
// (GITHUB_TOKEN or GH_TOKEN, GITLAB_TOKEN), never from a flag.
package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/agent-fox-dev/aflib/envelope"
)

const tool = "issue"

// description is the sentence --schema gives.
const description = "Reads and writes the issues, pull requests and repositories of GitHub and GitLab " +
	"through one interface: a thread with its comments (and, for a pull request, its branches, CI " +
	"checks, reviews and changed files), a repository's metadata, the issues of a repository; and " +
	"creates issues, labels and pull requests, comments, edits, closes, reopens, reviews and merges. " +
	"Without a form flag it reads and changes nothing. Every write is listed in side_effects; " +
	"--dry-run makes the reads and none of the writes. The token comes from the environment."

// inputDescription is what --schema says the one positional input may be.
const inputDescription = "At most one reference: an issue or pull request (a web URL, owner/repo#N, " +
	"owner/repo!N for a pull or merge request, or N, #N, !N in the repository of the origin remote " +
	"of --dir), or a repository (owner/repo, group/subgroup/project, a repository or git remote " +
	"URL). The forms that take a repository use the origin remote's when none is given."

const usage = `issue — read and write the issues and pull requests of GitHub and GitLab

Usage:
  issue [flags] <ref>                     an issue or pull request: its thread
  issue [flags] <repo>                    a repository: its metadata
  issue [<repo>] --list [filters]         the issues of a repository
  issue [<repo>] --create --title T [--body B] [--label L] [--assignee A]
  issue <ref> --comment --body B
  issue <ref> --edit [--title T] [--body B] [--add-label L] [--remove-label L] [--assignee A]
  issue <ref> --close [--body B]
  issue <ref> --reopen
  issue [<repo>] --create-label NAME [--color C] [--description D]
  issue [<repo>] --create-pr --head H --base B --title T [--body B] [--draft]
  issue <pr> --review --body B
  issue <pr> --merge [--method merge|squash|rebase] [--sha S]

The reference names a thread or a repository:
  https://github.com/o/r/issues/42, …/pull/7, https://gitlab.com/g/p/-/merge_requests/5
  o/r#42        issue 42 (on GitHub, also pull request 42)
  o/r!7         pull or merge request 7
  42, #42, !7   the same, in the repository of the origin remote of --dir
  o/r, g/s/p    a repository; also a repository URL or a git remote URL
The forms that take a repository use the origin remote's when none is given.

Forms (one per call; none is the read):
  (none)          read the thread: the issue and its comments, and for a pull
                  request its branches and state; --checks, --reviews and
                  --files add its CI checks, reviews and changed files. Given
                  a repository, read the repository
  --list          list the issues of a repository (pull requests left out),
                  filtered by --state, --label, --assignee, ordered by --sort
                  and --direction, at most --limit
  --create        open an issue with --title, --body, --label, --assignee
  --comment       comment on a thread; the body is --body or --body-file
  --edit          change the title, the body, the assignees, add labels and
                  remove labels; a body cannot be emptied
  --close         close a thread, after a comment when a body is given
  --reopen        reopen a thread
  --create-label  create a label; one that exists already is not an error
  --create-pr     open a pull or merge request from --head into --base
  --review        post a review comment on a pull request
  --merge         merge a pull request
Closing a closed thread and reopening an open one change nothing.

A body is --body TEXT or --body-file PATH, where - is stdin. --dry-run makes
the reads a write form makes (the thread or repository must exist) and none of
its writes, and lists them in result.planned.

Output is one JSON object on stdout. Every write is listed in side_effects, a
failed one included; next names the call to make after this one. --schema
prints the flags and the shape of the output as JSON Schema.

The token is read from the environment: GITHUB_TOKEN or GH_TOKEN for GitHub,
GITLAB_TOKEN for GitLab. GITHUB_API_URL and GITLAB_API_URL name a self-hosted
forge. A read of a public repository needs no token.

Exit codes:
  0  done — read, or written, or nothing needed to change
  1  failed: no token or one the forge refused (auth), a thread or repository
     the forge does not have (input), a merge it refused (conflict), a rate
     limit or a server or network failure (api, retryable), --timeout
     (aborted, retryable); the stage is named in error
  2  usage error — nothing was written

Examples:
  issue https://github.com/acme/widgets/issues/42
  issue 42
  issue acme/widgets!7 --checks --reviews
  issue --list --label bug --limit 10
  issue --create --title "Crash on start" --body-file report.md
  issue 42 --comment --body-file - < reply.md
  issue 42 --edit --add-label triaged --remove-label new
  issue 42 --close --body "Fixed in #43" --dry-run
  issue --create-pr --head fix-crash --base main --title "Fix the crash"
  issue 7 --merge --method squash

Flags:
`

// Main parses argv, runs the call and writes one envelope to stdout. It
// returns the exit code.
//
// Two paths are for a person and print text instead: --version, and
// -h/--help or a bare invocation, which print the help to stderr. A bare
// invocation also writes a usage envelope when stdout is not a terminal, so
// a program piping stdout still gets one object to parse.
func Main(argv []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var o options
	fs := flag.NewFlagSet(tool, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	o.register(fs)
	run := newRun()

	var parseOut bytes.Buffer
	fs.SetOutput(&parseOut)
	ref, flagArgs, err := splitArgs(fs, argv)
	fs.SetOutput(stderr)
	if err != nil {
		_, _ = io.Copy(stderr, &parseOut)
		if errors.Is(err, flag.ErrHelp) {
			return envelope.ExitOK
		}
		fmt.Fprintf(stderr, "%s: %v\n", tool, err)
		return run.emit(stdout, nil, usageInfo(err, nil))
	}
	fs.Visit(func(f *flag.Flag) { o.given[f.Name] = true })
	if o.version {
		fmt.Fprintf(stdout, "%s %s\n", tool, identity())
		return envelope.ExitOK
	}
	if o.schema {
		b, err := envelope.Encode(describe(fs))
		if err != nil {
			return run.emit(stdout, nil, &envelope.ErrorInfo{Stage: "schema", Category: envelope.CategoryInternal, Message: err.Error()})
		}
		_, _ = stdout.Write(b)
		return envelope.ExitOK
	}
	ref = strings.TrimSpace(ref)
	if ref == "" && o.form() == formRead {
		fs.Usage()
		if isTerminal(stdout) {
			return envelope.ExitUsage
		}
		return run.emit(stdout, nil, usageInfo(errors.New(noInputMessage), nil))
	}
	if refusal := o.check(ref); refusal != nil {
		fmt.Fprintf(stderr, "%s: %s\n", tool, refusal.Message)
		return run.emit(stdout, nil, refusal)
	}

	c := &call{opts: &o, run: run, input: ref, flagArgs: flagArgs, stdin: stdin}
	failure := c.execute()
	if failure != nil && failure.Category == envelope.CategoryUsage {
		fmt.Fprintf(stderr, "%s: %s\n", tool, failure.Message)
	}
	return run.emit(stdout, c, failure)
}

const noInputMessage = "no input: give an issue or pull request (a URL, owner/repo#N, owner/repo!N, N), " +
	"a repository (owner/repo), or a form that takes the origin remote's repository (--list, --create, " +
	"--create-label, --create-pr)"

func usageInfo(err error, hint *envelope.FixHint) *envelope.ErrorInfo {
	return &envelope.ErrorInfo{Stage: "usage", Category: envelope.CategoryUsage, Message: err.Error(), FixHint: hint}
}

// splitArgs separates the one positional input from the flags, so that flags
// may come before or after it, and parses the flags. It also returns the
// flag tokens as given, for a next call that repeats them.
func splitArgs(fs *flag.FlagSet, argv []string) (positional string, flagArgs []string, err error) {
	var positionals []string
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		switch {
		case a == "--":
			positionals = append(positionals, argv[i+1:]...)
			i = len(argv)
		case a == "-":
			positionals = append(positionals, a)
		case strings.HasPrefix(a, "-"):
			flagArgs = append(flagArgs, a)
			// "--name value" consumes the next token; "--name=value" and a
			// boolean flag do not.
			if !strings.Contains(a, "=") && i+1 < len(argv) && needsValue(fs, a) {
				i++
				flagArgs = append(flagArgs, argv[i])
			}
		default:
			positionals = append(positionals, a)
		}
	}
	if err := fs.Parse(flagArgs); err != nil {
		return "", nil, err
	}
	switch len(positionals) {
	case 0:
		return "", flagArgs, nil
	case 1:
		return positionals[0], flagArgs, nil
	default:
		return "", nil, fmt.Errorf("expected at most one input, got %d: %s — quote a value that has spaces in it",
			len(positionals), strings.Join(positionals, " "))
	}
}

// needsValue reports whether a flag takes a separate value token.
func needsValue(fs *flag.FlagSet, arg string) bool {
	f := fs.Lookup(strings.TrimLeft(arg, "-"))
	if f == nil {
		return false // unknown: let Parse report it
	}
	bf, ok := f.Value.(interface{ IsBoolFlag() bool })
	return !ok || !bf.IsBoolFlag()
}

// isTerminal reports whether w is a character device, a terminal a person
// reads.
func isTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// withoutFlag returns args minus every occurrence of the boolean flag name.
func withoutFlag(args []string, name string) []string {
	return slices.DeleteFunc(slices.Clone(args), func(a string) bool {
		return a == "-"+name || a == "--"+name || strings.HasPrefix(a, "--"+name+"=") || strings.HasPrefix(a, "-"+name+"=")
	})
}

// defaultTimeout bounds a whole call, every request of it.
const defaultTimeout = 2 * time.Minute

func main() {
	os.Exit(Main(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}
