---
title: "hobnob vs just: wizards next to your justfile"
titleTemplate: false
description: "How hobnob compares with just: where just is the better pick, the same wizard written side by side, and how to run both."
aside: false
pageClass: compare-page
---

# hobnob vs just

[just](https://just.systems) runs recipes from a `justfile`, in a terse
syntax close to make's. It's built for shortcuts: name a command, run it with
`just test`. hobnob is built for wizards: tasks that ask, fetch and decide as
they go.

You don't have to pick one. Keep the justfile for shortcuts, and write the
wizards in hobnob.

## At a glance

|                                 | hobnob                                                        | just                                                      |
| ------------------------------- | ------------------------------------------------------------- | --------------------------------------------------------- |
| A file of short shortcuts       | YAML, a few lines longer                                      | Terse, one line per command                               |
| Arguments                       | `NAME=value`, or asked for when missing                       | Positional, with defaults                                 |
| Recipes in other languages      | Through a script file or `python3 -c`                         | Any `#!` interpreter                                      |
| Running things at once          | Only background jobs inside one command                       | `[parallel]` dependencies                                 |
| Windows                         | No, macOS and Linux                                           | Yes                                                       |
| Asking for a value              | A `get:` step: text box, menu or multi-select, anywhere        | `[confirm]` asks yes or no. Anything else is bash         |
| A menu built from live data     | `options:` from an earlier step's output                      | A pipe into gum or `fzf`, if it's installed               |
| Using an earlier command's output | Any later step, with JSON kept as data                      | Shell variables, inside one `#!` recipe                   |
| Values back from another task   | `results:`, by name                                           | None from a dependency. Capture what a recipe prints      |
| Bare command                    | A menu of tasks                                               | Runs the first recipe. `--choose` opens a fuzzy finder    |

## The same wizard

A pull request review: pick one of the open pull requests, check it out, run
the tests, then approve it or move on.

<div class="side-by-side">
<div>

hobnob

```yaml
tasks:
  review:
    info: Check out a pull request, test it, and approve it.
    steps:
      - run: gh pr list --json headRefName
        quiet: true
        into:
          - BRANCHES: stdout[*].headRefName
      - get:
          - BRANCH:
              info: Which pull request?
              options: .BRANCHES
      - run: gh pr checkout {{.BRANCH}}
      - run: go test ./...
      - get:
          - NEXT:
              info: Tests pass. Approve it, or skip?
              options: [approve, skip]
      - run: gh pr review {{.BRANCH}} --approve
        if: '{{ eq .NEXT "approve" }}'
```

</div>
<div>

just

```make
# Check out a pull request, test it, and approve it.
review branch="" next="":
    #!/usr/bin/env bash
    set -euo pipefail
    branch="{{branch}}"
    if [ -z "$branch" ]; then
      branch=$(gh pr list --json headRefName --jq '.[].headRefName' \
        | fzf --prompt "Which pull request? ")
    fi
    gh pr checkout "$branch"
    go test ./...
    next="{{next}}"
    until [ "$next" = approve ] || [ "$next" = skip ]; do
      read -rp "Tests pass. Approve it, or skip? " next
    done
    if [ "$next" = approve ]; then gh pr review "$branch" --approve; fi
```

</div>
</div>

Both ask for what isn't passed in, and both take it from the command line:
`hobnob review BRANCH=fix-login NEXT=approve`, or
`just review fix-login approve`. Left out, hobnob shows a menu of the open
pull requests:

```
$ hobnob review
run: [review] gh pr list --json headRefName
⊙ [review] output hidden

Select a value for BRANCH.
Which pull request?
▶ fix-login
  add-search
  bump-deps
↑↓ move  enter select
```

The differences:

- **The menu needs another tool.** just has no way to ask for an argument,
  so the list of pull requests goes through
  [fzf](https://github.com/junegunn/fzf), which everyone who runs the
  recipe has to install. In hobnob, `options:` takes the list straight from
  the step before.
- **The second question is hand-written.** It comes after the tests, so it
  can't be an argument check. In just it's an `until` loop around `read`. In
  hobnob it's another [`get:`](../guide/prompts.md), placed after the step
  it follows.
- **The recipe became a bash script.** Each line of a just recipe runs in
  its own shell. Once lines share `branch` and `next`, the recipe needs a
  `#!` line, and just stops printing each command before it runs. hobnob's
  steps share values themselves, and each is still printed.
- **Running without a terminal.** Without the arguments, fzf fails with an
  error of its own, and `read` exits with no message at all. hobnob stops
  and says which `NAME=` to pass.

A recipe like this is a [wizard written as a script](./bash.md), with the
same costs.

## Where just is the better pick

- **Shortcuts.** A justfile of one-line commands is terser than the same
  tasks in YAML, and that's most of what a justfile holds.
- **Arguments.** Recipes take positional arguments with defaults, such as
  `just test unit`. hobnob only takes `NAME=value`.
- **Any language.** A recipe that starts with `#!` runs as a script in
  Python, Node or anything else. hobnob runs its commands with `sh`, so other
  languages go through a script file or a command such as `python3 -c`.
- **Running things at the same time.** `[parallel]` runs a recipe's
  dependencies at once. hobnob runs steps one after another, and overlaps
  work only through
  [background jobs](../guide/commands.md#background-jobs) inside one command.
- **Windows.** just runs on Windows. hobnob is released for macOS and Linux.

## Running both

Each can call the other. A recipe can hand over to a hobnob wizard, passing
its arguments along:

```make
# Check out a pull request, test it, and approve it.
[positional-arguments]
review *args:
    hobnob review "$@"
```

```bash
just review                                  # asks
just review BRANCH=fix-login NEXT=approve    # doesn't
```

And a hobnob wizard can use a just shortcut:

```yaml
tasks:
  release:
    steps:
      - get:
          - ENV:
              options: [staging, production]
      - run: just test
      - run: ./scripts/deploy.sh {{.ENV}}
```

## If you move a recipe over

| just                                  | hobnob                                                                     |
| ------------------------------------- | -------------------------------------------------------------------------- |
| A `#` comment above a recipe          | [`info:`](../guide/tasks.md#describing-a-task)                             |
| A recipe's lines                      | `steps:`, each a [`run:`](../guide/commands.md)                            |
| A recipe argument                     | A [`get:`](../guide/prompts.md) step, answered with `hobnob task NAME=value` |
| A dependency, as in `deploy: build`   | A [`call:`](../guide/calling-tasks.md) step                                |
| A recipe that prints a value          | A task with [`results:`](../guide/calling-tasks.md#returning-values-with-results) |
| A shell variable in a `#!` recipe     | A `run:` step with [`into:`](../guide/capturing-output.md)                 |
| `name := "value"`                     | [`vars:`](../guide/variables.md#variables-for-the-whole-file)              |
| ``name := `command` ``                | A `run:` step with [`into:`](../guide/capturing-output.md)                 |
| `set dotenv-load`                     | [`env:`](../guide/variables.md#loading-variables-from-a-file)              |
| `mod name`                            | [`modules:`](../guide/modules.md)                                          |
| `[private]`, or a name starting `_`   | A task name starting with `_` (see [Helper tasks](../guide/tasks.md#helper-tasks)) |
| `[confirm]`                           | A `get:` menu of `no` and `yes`, then a step with `if:`                    |
| `just --choose`                       | `hobnob`                                                                   |

<p class="compare-checked">Last checked November 2026.</p>
