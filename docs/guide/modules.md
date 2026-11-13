# Modules

As a project grows, you can split its tasks across several files. List
another taskfile under `modules:`, and its tasks become available under a
prefix:

```yaml
# trivia.yml
const:
  - API: https://the-trivia-api.com/v2

tasks:
  random:
    info: Fetch a random question.
    steps:
      - run: [curl, -s, "{{.API}}/questions?limit=1"]
```

```yaml
# hobnob.yml
modules:
  - trivia: ./trivia.yml

tasks:
  standup:
    steps:
      - call: trivia:random
```

```
$ hobnob --list
Available tasks for this project:
• standup
• trivia:random       Fetch a random question.
```

`trivia` is the module's key, and every task from `trivia.yml` is named
`trivia:` something. You can run them, call them and pick them like any
other task.

A module keeps its own details to itself. `API` is available to every task in
`trivia.yml`, but the tasks in `hobnob.yml` can't see it.

## Choosing what to import

Instead of a path, a module can take options:

```yaml
modules:
  - trivia: ./trivia.yml
  - report:
      path: ./reporting.yml
      show: [daily, weekly]
      flatten: true
```

| Option     | Does                                                         |
| ---------- | ------------------------------------------------------------ |
| `path:`    | The file to import, relative to the file importing it.       |
| `show:`    | Imports only these tasks.                                    |
| `hide:`    | Imports every task except these.                             |
| `flatten:` | Also makes each task available without its prefix: `daily`.  |

To hide a whole module from `--list` and the picker, start its key with `_`,
as in `_drafts`. Other tasks can still call its tasks, but you can't run them
directly.

## Variables inside a module

A module can have its own `const:`, `vars:` and `env:`. They apply only to
the module's tasks, never to the file that imports it.

When a module's task runs, it sees, from weakest to strongest:

1. **The module's own variables**, from its `vars:` and `env:`.
2. **Its caller's variables**, as any [called task](./calling-tasks.md)
   does. A caller's `set:` or `with:` overrides the module's `vars:`.
3. **The module's `const:`**, which always wins, even over the importer's
   `const:` or a command-line argument.

## Rules

### Importing

- **`show:` and `hide:` can be combined.** `show:` picks the tasks, then
  `hide:` drops some of those.
- **A module's own `_` tasks are never imported.** Its other tasks can still
  call them.
- **With `flatten:`, `--list` shows the bare name**, though the prefixed
  name still works.
- **With `flatten:`, a name that's already taken keeps its owner.** If a task
  in the importing file, or in an earlier flattened module, already has the
  bare name, the module's task is only available with its prefix.

### Working directory

- **A module task's `dir:` is relative to the module's file.** Without a
  `dir:`, it runs wherever its caller runs, as any
  [called task](./commands.md#running-in-another-directory) does.

### Variables

- **A module's variables are worked out once, when the files load**, the
  same way as the main file's (see
  [Which value wins](./variables.md#which-value-wins)). They can build on the
  importer's command-line arguments and constants, but not on anything a
  caller sets in a step.
- **A module's `env:` and `vars:` only fill in names nothing else has set.**
  With `STAGE=prod` on the command line, a module whose `.env` sets
  `STAGE=dev` uses `prod`. A name set only in your shell's environment
  counts as not set.
