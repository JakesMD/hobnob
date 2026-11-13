# hobnob guide

A tour of hobnob, one topic at a time. Start anywhere. For exhaustive syntax,
see the [reference](REFERENCE.md).

## Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static binary into `~/.local/bin`, plus completion for bash, zsh and fish.

Nothing written yet? `hobnob --demo ask-trivia` runs a taskfile baked into the
binary.

## Running tasks

Hobnob finds the nearest `hobnob.yml` in the current directory or its
parents. Run a task by name:

```bash
hobnob ask-trivia
```

Bare `hobnob` runs the task named `default`, or opens a picker if there is
none:

```bash
hobnob            # default task, or the picker
hobnob --list     # every task, with its description
hobnob --select   # the picker, even when a default exists
```

Point at a different file with `--file ops/trivia.yml`.

## Writing your first taskfile

A task is a name and a list of steps:

```yaml
tasks:
  ask-trivia:
    info: Ask a random trivia question.
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
```

`info:` is what `--list` and the picker show. No targets, no dependency
graph, no phony declarations.

Each step is exactly one of `run:`, `set:`, `get:`, `call:` or `loop:`. A key
that kind doesn't take, like `soft:` on a `set:`, fails at load rather than
being ignored. See [Steps](REFERENCE.md#steps).

## Asking for input

`get:` prompts for a variable:

```yaml
- get:
    - CATEGORY:
        info: Which category?
        default: general_knowledge
- run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
```

**`get:` is skipped when the variable is already set**, so the same task is
interactive or not depending on how you call it:

```bash
hobnob ask-trivia                     # prompts
hobnob ask-trivia CATEGORY=history    # no prompt
```

Anything that put `CATEGORY` in scope counts: a CLI argument, a `vars:` default,
an earlier step.

> [!TIP]
> Put `get:` steps early, so nobody waits on a slow command before answering.

## Using what a command printed

`into:` captures a command's output into a variable:

```yaml
- run: git rev-parse --short HEAD
  into:
    - SHA: stdout | trim
- run: echo "building {{.SHA}}"
```

`stderr` and `exit` work the same way, and filters chain with `|` exactly as
they do inside `{{ }}`.

## Digging into JSON

Captured output that parses as JSON becomes real data, not text. Dot into it:

```yaml
- run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
  into:
    - QUESTIONS: stdout
- run: echo "{{ .QUESTIONS[0].question.text }} ... {{ .QUESTIONS[0].correctAnswer }}"
```

No parse step, no `jq`. Numbers stay numbers too: capture
`/v2/metadata` as `META`, and ten steps later
`{{ gt .META.byCategory.science 1000 }}` still compares numerically.

Arrays, indexes, slices and wildcards all work:

```yaml
- run: curl -s "https://the-trivia-api.com/v2/questions?limit=5"
  into:
    - QUESTIONS: stdout
- run: echo "{{ .QUESTIONS[0].question.text }}"
- run: echo "{{ .QUESTIONS[*].difficulty }}"   # [hard, hard, easy, hard, medium]
```

> [!NOTE]
> Only captured output and YAML literals become structured. CLI arguments and
> env vars stay text; pipe them through `| json` to parse them. See
> [Types](REFERENCE.md#types).

## Building a prompt from live data

A captured array is a real list, so it can be the menu. The API's `/metadata`
endpoint counts questions per category, and `keys` turns those counts into a
list of category names:

```yaml
- run: curl -s https://the-trivia-api.com/v2/metadata
  into:
    - CATEGORIES: stdout.byCategory | keys
- get:
    - CATEGORY:
        info: Which category?
        options: .CATEGORIES
```

The prompt can only offer categories the API has: a form built from a previous
step's output.

`options: .CATEGORIES` has no `{{ }}`. A field that is one variable reference can
drop the braces, which also keeps the value's type.

## Calling other tasks

`call:` runs another task in a copy of scope. Nothing leaks back unless you
name it in `into:`:

```yaml
- call: _fetch-question
  with:
    - CATEGORY: science
  into:
    - QUESTIONS: .RESPONSE
```

To keep a variable's name, list it bare: `into: [RESPONSE]`. A child's
`_`-prefixed variables, like `_RAW`, are private: `into:` can't read them.

A task that exists to produce a value can declare it with `results:`. Then a
`call:` without `into:` brings it back:

```yaml
tasks:
  _pick-category:
    results: [CATEGORY]
    steps:
      - get:
          - CATEGORY:
              options: [general_knowledge, science]

  ask-trivia:
    steps:
      - call: _pick-category # CATEGORY is now in scope
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
```

`results:` is a promise: after the call, `CATEGORY` exists, whether the task
asked for it or `hobnob ask-trivia CATEGORY=science` already set it. An
`into:` on the call takes over and pulls exactly what it lists instead.

A `_` prefix makes a task internal: it runs, but it is hidden from `--list` and
cannot be named on the command line.

## Doing something only once

A `once: true` task runs at most once per invocation, however many tasks call
it. Later calls replay the first run's result:

```yaml
tasks:
  _categories:
    once: true
    results: [CATEGORY]
    steps:
      - run: curl -s https://the-trivia-api.com/v2/metadata
        into:
          - CATEGORIES: stdout.byCategory | keys
      - get:
          - CATEGORY:
              options: .CATEGORIES

  ask-trivia:
    steps:
      - call: _categories
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
```

Any other task calling `_categories` gets the same answer, with no second
prompt or request. When it replays, hobnob prints what the first run set:
`(cached — CATEGORIES=[…] CATEGORY=history)`.

## Looping over values

Iterate a list, with the current element as `{{.ITEM}}`:

```yaml
- loop: .CATEGORIES
  steps:
    - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.ITEM}}"
```

An object iterates as `{{.KEY}}` and `{{.VALUE}}`. A matrix runs every
combination:

```yaml
- loop:
    CATEGORY: [general_knowledge, science]
    COUNT: [1, 3]
  steps:
    - run: echo "{{.CATEGORY}} x {{.COUNT}}"
```

## Shared values and defaults

Two top-level blocks hold file-wide variables. `const:` is fixed, even against
a CLI argument. `vars:` is a default anyone can override:

```yaml
const:
  - API: https://the-trivia-api.com/v2

vars:
  - CATEGORY: general_knowledge
```

The full order is `env < vars: < env files < CLI args < const:`, and a task's
steps run after all of it. See [Precedence](REFERENCE.md#precedence).

`vars:` can build a default from anything above it: an `env:` file, a CLI arg,
or `const:`:

```yaml
env:
  - .env # API_HOST=staging.example.com

vars:
  - API: "https://{{.API_HOST}}/v1"
```

> [!WARNING]
> `const:` entries can reference only earlier `const:` entries, and no task may
> write to a `const:` name, not even as a `loop:`'s `ITEM`, `KEY` or `VALUE`. An `env:` path cannot reference a `vars:` name;
> select the file with a CLI arg, the OS env or `const:` instead. See
> [Env files](REFERENCE.md#env-files).

## Splitting across files

`modules:` imports another taskfile. A module keeps its details to itself,
which makes it the natural home for a wrapper:

```yaml
# trivia.yml
const:
  - API: https://the-trivia-api.com/v2

tasks:
  random:
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

Imported tasks are namespaced by module key. `API` reaches every task in
`trivia.yml` and stays invisible to the parent.

## Passing variables safely

Write `run:` as a list and it executes directly, with no shell in between:

```yaml
- run: [curl, -s, "{{.API}}/questions?limit=1&categories={{.CATEGORY}}"]
```

Each element is one argument, so a value holding spaces, quotes or a semicolon
arrives intact, and the URL's `&` and `?` are just characters. The string form splices values into text the shell re-parses.

> [!TIP]
> Use the list form whenever an argument contains a variable. Keep the string
> form for pipes, redirects and globs, escaping values with
> [`quote`](REFERENCE.md#filters).

A variable holding a list splices into several arguments. To build one list
from another, spread it with a trailing `...`; without it, a list nests inside
the new one:

```yaml
- set:
    - CURL_OPTS: [-s, --max-time, "10"]
- set:
    - CURL_OPTS: [.CURL_OPTS..., --verbose]
  if: '[ "{{.VERBOSE}}" = true ]'
- run: [curl, .CURL_OPTS, "{{.API}}/questions?limit=1"]
```

## When a command fails

A non-zero exit stops the run. `soft: true` continues past it, and `into:`
still captures so the next step can react:

```yaml
- run: curl -sf {{.API}}/question/no-such-id
  soft: true
  into:
    - CODE: exit
- run: echo "no question with that id"
  if: '{{ ne .CODE 0 }}'
```

`if:` works on any step, and on a whole task.

## Hiding noisy output

`quiet:` replaces a command's output with a one-line message:

```yaml
- run: curl -o questions.json "{{.API}}/questions?limit=10"
  quiet: Downloading ten questions
```

```
⊙ [seed] Downloading ten questions… (output hidden)
```

Output is hidden only on success. A failing quiet step replays everything it
suppressed, so nothing fails silently.

> [!NOTE]
> Jobs a step backgrounds with `&` stop when the step does. End the command
> with `wait` to let them finish, or see
> [background jobs](REFERENCE.md#run-shell-commands) to keep one running.

## Non-interactive runs

Prompts are skipped when stdin is not a terminal, when `CI` is set, or with
`--no-input`. A `get:` then uses its `default:`, or aborts without one. The
same taskfile works for a person and a script:

```bash
hobnob ask-trivia                                  # asks
hobnob ask-trivia CATEGORY=history --no-input      # does not
```

## Conventions

- **Task names** in kebab-case (`ask-trivia`).
- **Variable names** in `ALL_CAPS` with underscores.
- **`info:` first**, in tasks and in `get:` entries.

## Reference

The [reference](REFERENCE.md) has every field, filter and flag:

| Section | Covers |
| --- | --- |
| [CLI](REFERENCE.md#cli) | Every flag, auto-discovery, `--demo`, stopping a run. |
| [File structure](REFERENCE.md#file-structure) | The five top-level keys. |
| [Tasks](REFERENCE.md#tasks) | `info:`, `if:`, `dir:`, `once:` memoization. |
| [Steps](REFERENCE.md#steps) | `set:`, `run:`, `get:`, `call:`, `loop:` in full. |
| [Variables](REFERENCE.md#variables) | Precedence, `const:`/`vars:`, env files, built-ins. |
| [Types](REFERENCE.md#types) | Where structure comes from, and what stays text. |
| [Accessors](REFERENCE.md#accessors) | `.A.b[0][*]`, dynamic keys, slices, absence. |
| [Filters](REFERENCE.md#filters) | `default`, `json`, `keys`, `quote`, comparisons. |
| [Modules](REFERENCE.md#modules) | Importing files, namespaces, scoping. |
