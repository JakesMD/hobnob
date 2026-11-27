---
description: "Every hobnob command-line flag: running tasks, passing variables, --browser, --no-input, --list, --demo and more."
---

# CLI

```
hobnob [--file <path> | --demo] [<task>] [KEY=VALUE ...] [--no-input] [--quiet | -q] [--browser [--port <number>]]
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

Quiet run, for agents and logs: it hides command output, variables, calls,
skips and notes, and prints only each `run:` command and every task's end. A
failing step still shows its output, and a prompt a command prints with no
newline (`read`, `sudo`, `[y/N]`) shows as it arrives. `HOBNOB_QUIET=1` does the
same. Nothing else turns it on: not a pipe, not CI, not a detected agent.

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

## `--browser`

Runs the task behind a local page and opens it in the default browser.
The terminal keeps the usual run log, plus the page's address. Without a task
the page opens a task picker, even when the taskfile has a `default` task.
It combines with `--file`, `--demo`, `KEY=VALUE` and `--quiet`, is an error
with `--no-input`, `--list`, `--select` and `--help`, and is not yet supported
on Windows. See [Browser mode](../guide/browser-mode.md).

```bash
hobnob --browser setup
hobnob --browser
```

## `--port <number>`

Serves the `--browser` page on this port of `127.0.0.1` instead of a random free
one, at an address with no token, and doesn't open the browser or give up on a
closed tab. The page is always the task picker. It is an error without
`--browser`, with a task, with a number outside 1 to 65535, and when the port
is already in use. See [Browser mode](../guide/browser-mode.md).

```bash
hobnob --browser --port 8080
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
error) and combines with every other flag.

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

Prints usage, the flags, the docs links, and the grouped task list.

## `--version`

Prints the version. Works outside any taskfile.

## `--upgrade`

Reruns the latest installer; see [Upgrading](../installation.md#upgrading).

## `completion <shell>`

Prints the completion script for `bash`, `zsh` or `fish`. The installer wires
it up; run it yourself only to regenerate.

```bash
hobnob completion zsh > ~/.zsh/completions/_hobnob
```

## Stopping a run

CTRL+C signals the running command and any
[background jobs](../guide/commands.md#background-jobs), then waits for the
command to exit; no further steps start. Jobs still running 2 seconds after
that are killed. A second CTRL+C kills the command and the jobs immediately.
Closing the terminal (`SIGHUP`) or sending `SIGTERM` stops a run like CTRL+C.
