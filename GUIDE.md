# hobnob guide

A tour of hobnob, one topic at a time. Start anywhere. For exhaustive syntax,
see the [reference](REFERENCE.md).

## Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static binary into `~/.local/bin`, plus completion for bash, zsh and fish.

Nothing written yet? `hobnob --demo tell-joke` runs a taskfile baked into the
binary.

## Running tasks

Hobnob finds the nearest `hobnob.yml` in the current directory or its
parents. Run a task by name:

```bash
hobnob tell-joke
```

Bare `hobnob` runs the task named `default`, or opens a picker if there is
none:

```bash
hobnob            # default task, or the picker
hobnob --list     # every task, with its description
hobnob --select   # the picker, even when a default exists
```

Point at a different file with `--file ops/jokes.yml`.

## Writing your first taskfile

A task is a name and a list of steps:

```yaml
tasks:
  tell-joke:
    info: Tell a random joke.
    steps:
      - run: curl -s https://official-joke-api.appspot.com/random_joke
```

`info:` is what `--list` and the picker show. No targets, no dependency
graph, no phony declarations.

## Asking for input

`get:` prompts for a variable:

```yaml
- get:
    - TYPE:
        info: Which kind of joke?
        default: general
- run: curl -s https://official-joke-api.appspot.com/jokes/{{.TYPE}}/random
```

**`get:` is skipped when the variable is already set**, so the same task is
interactive or not depending on how you call it:

```bash
hobnob tell-joke              # prompts
hobnob tell-joke TYPE=dad     # no prompt
```

Anything that put `TYPE` in scope counts: a CLI argument, a `vars:` default,
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
- run: curl -s https://official-joke-api.appspot.com/random_joke
  into:
    - JOKE: stdout
- run: echo "{{ .JOKE.setup }} ... {{ .JOKE.punchline }}"
```

No parse step, no `jq`. Ten steps later `.JOKE.id` is still a number, so
`{{ gt .JOKE.id 400 }}` compares numerically.

Arrays, indexes, slices and wildcards all work:

```yaml
- run: curl -s https://official-joke-api.appspot.com/jokes/random/5
  into:
    - JOKES: stdout
- run: echo "{{ .JOKES[0].setup }}"
- run: echo "{{ .JOKES[*].id }}"      # [99, 32, 218, 407, 209]
```

> [!NOTE]
> Only captured output and YAML literals become structured. CLI arguments and
> env vars stay text; pipe them through `| json` to parse them. See
> [Types](REFERENCE.md#types).

## Building a prompt from live data

A captured array is a real list, so it can be the menu. Let the API's `/types`
endpoint populate the prompt:

```yaml
- run: curl -s https://official-joke-api.appspot.com/types
  into:
    - TYPES: stdout
- get:
    - TYPE:
        info: Which kind of joke?
        options: .TYPES
```

The prompt can only offer types the API has: a form built from a previous
step's output.

`options: .TYPES` has no `{{ }}`. A field that is one variable reference can
drop the braces, which also keeps the value's type.

## Calling other tasks

`call:` runs another task in a copy of scope. Nothing leaks back unless you
name it in `into:`:

```yaml
- call: _fetch-joke
  with:
    - TYPE: programming
  into:
    - JOKE: .RESPONSE
```

A `_` prefix makes a task internal: it runs, but it is hidden from `--list` and
cannot be named on the command line.

## Doing something only once

A `once: true` task runs at most once per invocation, however many tasks call
it. Later calls replay the first run's result:

```yaml
tasks:
  _types:
    once: true
    steps:
      - run: curl -s https://official-joke-api.appspot.com/types
        into:
          - TYPES: stdout
      - get:
          - TYPE:
              options: .TYPES

  tell-joke:
    steps:
      - call: _types
        into: [{ TYPE: .TYPE }]
      - run: curl -s https://official-joke-api.appspot.com/jokes/{{.TYPE}}/random
```

Any other task calling `_types` gets the same answer, with no second prompt or
request. Hobnob prints `(cached — TYPE=dad …)` when it replays.

## Looping over values

Iterate a list, with the current element as `{{.ITEM}}`:

```yaml
- loop: .TYPES
  steps:
    - run: curl -s https://official-joke-api.appspot.com/jokes/{{.ITEM}}/random
```

An object iterates as `{{.KEY}}` and `{{.VALUE}}`. A matrix runs every
combination:

```yaml
- loop:
    TYPE: [general, programming]
    COUNT: [1, 3]
  steps:
    - run: echo "{{.TYPE}} x {{.COUNT}}"
```

## Shared values and defaults

Two top-level blocks hold file-wide variables. `const:` is fixed, even against
a CLI argument. `vars:` is a default anyone can override:

```yaml
const:
  - API: https://official-joke-api.appspot.com

vars:
  - TYPE: general
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
> write to a `const:` name. An `env:` path cannot reference a `vars:` name;
> select the file with a CLI arg, the OS env or `const:` instead. See
> [Env files](REFERENCE.md#env-files).

## Splitting across files

`modules:` imports another taskfile. A module keeps its details to itself,
which makes it the natural home for a wrapper:

```yaml
# jokes.yml
const:
  - API: https://official-joke-api.appspot.com

tasks:
  random:
    steps:
      - run: [curl, -s, "{{.API}}/random_joke"]
```

```yaml
# hobnob.yml
modules:
  - jokes: ./jokes.yml

tasks:
  standup:
    steps:
      - call: jokes:random
```

Imported tasks are namespaced by module key. `API` reaches every task in
`jokes.yml` and stays invisible to the parent.

## Passing variables safely

Write `run:` as a list and it executes directly, with no shell in between:

```yaml
- run: [curl, -s, "{{.API}}/jokes/{{.TYPE}}/random"]
```

Each element is one argument, so a value holding spaces, quotes or a semicolon
arrives intact. The string form splices values into text the shell re-parses.

> [!TIP]
> Use the list form whenever an argument contains a variable. Keep the string
> form for pipes, redirects and globs, escaping values with
> [`quote`](REFERENCE.md#filters).

## When a command fails

A non-zero exit stops the run. `soft: true` continues past it, and `into:`
still captures so the next step can react:

```yaml
- run: curl -sf {{.API}}/jokes/999999
  soft: true
  into:
    - CODE: exit
- run: echo "no joke with that id"
  if: '{{ ne .CODE 0 }}'
```

`if:` works on any step, and on a whole task.

## Hiding noisy output

`quiet:` replaces a command's output with a one-line message:

```yaml
- run: curl -o jokes.json {{.API}}/jokes/ten
  quiet: Downloading ten jokes
```

```
⊙ [seed] Downloading ten jokes… (output hidden)
```

Output is hidden only on success. A failing quiet step replays everything it
suppressed, so nothing fails silently.

## Non-interactive runs

Prompts are skipped when stdin is not a terminal, when `CI` is set, or with
`--no-input`. A `get:` then uses its `default:`, or aborts without one. The
same taskfile works for a person and a script:

```bash
hobnob tell-joke                          # asks
hobnob tell-joke TYPE=dad --no-input      # does not
```

## Conventions

- **Task names** in kebab-case (`tell-joke`).
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
