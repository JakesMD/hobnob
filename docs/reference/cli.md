# CLI

```
hobnob [--file <path> | --demo] [<task>] [KEY=VALUE ...] [--no-input]
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
name, which is what makes an interactive task scriptable. Values are always
text; see [Text that looks like JSON](../guide/structured-data.md#text-that-looks-like-json).

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

Prints every public task with its `info:`. Internal tasks and modules are
omitted.

## `--select`

Opens the picker even when a `default` task exists. With no terminal, under
CI, or with `--no-input`, it prints the task list instead.

## `--help`

Prints usage, the docs links, and the task list.

## `--version`

Prints the version. Works outside any taskfile.

## `--upgrade`

Replaces the running binary with the latest release. Works outside any
taskfile.

## `completion <shell>`

Prints the completion script for `bash`, `zsh` or `fish`. The installer wires
it up; run it yourself only to regenerate.

```bash
hobnob completion zsh > ~/.zsh/completions/_hobnob
```

## Stopping a run

CTRL+C signals the running command and waits for it to exit; no further steps
start. A second CTRL+C kills it immediately.
