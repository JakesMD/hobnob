# Tasks

A taskfile is a file called `hobnob.yml` at the top of your project. It
holds your project's tasks under `tasks:`. Each task has a name and a list of
steps, which run from top to bottom:

```yaml
tasks:
  ask-trivia:
    info: Ask a random trivia question.
    steps:
      - run: echo "Which planet is closest to the sun?"
      - run: echo "Mercury"
```

```
$ hobnob ask-trivia
run: [ask-trivia] echo "Which planet is closest to the sun?"
[ask-trivia] Which planet is closest to the sun?
run: [ask-trivia] echo "Mercury"
[ask-trivia] Mercury
✓ [ask-trivia] done
```

hobnob prints each command before running it, then what the command prints,
then `✓` once the task is done.

## Describing a task

`info:` is a one-line description. hobnob shows it beside the task's name in
`--list` and in the picker:

```
$ hobnob --list
Available tasks for this project:
• ask-trivia  Ask a random trivia question.
```

## Kinds of step

Every step does one of five things, and most of this guide is about them:

| Step    | Does                                      | Page                                |
| ------- | ----------------------------------------- | ----------------------------------- |
| `run:`  | Runs a command.                           | [Running commands](./commands.md)   |
| `set:`  | Sets variables.                           | [Variables](./variables.md)         |
| `get:`  | Asks for a value.                         | [Prompts](./prompts.md)             |
| `loop:` | Runs steps once per item.                 | [Loops](./loops.md)                 |
| `call:` | Runs another task and brings values back. | [Calling tasks](./calling-tasks.md) |

Any step can also take [`if:`](./conditions.md) to run only when a condition
holds. The [schema](../reference/schema.md#steps) lists every key each kind
of step takes.

## Task options

Besides `info:` and `steps:`, a task can take a few options that apply to all
its steps. Each is explained on the page that needs it:

- [`if:`](./conditions.md#skipping-a-whole-task) skips the task unless a
  condition holds.
- [`dir:`](./commands.md#running-in-another-directory) runs its commands in
  another directory.
- [`results:`](./calling-tasks.md#returning-values-with-results) lists the
  values it hands back to a task that calls it.
- [`once:`](./running-once.md) runs it only once, however many tasks call it.

## Helper tasks

Start a task's name with `_`, as in `_fetch-question`, to make it a helper.
It's left out of `--list` and the picker, and only another task can
[call](./calling-tasks.md) it.

## Naming

- **Task names** are kebab-case: `ask-trivia`.
- **Variable names** are `ALL_CAPS` with underscores: `CATEGORY`,
  `TIMEOUT_SECS`.
- **`info:` comes first**, in tasks and in `get:` entries, so it's the first
  thing you read.

## Rules

- **Running a helper task directly is an error**:
  `hobnob _fetch-question` fails with
  `task "_fetch-question" is internal and cannot be called directly`.
