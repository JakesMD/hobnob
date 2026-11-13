# Running tasks

Run a task by giving its name:

```bash
hobnob ask-trivia
```

You can run it from anywhere in your project. hobnob looks for `hobnob.yml`
in the current directory, then in each parent directory in turn, and uses the
first one it finds.

## Picking a task

Run `hobnob` on its own and you don't need to remember any task names. It
opens a menu of every task, with its `info:`, and you pick one.

If the taskfile has a task called `default`, bare `hobnob` runs that
instead. These flags help either way:

```bash
hobnob --list     # print every task, with its info:
hobnob --select   # open the menu, even when there's a default task
```

## Passing variables

Write `NAME=value` after the task name to set a variable for the run:

```bash
hobnob ask-trivia CATEGORY=science COUNT=3
```

A value passed this way:

- **overrides the taskfile's defaults**, but not its constants (see
  [Which value wins](./variables.md#which-value-wins)).
- **answers a prompt** for the same name, so the prompt is skipped. That's
  how a task that asks questions can run in a script (see
  [Running without prompts](./prompts.md#running-without-prompts)).
- **is always text**, even when it looks like JSON (see
  [Text that looks like JSON](./structured-data.md#text-that-looks-like-json)).

## Using another taskfile

`--file` runs a task from a specific file instead of the nearest
`hobnob.yml`:

```bash
hobnob --file ops/trivia.yml ask-trivia
```

The [CLI reference](../reference/cli.md) lists every flag.

## Rules

- **hobnob looks for `hobnob.yml`, then `hobnob.yaml`**, in each directory.
- **Commands run in the taskfile's directory**, wherever you ran hobnob from
  (see [Running in another directory](./commands.md#running-in-another-directory)).
- **Without a terminal, there's no menu.** When hobnob's input isn't a
  terminal or `CI` is set, bare `hobnob` prints the help and the task list
  instead, and `--select` prints the task list. `--select --no-input` does
  the same.
- **Every argument after the task name must be `NAME=value`** or
  `--no-input`. Anything else is an error.
