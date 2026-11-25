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
- a **bottom bar** holds the buttons for the screen.

The page never shows a command. A step with [`quiet:`](./commands.md) shows no
output line either.

## How a run ends

- **Done** shows a tick, "You're all set", the task and how long it took.
- **Failed** names the task whose step failed and the last line it printed, or
  the reason it failed when it printed nothing. **Copy details** copies the
  run's log, with [secrets](./secrets.md) masked, to send to whoever can help.
- **Cancelled** shows when the run was interrupted.

hobnob exits once the page has been sent how the run ended. The page keeps
that screen after hobnob has gone. The exit code is the one `hobnob <task>`
reports: 0 on success, 1 on failure.

## Rules

- `--web` takes a task name, and combines with `--file`, `--demo`,
  `KEY=VALUE` and `--quiet`.
- `--quiet` changes only the terminal. The page still shows each task's
  output line.
- `--web` is an error with `--no-input`, `--list`, `--select` and `--help`,
  and with no task. It is not yet supported on Windows.
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
