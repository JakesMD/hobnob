# Browser mode

`--web` runs a task behind a local web page and opens it in your default
browser. The page shows what the task is doing, then a plain success or failure
screen, so someone who never opens a terminal can still watch a setup run.

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

The terminal that started hobnob shows the usual run log, plus the page's
address. The page is a setup assistant:

- a **rail** on the left names the task, then lists a **Setup** entry for the
  task and for each task it [calls](./calling-tasks.md), added when it starts,
  ticked when it finishes, and crossed when it fails;
- the **main area** shows the label of the task now running, an activity bar,
  the last line of output (ANSI colour codes removed, [secrets](./secrets.md) masked) and a clock
  of the time since the run began. The label is the task's `info:`, or its
  name when it has none;
- a **bottom bar** holds the buttons for the screen. While steps run it holds
  **Cancel** and a note to keep the window open.

The page never shows a command. A step with [`quiet:`](./commands.md) shows no
output line either.

## Offering several tasks

`--web` with no task opens a **task picker** instead. It lists every task
`hobnob --list` shows: tasks whose name starts with `_`, and tasks from a module
imported with a `_`-prefixed name, are left out. Each task shows its
[`info:`](./tasks.md) as the title with the name smaller beneath it, and a
search box filters the list. Choosing a task runs it.

```bash
hobnob --web
```

The picker opens even when the taskfile has a `default` task. This differs from
the terminal, where a plain `hobnob` runs `default`; to run it in browser mode,
name it: `hobnob default --web`.

When hobnob was started without a task, Done and Failed also offer **Home**,
which returns to the picker for another run. Each run starts with a fresh rail
and log. A task named on the command line ends at Done with no Home.
## Prompts

A [`get:`](./prompts.md) text prompt gets its own page: the entry's `info:` as
a large heading, a text box filled with its `default:` (a password field for
`secret:`; the page never receives a secret's default, and Continue on the untouched field takes it) and **Continue** in the bottom bar, plus **Skip** for an
`optional:` prompt. An answer goes through the same rules as in the terminal:
a required prompt needs an answer, then `match:` and `check:` run, and a
`check:` that runs a shell condition behaves exactly as it does there. A refused
answer keeps you on the page with the reason under the field.

The rail's **Your answers** section gets one entry per answered prompt, labelled
by the entry's `info:` (its variable name without one). A secret shows masked.
A prompt skipped, or settled by a `KEY=VALUE` preset, has no entry; presets
skip their prompts as in the terminal.

A [menu](./prompts.md) (`options:`) shows its options as cards you click: radio
cards for one choice, checkbox cards under `multi:`. With more than six options a
search box filters the cards. The `default:` starts chosen; under `multi:` it is
a JSON list, or one option, and those options start ticked, with a running count
in the bottom bar. A required menu needs a choice (at least one under `multi:`),
and `check:` judges the choice, or the whole list under `multi:`, as in the
terminal; a refusal shows under the cards. Continue on an untouched menu takes
its default. **Skip**, or Continue with nothing ticked on an optional `multi:`
menu, gives no answer.

A [picker](./prompts.md#picking-a-file-or-directory) (`type: file` or `dir`) is
a column view like the Finder's: one column per folder, from your home folder
(or the filesystem root when the start isn't under it) down to the open folder.
It starts in the entry's `default:` folder, as in the terminal. Click a folder to
open it; click a file to pick it. In a `type: dir` picker the open folder is the
answer, so a `default:` folder starts out picked and Continue takes it at once.
Under `multi:` a click ticks or unticks an entry (the `›` beside a folder opens
it) and the bottom bar counts the picks. Entries `match:` excludes are hidden,
not greyed out, as in the terminal; a folder that only fails the plain patterns
of a `type: dir` picker can be opened but not picked. The bottom bar says what is
picked. Continue sends the picks through the same rules as the terminal: they
must exist, be the right kind and match.

There are no admin prompts in browser mode.

## How a run ends

- **Done** shows a tick, "You're all set", the task and how long it took.
- **Failed** names the task whose step failed and the last line it printed, or
  the reason it failed when it printed nothing. **Copy details** copies the
  run's log, with [secrets](./secrets.md) masked, to send to whoever can help.
- **Cancelled** shows when the run was interrupted, by **Cancel** on the page or
  CTRL+C in the terminal.

## Stopping a run

The first press of **Cancel** acts as a first CTRL+C: the running step is asked
to stop, and the page shows "Stopping…" while the button becomes **Force stop**.
A step that ignores the request keeps running until the second press, which acts
as a second CTRL+C and kills it. A run that ends after **Cancel** is Cancelled,
not Failed, and exits like an interrupted run: 1.

CTRL+C in the terminal works as it always does. The page then shows Cancelled,
and if hobnob exits before it can say how the run ended, "hobnob has stopped".
A page already on Done or Failed keeps that screen.

With a task, hobnob exits once the page has been sent how the run ended. The
page keeps that screen after hobnob has gone. Without a task, hobnob stays until
the page is closed or CTRL+C stops it, so Home can start another run. The exit
code is the last run's, as `hobnob <task>` reports it: 0 on success, 1 on
failure, and 0 when no run happened.

## Rules

- `--web` takes an optional task name, and combines with `--file`, `--demo`,
  `KEY=VALUE` and `--quiet`.
- `--quiet` changes only the terminal. The page still shows each task's
  output line.
- `--web` is an error with `--no-input`, `--list`, `--select` and `--help`,
  It is not yet supported on Windows.
- Prompts are on whatever the terminal is: the browser is the person. A command
  can't read from the terminal in browser mode, so don't write `run:` commands
  that `read` or ask `[y/N]`.
- Output is masked on the page exactly as it is in the terminal.

## Security

The page is served on `127.0.0.1` only, on a random free port. Its address
carries a random token, and hobnob refuses every request that:

- lacks the token, or has a wrong one;
- has a `Host` other than that address and port, which stops a web page on
  another site reaching hobnob through a rebinding of its own name;
- has an `Origin` that isn't that address and port. A request with no
  `Origin` is fine.

Anyone who can read your terminal can open the page, so treat the address like
a password for the length of the run.
