# Variables

A variable holds a value so that later steps can use it. Set one with `set:`,
and read it with `{{ .NAME }}`:

```yaml
tasks:
  greet:
    steps:
      - set:
          - NAME: Ada
      - run: echo "Hello, {{ .NAME }}"
```

```
$ hobnob greet
run: [greet] echo "Hello, Ada"
[greet] Hello, Ada
✓ [greet] done
```

The dot in `.NAME` means "the variable called `NAME`". Variable names are
written in `ALL_CAPS` by convention.

## Setting variables in a step

One `set:` step can set several variables. They're set top to bottom, so each
one can use the ones above it:

```yaml
tasks:
  setup:
    steps:
      - set:
          - API: https://the-trivia-api.com/v2
          - RANDOM_URL: "{{ .API }}/questions?limit=1"
```

A variable set in a step exists from that step onwards, until the task ends.

A value doesn't have to be text. Write a YAML list or map and the variable
holds a real list or map, which you can loop over or pick items out of:

```yaml
tasks:
  setup:
    steps:
      - set:
          - CATEGORIES: [history, science]
          - LABELS: { history: History buffs, science: Lab coats }
```

[Structured data](./structured-data.md) covers what you can do with them.

## Variables for the whole file

When several tasks need the same value, set it once at the top of the file
instead of in every task. There are two blocks for this:

- **`vars:`** holds defaults. Whoever runs the task can override them.
- **`const:`** holds fixed values. Nothing can override them.

```yaml
const:
  - API: https://the-trivia-api.com/v2

vars:
  - CATEGORY: general_knowledge

tasks:
  ask:
    steps:
      - run: echo "{{ .API }}/questions?categories={{ .CATEGORY }}"
```

Without arguments, the task uses the defaults. Passing `CATEGORY=science`
overrides the `vars:` default, but passing `API=…` changes nothing, because
`API` is a constant:

```
$ hobnob ask CATEGORY=science
run: [ask] echo "https://the-trivia-api.com/v2/questions?categories=science"
[ask] https://the-trivia-api.com/v2/questions?categories=science
✓ [ask] done
```

Entries in both blocks are written like `set:` entries, and each can use the
ones above it.

## Loading variables from a file

`env:` loads variables from files. It's the place for values that differ
between machines or shouldn't be committed, such as API keys:

```yaml
env:
  - .env
```

```bash
# .env
API_KEY=abc123
```

Each line of `.env` becomes a variable, so a task can read `{{ .API_KEY }}`.
Paths are relative to the hobnob file. To hide values like this one in
hobnob's output, see [Secrets](./secrets.md#marking-a-value-secret).

## Which value wins

The same name can be set in more than one place. Before a task runs, hobnob
gathers variables from five places, and the strongest one wins. From weakest
to strongest:

1. **Your shell's environment variables.** They're the weakest so that
   something set in your terminal can't make a task behave differently on
   your machine.
2. **`vars:`**
3. **`env:` files**
4. **Command-line arguments**, such as `CATEGORY=science`.
5. **`const:`** It beats command-line arguments, which is what makes it
   constant.

Once the task runs, its steps (`set:`, prompts, captured output, loops and
calls) override any of these, and the latest step wins. The one thing a step
can't do is set a `const:` name.

hobnob works them out in the opposite order, strongest first, so each one can
use whatever has already been worked out:

1. **`const:`** can use earlier constants and the
   [built-in variables](#built-in-variables). Nothing else.
2. **Command-line arguments** are taken as given.
3. **`env:` file paths** can use constants, command-line arguments, the
   built-in variables and your shell's environment.
4. **`vars:`** can use all of that, plus the variables the `env:` files
   loaded.
5. **Steps** run last, in order, and can use any variable set before them.

So a `vars:` entry can build its default from an `env:` file:

```yaml
env:
  - .env # API_HOST=staging.example.com

vars:
  - API: "https://{{ .API_HOST }}/v1"
```

Variables also come from [command-line arguments](./running-tasks.md#passing-variables),
[prompts](./prompts.md), [captured output](./capturing-output.md) and
[loops](./loops.md). Each has its own page.

## Built-in variables

hobnob sets two variables itself:

- `HOBNOB_FILE_DIR`: the directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR`: the directory you ran hobnob from.

## Rules

The rules below cover the edge cases. You won't need most of them day to day.

### `const:`

- **A step that sets a `const:` name stops the file loading**, before any
  task runs. That covers `set:`, `get:`, `into:`, `with:` and `loop:`
  anywhere in the file, including the names a loop gives its items (see
  [Loops](./loops.md#rules)).

### `vars:`

- **An entry that's overridden isn't evaluated at all.**
- **An entry can't use its own name.** This fails to load, even when
  `CATEGORY` is set in your shell:

  ```yaml
  vars:
    - CATEGORY: '{{ .CATEGORY | default "general_knowledge" }}' # error
  ```

  A `vars:` entry is already a default, so write `- CATEGORY:
  general_knowledge`. To let a shell variable set the default, read it under
  a different name:

  ```yaml
  vars:
    - CATEGORY: '{{ .TRIVIA_CATEGORY | default "general_knowledge" }}'
  ```

### `env:`

- **Only `.sh` files are run.** Any other file is read as `NAME=value` lines.
  A `.sh` file is run in a separate shell, and only the variables it sets or
  changes are loaded.
- **A missing file prints a warning and is skipped.**
- **Later files override earlier ones.**
