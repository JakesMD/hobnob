---
title: "hobnob vs make: wizards next to your Makefile"
titleTemplate: false
description: "How hobnob compares with make: where make is the better pick, the same wizard written side by side, and how to run both."
aside: false
pageClass: compare-page
---

# hobnob vs make

[make](https://www.gnu.org/software/make/) is a build tool. It turns files
into other files, and rebuilds only what changed. Many projects also use it
as a list of commands, as `.PHONY` targets.

hobnob is neither. It's for wizards: tasks that ask, fetch and decide as
they go. Keep make for building, and write the wizards in hobnob.

## At a glance

|                                   | hobnob                                                  | make                                                         |
| --------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------ |
| Skipping work that's up to date   | No, every task runs every time                          | Yes, by comparing files' timestamps                          |
| Running things at once            | Only background jobs                                    | `make -j`                                                    |
| Already installed                 | No, one binary to add                                   | With macOS's developer tools and almost every Linux          |
| Asking for a value                | A `get:` step: text box, menu or multi-select           | `read` in a command, by hand                                 |
| Checking a value                  | `check:`, which asks again until it passes              | A shell test that fails the target                           |
| Using a command's output later    | Any later step, with JSON kept as data                  | One shell line joined with `\`, or `$(shell)` before anything runs |
| A list of the tasks               | Built in                                                | A hand-written `help` target                                 |
| The shell's `$`                   | Plain `$`. hobnob's own variables are `{{ }}`           | `$$`, and every command line starts with a tab               |

## The same wizard

Tag a release: show the last version, ask for the new one, check it looks
like a version, confirm, then tag and push.

<div class="side-by-side">
<div>

hobnob

```yaml
tasks:
  release:
    info: Tag and push a new release.
    steps:
      - run: git describe --tags --abbrev=0
        quiet: true
        into:
          - LAST: stdout | trim
      - get:
          - VERSION:
              info: Last release was {{.LAST}}. New version?
              check: "echo {{ .VERSION | quote }} | grep -Eq '^v[0-9]+[.][0-9]+[.][0-9]+$'"
          - SURE:
              info: Tag and push {{.VERSION}}?
              options: ["no", "yes"]
      - run: echo "Nothing tagged." && exit 1
        if: '{{ eq .SURE "no" }}'
      - run: git tag {{.VERSION}}
      - run: git push origin {{.VERSION}}
```

```
$ hobnob release
release $ git describe --tags --abbrev=0 # output hidden
release ✓ 0.0s
╭─ release » VERSION ──────────────────────────────────────────────────────╮
│                                                                          │
│  Last release was v1.4.2. New version?                                   │
│                                                                          │
│  › 1.5                                                                   │
│                                                                          │
│  ✗ validation failed: echo {{ .VERSION | quote }} | grep -Eq '^v[0-      │
│  9]+[.][0-9]+[.][0-9]+$'                                                 │
│                                                                          │
╰──────────────────────────────────────────────────────────────────────────╯
 enter confirm
```

</div>
<div>

make

```make
.PHONY: release

release: ## Tag and push a new release. Pass VERSION=vX.Y.Z, and SURE=y to skip the question.
	@echo "Last release was $$(git describe --tags --abbrev=0)."
	@echo "$(VERSION)" | grep -Eq '^v[0-9]+[.][0-9]+[.][0-9]+$$' || { echo "Pass VERSION=vX.Y.Z"; exit 1; }
	@[ "$(SURE)" = y ] || { printf "Tag and push $(VERSION)? [y/N] "; read sure; [ "$$sure" = y ]; }
	git tag $(VERSION)
	git push origin $(VERSION)
```

```
$ make release
Last release was v1.4.2.
Pass VERSION=vX.Y.Z
make: *** [release] Error 1
```

</div>
</div>

In CI, both take the answers on the command line:
`hobnob release VERSION=v1.5.0 SURE=yes`, or
`make release VERSION=v1.5.0 SURE=y`.

The differences:

- **make can't ask for the version.** A target can't stop to ask for a
  variable, so `make release` without `VERSION` fails, and you run it again.
  You find out the last version from the error. hobnob shows it in the
  question, and asks until the answer passes `check:`.
- **The confirmation is hand-written.** In make it's `read`, wrapped in a test
  so CI can skip it with `SURE=y`. In hobnob it's a menu, and `SURE=yes`
  answers it like any other question.
- **The escaping and the tabs.** In a Makefile, `$` belongs to make, so the
  shell's `$` is written `$$`, even inside the regular expression. Its
  commands must start with a tab, not spaces. hobnob's file is plain YAML,
  and its `{{ }}` never clashes with the shell.
- **Output from one line is gone on the next.** Each line of a target runs in
  its own shell. Here that's fine, but anything a later line needs has to be
  fetched again, or the lines joined with `\`. GNU make's `.ONESHELL` runs a
  whole target in one shell, but the make 3.81 that comes with macOS
  ignores it. `$(shell …)` doesn't help either, because it runs before the
  target's first command. hobnob's [`into:`](../guide/capturing-output.md)
  keeps any step's output, as data, for the steps after it.

## Where make is the better pick

- **Building files.** make knows which files a target makes and which it
  needs, and skips the target when nothing changed. hobnob runs every task,
  every time.
- **Running things at the same time.** `make -j` builds independent targets
  in parallel. hobnob runs steps one after another, and overlaps work only
  through [background jobs](../guide/commands.md#background-jobs), which keep
  running while later steps run.
- **Already installed.** make comes with macOS's developer tools and almost
  every Linux. hobnob is one more binary to install, though it's one
  command (see [Installation](../installation.md)).

## Running both

A hobnob wizard can call make, which still skips what's up to date:

```yaml
tasks:
  release:
    steps:
      - get:
          - VERSION:
              info: New version?
      - run: make -j
      - run: git tag {{.VERSION}}
```

And a Makefile can keep its familiar target names, with a wizard behind one:

```make
.PHONY: release
release:
	hobnob release
```

## If you move a target over

| make                                    | hobnob                                                                  |
| --------------------------------------- | ----------------------------------------------------------------------- |
| A `.PHONY` target                       | A [task](../guide/tasks.md)                                             |
| A `## comment` after a target           | [`info:`](../guide/tasks.md#describing-a-task)                          |
| A target's command lines                | `steps:`, each a [`run:`](../guide/commands.md)                         |
| Prerequisites, as in `deploy: test`     | A [`call:`](../guide/calling-tasks.md) step                             |
| `NAME := value` or `NAME ?= value`      | [`vars:`](../guide/variables.md#variables-for-the-whole-file)           |
| `$(shell command)`                      | A `run:` step with [`into:`](../guide/capturing-output.md)              |
| `$$` for the shell's `$`                | Plain `$`. hobnob's own variables are `{{ .NAME }}`                     |
| `include other.mk`                      | [`modules:`](../guide/modules.md)                                       |
| A `help` target                         | Bare `hobnob`, or `hobnob --list`                                       |
| `-include .env` and `export`            | [`env:`](../guide/variables.md#loading-variables-from-a-file)           |
| A check that `VERSION` was passed       | A [`get:`](../guide/prompts.md) step, with `check:`                     |
| `read` in a command                     | A `get:` step                                                           |

<p class="compare-checked">Last checked November 2026.</p>
