# Taskfile schema

Every key hobnob reads, as tables. Each row links to the guide page that
explains it.

```yaml
const: # fixed values, nothing outside the file can override
  - API: https://the-trivia-api.com/v2

vars: # overridable defaults
  - CATEGORY: general_knowledge

env: # files to source
  - .env

modules: # imported hobnob files
  - report: ./reporting.yml

tasks:
  ask-trivia:
    steps:
      - run: curl -s "{{.API}}/questions?limit=1&categories={{.CATEGORY}}"
```

## Top level

All five keys are optional.

| Key        | Holds                                              | Guide                                                   |
| ---------- | -------------------------------------------------- | ------------------------------------------------------- |
| `const:`   | Fixed values; nothing outside the file overrides.  | [Variables](../guide/variables.md#variables-for-the-whole-file)       |
| `vars:`    | Overridable defaults.                              | [Variables](../guide/variables.md#variables-for-the-whole-file)       |
| `env:`     | Files to source.                                   | [Env files](../guide/variables.md#loading-variables-from-a-file)            |
| `modules:` | Imported hobnob files.                             | [Modules](../guide/modules.md)                          |
| `tasks:`   | The tasks, by name. A `_` prefix makes one internal. | [Tasks](../guide/tasks.md)                              |

## Tasks

| Key        | Meaning                                                 | Guide                                                              |
| ---------- | ------------------------------------------------------- | ------------------------------------------------------------------ |
| `info:`    | One-line description, shown in `--list` and the picker. | [Tasks](../guide/tasks.md)                                         |
| `steps:`   | The sequence to run.                                    | [Tasks](../guide/tasks.md)                                         |
| `if:`      | Shell condition; non-zero skips the whole task.         | [Conditions](../guide/conditions.md)                               |
| `dir:`     | Working directory for the task's `run:` steps.          | [Working directory](../guide/commands.md#running-in-another-directory)        |
| `once:`    | Memoize: run at most once per invocation.               | [Running once](../guide/running-once.md)                           |
| `results:` | Variables a `call:` without `into:` hands back.         | [Calling tasks](../guide/calling-tasks.md#returning-values-with-results)    |

## Steps

A step names exactly one kind, and takes only that kind's keys. Every kind
also takes `if:`.

| Kind    | Keys besides `if:`                 | Guide                                                        |
| ------- | ---------------------------------- | ------------------------------------------------------------ |
| `set:`  | none                               | [Variables](../guide/variables.md#setting-variables-in-a-step) |
| `run:`  | `into:`, `dir:`, `soft:`, `quiet:` | [Running commands](../guide/commands.md)                     |
| `get:`  | none                               | [Prompts](../guide/prompts.md)                               |
| `call:` | `with:`, `into:`, `dir:`, `soft:`  | [Calling tasks](../guide/calling-tasks.md)                   |
| `loop:` | `steps:`                           | [Loops](../guide/loops.md)                                   |

### `set:` entries

| Form                                | Meaning                                         |
| ----------------------------------- | ----------------------------------------------- |
| `- NAME: value`                     | A template, a bare reference or a literal.       |
| `- NAME: { value: …, secret: true }` | Same, masked wherever hobnob shows it.          |

`const:` and `vars:` entries take the same forms. See [Secrets](../guide/secrets.md).

### `run:` keys

| Key      | Meaning                                                       | Guide                                                    |
| -------- | ------------------------------------------------------------- | -------------------------------------------------------- |
| `into:`  | Capture `stdout`, `stderr` or `exit` into variables.          | [What you can capture](../guide/capturing-output.md#what-you-can-capture) |
| `dir:`   | Working directory, this step only.                            | [Working directory](../guide/commands.md#running-in-another-directory)       |
| `soft:`  | A non-zero exit continues the timeline instead of halting it. | [Errors](../guide/errors.md)                             |
| `quiet:` | Hide output on success.                                       | [Hiding noisy output](../guide/commands.md#hiding-noisy-output) |

A string runs through the shell; a list runs directly, one element per
argument. See [Running commands](../guide/commands.md).

### `get:` keys

| Key         | Meaning                                                           |
| ----------- | ----------------------------------------------------------------- |
| `info:`     | Prompt text.                                                      |
| `default:`  | Pre-filled value, used as-is when prompts are off.                |
| `options:`  | Turns it into a select. A list literal or a variable holding one. |
| `multi:`    | Multi-select; the result is an array.                             |
| `check:`    | Shell condition the answer must satisfy (exit 0).                 |
| `secret:`   | Mask wherever hobnob shows it; command output is not.             |
| `optional:` | Skip silently if unanswered, leaving the variable empty.          |

See [Prompts](../guide/prompts.md).

### `call:` keys

| Key      | Meaning                                                        |
| -------- | -------------------------------------------------------------- |
| `call:`  | The task to run, `module:task` for an imported one.            |
| `with:`  | Variables set in the child's copy of scope.                    |
| `into:`  | Variables pulled back from the child; replaces its `results:`. |
| `dir:`   | Working directory for the child; beats the child's own `dir:`. |
| `soft:`  | A failed call continues the timeline instead of halting it.    |

See [Calling tasks](../guide/calling-tasks.md).

### `loop:` forms

| Form                       | Iterates                         | Variables          |
| -------------------------- | -------------------------------- | ------------------ |
| `loop: .LIST`              | An array, in order.              | `ITEM`             |
| `loop: .OBJECT`            | An object, in sorted-key order.  | `KEY`, `VALUE`     |
| `loop: { A: […], B: […] }` | Every combination of the arrays. | `A`, `B`           |

See [Loops](../guide/loops.md).

## Modules

| Key        | Meaning                                             |
| ---------- | --------------------------------------------------- |
| `path:`    | File to import, relative to the importing taskfile. |
| `show:`    | Whitelist. Only these task names are imported.      |
| `hide:`    | Blacklist. Everything else is imported.             |
| `flatten:` | Also register tasks under their bare name.          |

The short form `- key: ./file.yml` sets only `path:`. See
[Modules](../guide/modules.md).

## Built-in variables

| Name                    | Holds                                |
| ----------------------- | ------------------------------------ |
| `HOBNOB_FILE_DIR`       | Directory containing the hobnob file. |
| `HOBNOB_INVOCATION_DIR` | Directory hobnob was run from.       |
