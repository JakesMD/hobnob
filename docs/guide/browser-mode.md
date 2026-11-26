# Browser mode

`--web` runs a task behind a local web page and opens it in your default
browser. The page shows what the task is doing, asks its prompts, and ends on a
plain success or failure screen, so someone who never opens a terminal can still
run your setup.

```bash
hobnob --web setup
```

```
$ hobnob --web setup
hobnob » Browser mode: http://127.0.0.1:52814/?token=3f9c…
setup $ npm ci
setup │ added 214 packages
setup ✓ 1.1s
setup ✓ done after 1.1s
```

The terminal shows the usual run log, plus the page's address.

## What the page shows

The page never shows a command. While a task runs it shows the task's
[`info:`](./tasks.md) (or its name when it has none) and the last line of
output, with [secrets](./secrets.md) masked. Variables in `info:` are filled in
as the task starts, so `info: Installing {{.APP}}` reads as it would to the
person. A step with [`quiet:`](./commands.md) shows no output line.

So write tasks for the person watching: give each task an `info:` that says
what it is doing, and have a failing step print why. The failure screen names
the task that failed and its last line of output.

## Offering several tasks

`--web` with no task opens a picker listing every task `hobnob --list` shows,
titled by its `info:`. Choosing one runs it, and the end screen offers **Home**
to pick another.

```bash
hobnob --web
```

The picker opens even when the taskfile has a `default` task. To run `default`
in browser mode, name it: `hobnob default --web`.

## Prompts

[`get:`](./prompts.md) prompts, menus, file and directory pickers, and
[`say:`](./prompts.md#showing-a-message) messages each get a page of their own,
headed by the entry's `info:`, or by the message for a `say:`. They follow the same rules as in the terminal:
`default:`, `optional:`, `multi:`, `match:` and `check:` all behave the same, and
a refused answer shows its reason on the page. Prompts settled by a `KEY=VALUE`
preset are skipped. The page never receives a `secret:` prompt's default.

A web address starting `http://` or `https://` in a `say:` message is a link
that opens in a new tab, so a message can send the person somewhere and keep
the run's page open behind it:

```yaml
tasks:
  setup:
    steps:
      - say: Make an account at https://the-trivia-api.com, then press Continue.
```

## Rules

- **Commands can't read from the terminal.** The browser is the person, so
  don't write `run:` commands that `read` or ask `[y/N]`; use a prompt instead.
- **Cancel on the page is CTRL+C.** The first press asks the running step to
  stop, the second kills it. A cancelled run exits with 1.
- **hobnob waits 60 seconds for a closed tab.** The wait only counts while
  hobnob is waiting on the person: at a prompt, the picker or an end screen.
  When it runs out at a prompt, the run is cancelled as by CTRL+C; anywhere
  else hobnob just exits.
- **The exit code is the last run's**: 0 on success, 1 on failure, 0 when no
  run happened. With a task, hobnob exits once the run ends; without one, it
  stays for Home until CTRL+C or the wait runs out.
- **`--web` combines with** `--file`, `--demo`, `KEY=VALUE` and `--quiet`
  (which changes only the terminal). It is an error with `--no-input`,
  `--list`, `--select` and `--help`, and not yet supported on Windows.
- **The page is local and guarded.** It is served on `127.0.0.1` on a random
  port, and its address carries a random token. Anyone who can read your
  terminal can open it, so treat the address like a password while hobnob runs.
