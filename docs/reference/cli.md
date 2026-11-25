# CLI

```
hobnob [--file <path> | --demo] [<task>] [KEY=VALUE ...] [--no-input] [--quiet | -q]
hobnob [--file <path> | --demo] (--list | --select | --help)
hobnob (--version | --upgrade | completion <shell>)
```

## `<task>`

The task to run, always the first argument. Without one, hobnob runs the task
named `default`, or opens the picker if there is none.

```bash
hobnob ask-trivia
```

A `_`-prefixed task is internal: only `call:` can reach it.

## `KEY=VALUE`

Sets a variable for the run, above `vars:` and env files, below `const:` (see
[Precedence](../guide/variables.md#which-value-wins)). Repeatable. It also answers a `get:` for the same
name, which is what makes an interactive task scriptable. A value that's a
valid JSON list or object is data, anything else text; see
[Text that looks like JSON](../guide/structured-data.md#text-that-looks-like-json).

```bash
hobnob ask-trivia CATEGORY=science COUNT=3
```

## `--no-input`

Skips every prompt: a `get:` uses its `default:`, or aborts the run without
one. Implied when `CI` is set or stdin is not a terminal, so scripts and AI
agents get it for free.

```bash
hobnob ask-trivia --no-input
```

## `--quiet`, `-q`

Quiet run, for agents and logs: it prints only each `run:` command's `$` line
and every task's end line, called tasks included. Variables, calls, skips,
`✓` step lines, `#` notes and command output are left out, except that a step
that fails or soft-fails prints its output before its `✗` or `!` line, and a
prompt a command prints with no newline shows as it arrives. Failure
ends, the soft-failure count and the ctrl+c lines print as in a normal run.
`HOBNOB_QUIET=1` does the same. Nothing else turns it on: not a pipe, not CI,
not a detected agent.

```bash
hobnob deploy -q
HOBNOB_QUIET=1 hobnob deploy
```

```
$ hobnob deploy -q
setup $ npm ci
setup ✓ done after 1.1s
deploy $ go build ./...
deploy ✓ done after 2.1s
```

## `--file <path>`

Uses a specific taskfile.

```bash
hobnob --file ops/trivia.yml ask-trivia
```

Without it, hobnob searches upward from the current directory for
`hobnob.yml`, then `hobnob.yaml`. Relative paths inside a file resolve against
that file's directory, not where you ran from.

## `--demo`

Runs a small built-in taskfile. It replaces `--file` (passing both is an
error) and combines with every other flag. A grey `hobnob »` line on stderr
says so.

```bash
hobnob --demo ask-trivia
hobnob --demo --list
```

## `--list`

Prints every public task with its `info:`, grouped by the part of the name
before the first `:`; tasks without one come last. Internal tasks and modules
are omitted.

```
$ hobnob --list
trivia
  trivia:random  Fetch a random question.

standup
```

## `--select`

Opens the picker even when a `default` task exists. With no terminal, under
CI, or with `--no-input`, it prints the task list instead.

## `--help`

Prints usage, the flags, the docs links, and the grouped task list under
Tasks.

```
$ hobnob --help
hobnob v1.5.0

Usage
  hobnob [--file <path>] <task> [--no-input] [KEY=VALUE ...]
  hobnob [--file <path>] --list | --select | --help
  hobnob --demo | --version | --upgrade

Flags
  --file <path>   Hobnob file to use instead of auto-discovery
  --demo          Run the built-in demo taskfile instead of one of yours
  --list          List all available tasks
  --select        Interactively select a task to run
  --help          Show this help
  --no-input      Skip interactive prompts; fail if a required variable is missing
  --quiet, -q     Print only each command and each task's end (or set HOBNOB_QUIET=1)
  --version       Print version and exit
  --upgrade       Upgrade hobnob to the latest release

Docs
  Guide      https://jakesmd.github.io/hobnob/docs/guide/
  Reference  https://jakesmd.github.io/hobnob/docs/reference/

Tasks
  trivia
    trivia:random  Fetch a random question.

  standup
```

## `--version`

Prints the version. Works outside any taskfile.

## `--upgrade`

Reruns the latest installer; see [Upgrading](../installation.md#upgrading).
It writes in the log's style: `hobnob » downloading the latest installer…`, the
installer's output under `hobnob │`, then `hobnob ✓ upgraded dev → v1.5.0`.

## `completion <shell>`

Prints the completion script for `bash`, `zsh` or `fish`. The installer wires
it up; run it yourself only to regenerate.

```bash
hobnob completion zsh > ~/.zsh/completions/_hobnob
```

## Stopping a run

CTRL+C signals the running command and any
[background jobs](../guide/commands.md#background-jobs), then waits for the
command to exit; no further steps start. The log says `! stopping, ctrl+c again
to force` on the running task, and the step and each task in the call chain end
with `stopped` where a failure says `failed`. Jobs still running 2 seconds after
that are killed. A second CTRL+C kills the command and the jobs immediately.
Closing the terminal (`SIGHUP`) or sending `SIGTERM` stops a run like CTRL+C.
