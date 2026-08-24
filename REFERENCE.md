# hobnob reference

Every YAML field, template filter and CLI flag. New to hobnob? Start with the
[guide](GUIDE.md), which builds a working taskfile from scratch.

---

## CLI

```
hobnob [--file <path> | --demo] [<task>] [KEY=VALUE ...] [--no-input]
hobnob [--file <path> | --demo] (--list | --select | --help)
hobnob (--version | --upgrade | completion <shell>)
```

### `<task>`

The task to run, always the first argument. Without one, hobnob runs the task
named `default`; with no `default` task, it opens the picker.

```bash
hobnob tell-joke
```

A task whose name starts with `_` is internal and cannot be named here, only
reached by `call:` from another task.

### `KEY=VALUE`

Sets a variable for the run, above `vars:` and env files but below `const:`
(see [Precedence](#precedence)). Repeatable, and it satisfies a `get:` prompt
for the same name, which is what makes an interactive task scriptable.

```bash
hobnob tell-joke TYPE=programming COUNT=3
```

Values are always text, however JSON-shaped they look; see [Types](#types).

### `--no-input`

Skips every prompt. A `get:` with a `default:` uses it; one without aborts the
run. Implied by a `CI` env var or by stdin not being a terminal, so AI agents
and scripts get it without asking.

```bash
hobnob tell-joke --no-input
```

### `--file <path>`

Uses a specific taskfile instead of searching for one.

```bash
hobnob --file ops/jokes.yml tell-joke
```

Without it, hobnob searches upward from the current directory for `hobnob.yml`
or `hobnob.yaml`, taking the first hit (`.yml` wins when both sit in the same
directory). Relative paths inside the file resolve against the file's own
directory, not where you ran from.

### `--demo`

Runs a small built-in taskfile instead of one of yours.

```bash
hobnob --demo tell-joke
hobnob --demo --list
```

It is an alternative to `--file`, not a fallback: passing both is an error, and
hobnob never reaches for it on its own. It combines with every other flag.

### `--list`

Prints every public task with its `info:`. Tasks prefixed with `_`, and tasks
from `_`-prefixed modules, are omitted.

```bash
hobnob --list
```

### `--select`

Opens the interactive picker even when a `default` task exists.

```bash
hobnob --select
```

With no terminal, under CI, or with `--no-input`, there is nothing to pick
with, so it prints the task list instead.

### `--help`

Prints usage, the docs links, and the task list for the current taskfile.

### `--version`

Prints the version and exits. Answered before any taskfile is looked for, so it
works from anywhere.

### `--upgrade`

Replaces the running binary with the latest release. Also answered before any
taskfile is looked for.

### `completion <shell>`

Prints the completion script for `bash`, `zsh` or `fish`. The installer wires
this up for you; run it by hand only to re-generate one.

```bash
hobnob completion zsh > ~/.zsh/completions/_hobnob
```

### Stopping a run

CTRL+C signals the running command and waits for it to exit; no further steps
start. A second CTRL+C kills it immediately.

## File structure

Five optional top-level keys:

```yaml
const: # fixed values, nothing outside the file can override
  - API: https://official-joke-api.appspot.com

vars: # overridable defaults
  - TYPE: general

env: # files to source
  - .env

modules: # imported hobnob files
  - report: ./reporting.yml

tasks:
  tell-joke:
    steps:
      - run: curl -s {{.API}}/jokes/{{.TYPE}}/random
```

An unrecognized top-level key is a load error, not a silent no-op, so `taks:`
fails loudly instead of running with no tasks.

## Tasks

| Key      | Meaning                                                 |
| -------- | ------------------------------------------------------- |
| `info:`  | One-line description, shown in `--list` and the picker. |
| `steps:` | The sequence to run.                                    |
| `if:`    | Shell condition; non-zero skips the whole task.         |
| `dir:`   | Working directory for the task's `run:` steps.          |
| `once:`  | Memoize: run at most once per invocation.               |

A `_` prefix on a task name (`_fetch-joke`) makes it internal: it runs, but is
hidden from `--list` and the picker.

### `if:`

```yaml
tasks:
  explain-joke:
    if: '[ "{{.TYPE}}" = "programming" ]'
    steps:
      - run: echo "it is funny because it is true"
```

Exit 0 proceeds, non-zero skips. A skipped `call:` target is not an error;
execution simply continues in the caller.

### `dir:`

All relative paths resolve against the hobnob file's directory. With no `dir:`
set anywhere, steps run there.

```yaml
tasks:
  archive-joke:
    dir: ./jokes
    steps:
      - run: curl -s -o {{.ID}}.json {{.API}}/jokes/{{.ID}} # writes into ./jokes
      - run: ./lint-punchline.sh
        dir: ../scripts # this step only
      - call: _index # inherits ./jokes
      - call: _index
        dir: ./archive # overrides _index's own dir:
```

Precedence: call-step `dir:` > task `dir:` > inherited parent `dir:`. A run
step's `dir:` applies to that step alone. An `if:` always evaluates in the
inherited task `dir:`, even on a step that overrides it.

### `once:` (memoized tasks)

`once: true` makes a task run at most once per `hobnob` invocation. Every later
`call:` replays the first run's result, and each call site still pulls what it
wants through its own `into:`.

```yaml
tasks:
  _types:
    once: true
    steps:
      - run: curl -s {{.API}}/types
        into:
          - TYPES: stdout
      - get:
          - TYPE:
              info: Which kind of joke?
              options: .TYPES

  tell-joke:
    steps:
      - call: _types
        into: [{ TYPE: .TYPE }]
      - run: curl -s {{.API}}/jokes/{{.TYPE}}/random

  count-jokes:
    steps:
      - call: _types # cached: no second prompt, no second request
        into: [{ TYPES: .TYPES }, { TYPE: .TYPE }]
      - run: echo "{{ .TYPES | len }} types, you picked {{.TYPE}}"
```

- **A hit is announced**, never silent:
  `call: [count-jokes] _types (cached — TYPE=dad …)`.
- **The memo is the whole scope** the first run produced, not a "done" flag, so
  it replays across `call:`'s sandboxing. Two sibling calls reaching the same
  task both get its results.
- **A task skipped by its own `if:`** caches as having produced nothing. A
  failure under `soft: true` is not cached, and is retried by the next call.
- **Replay overwrites.** If a caller sets `TYPE` between two calls, the second
  call's `into:` restores the cached value. `once:` asserts a task's results
  are in scope; it is not "maybe run something". Only names that call site's
  `into:` asks for are touched.
- **`once:` is a property of the task**, not something a call site opts into.

## Steps

Five kinds. Any step takes `if:` to skip itself:

```yaml
- run: echo "nerd humour incoming"
  if: '[ "{{.TYPE}}" = "programming" ]'
```

### `set`: assign variables

```yaml
- set:
    - API: https://official-joke-api.appspot.com
    - RANDOM_URL: "{{.API}}/random_joke" # sees the line above it
    - LABELS: { dad: Dad jokes, programming: Nerd jokes }
    - WEBHOOK:
        value: .SLACK_URL
        secret: true # masked in terminal output
```

Entries resolve top to bottom. A YAML map or list literal becomes a real object
or array (see [Types](#types)).

### `run`: shell commands

```yaml
- run: curl -s {{.API}}/random_joke
  dir: ./jokes
  into:
    - JOKE: stdout
    - CURL_ERRORS: stderr
```

| Key      | Meaning                                                       |
| -------- | ------------------------------------------------------------- |
| `into:`  | Capture `stdout`, `stderr` or `exit` into variables.          |
| `dir:`   | Working directory, this step only.                            |
| `soft:`  | A non-zero exit continues the timeline instead of halting it. |
| `quiet:` | Hide output on success.                                       |
| `if:`    | Skip this step.                                               |

**`into:` sources.** `stdout` and `stderr` capture text, parsed as a value when
it is valid JSON; `exit` captures the exit code as a number. Each takes a
trailing [accessor](#accessors) and any [filter](#filters) chain:

```yaml
- run: curl -s {{.API}}/jokes/random/5
  into:
    - JOKES: stdout # the whole array
    - FIRST: stdout[0].setup # one field out of it
    - SETUPS: stdout[*].setup # every setup, as an array
```

Captures happen whether the command succeeded or failed, which lets a `soft:`
step report what went wrong:

```yaml
- run: curl -sf {{.API}}/jokes/999999
  soft: true
  into:
    - CODE: exit
    - ERRORS: stderr
- run: echo "no joke with that id (curl exited {{.CODE}})"
  if: "{{ ne .CODE 0 }}"
```

Only a command that never started (binary not found) captures nothing; a step
killed by a signal reports exit `-1`.

An `into:` entry can assemble a literal from several pieces at once:

```yaml
- run: curl -s {{.API}}/random_joke
  into:
    - CARD:
        text: stdout.setup
        id: stdout.id
    # CARD = {"text":"What do you call…","id":451}, id still a real number
```

**`quiet:`** replaces a command's output with a one-line message:

```yaml
- run: curl -o jokes.json {{.API}}/jokes/ten
  quiet: Downloading ten jokes
- run: ./import.sh
  quiet: true # hide output, no custom message
```

```
⊙ [seed] Downloading ten jokes… (output hidden)
```

Output is hidden only on success: a failing quiet step replays its full stdout
and stderr before the error propagates. `into:` still captures either way.
`quiet:` is valid on `run:` only.

#### Argv list form

A YAML sequence executes directly, one element per argument, with no shell:

```yaml
- run: [curl, -s, "{{.API}}/jokes/{{.TYPE}}/random"]
```

Each element is one argument whatever it contains, so no value is ever re-split
by a shell. Use the list form whenever an argument holds a variable.

An element is a whole field value: a bare `.VAR` or a filter chain works, and
one resolving to an array splices into several arguments:

```yaml
- set:
    - CURL_OPTS: ["-s", "--max-time", "10"]
- run: [curl, .CURL_OPTS, "{{.API}}/random_joke"]
# argv: curl -s --max-time 10 https://official-joke-api.appspot.com/random_joke
```

How elements resolve:

- **Array** splices into several arguments; an empty one splices to nothing.
- **`""`** stays an empty argument. Dropping it would shift later positions.
- **Object** is an error. Use an accessor to pick the field you meant.

The list form gives up pipes, redirects, globs, `&&` and shell builtins. `cd`
in particular is gone, which is what `dir:` is for. The string form stays for
all of it, and neither is deprecated.

In the string form, escape interpolated values with [`quote`](#filters). A YAML
block scalar drops YAML's own quoting layer, so nested `"` need no escaping:

```yaml
- run: |
    echo "{{ .JOKE[0].setup }} ... {{ .JOKE[0].punchline }}"
```

> **Unbuffered Python.** Scripts buffer stdout when not attached to a terminal,
> so output can appear late or all at once. Fix with
> `- set: [{PYTHONUNBUFFERED: 1}]`, or `python -u` per script.

### `get`: interactive prompts

Prompts for a variable, and is skipped when that variable is already in scope.

```yaml
- get: [TYPE] # bare form
- get:
    - COUNT:
        info: How many jokes?
        default: 3
        check: "[ {{.COUNT}} -le 10 ]"
    - TYPES:
        options: .ALL_TYPES
        multi: true
```

| Key         | Meaning                                                           |
| ----------- | ----------------------------------------------------------------- |
| `info:`     | Prompt text.                                                      |
| `default:`  | Pre-filled value, used as-is when prompts are off.                |
| `options:`  | Turns it into a select. A list literal or a variable holding one. |
| `multi:`    | Multi-select; the result is an array.                             |
| `check:`    | Shell condition the answer must satisfy (exit 0).                 |
| `secret:`   | Mask in terminal output.                                          |
| `optional:` | Skip silently if unanswered, leaving the variable empty.          |

With `--no-input`, a `CI` env var, or non-terminal stdin (AI agents, scripts),
prompts are skipped. A missing variable with no `default:` aborts the run.

### `call`: sub-tasks

Runs another task in a deep copy of scope. Nothing mutated in the child reaches
the caller except through `into:`.

```yaml
- call: _fetch-joke
  dir: ./jokes
  with:
    - TYPE: programming
    - TIMEOUT_SECS: 10
  into:
    - SETUP: .RESPONSE[0].setup
    - PUNCHLINE: .RESPONSE[0].punchline
```

An `into:` entry can be a map or list literal, built from several of the child's
values in one shot:

```yaml
- call: _fetch-joke
  into:
    - CARD:
        setup: .RESPONSE[0].setup
        punchline: .RESPONSE[0].punchline
```

`soft: true` continues past a failed call, same as on `run:`.

`with:` entries take no `secret:` flag; it is rejected at parse time. Masking
matches on value, so a secret stays masked once passed down even under a new
name. Mark it secret where it is defined.

### `loop`: iteration

**List form.** Iterates an array, current element as `{{.ITEM}}`:

```yaml
- run: curl -s {{.API}}/types
  into:
    - TYPES: stdout
- loop: .TYPES
  steps:
    - run: curl -s {{.API}}/jokes/{{.ITEM}}/random
```

A plain string source runs the body once with `ITEM` set to the whole string. It
is not split or parsed; see [Types](#types).

**Map form.** When the variable is an object, iterates its entries in sorted-key
order as `{{.KEY}}` and `{{.VALUE}}`:

```yaml
- set:
    - LABELS: { dad: Dad jokes, programming: Nerd jokes }
- loop: .LABELS
  steps:
    - run: echo "{{.KEY}} = {{.VALUE}}"
```

**Matrix form.** Every combination of the given arrays:

```yaml
- loop:
    TYPE: [general, programming]
    COUNT: [1, 3]
  steps:
    - run: curl -s {{.API}}/jokes/random/{{.COUNT}}
```

## Variables

Variables are evaluated at runtime with Go templates (`{{ .VAR }}`), never at
parse time.

### Precedence

```
env  <  vars:  <  env files  <  CLI args  <  const:  <  timeline (set / get / loop / call)
```

- **Env is lowest**, so ambient shell state cannot silently change behavior
  between machines.
- **`const:` outranks even CLI args.** That is what makes it a constant rather
  than a default.
- **Above `const:` there is no ranking**, only execution order. A task's own
  steps run after scope is built, each seeing everything before it.

### `const:` and `vars:`

Two top-level blocks of file-scoped variables, evaluated once at load, before
env files, CLI args and any task. Each entry takes the same shape `set:` does,
including `{ value:, secret: }` and map/list literals, resolved top to bottom.

```yaml
const:
  - API: https://official-joke-api.appspot.com
  - LABELS: { dad: Dad jokes, programming: Nerd jokes }

vars:
  - TYPE: general
  - WEBHOOK:
      value: .SLACK_URL
      secret: true
```

Two load-time rules keep them honest:

**`const:` is a closed world.** An entry may reference only earlier `const:`
entries and the two [built-ins](#built-in-variables). Otherwise a constant could
quietly read a lower-priority layer and still call itself fixed.

```yaml
const:
  - API: https://official-joke-api.appspot.com
  - TYPES_URL: "{{.API}}/types" # ok, earlier entry
  - TYPE: '{{ .TYPE | default "dad" }}' # error, not a file constant
```

**A `const:` name is reserved file-wide.** No task's `set:`/`get:`/`into:`/
`loop:` may write to it, or `const:` would only be constant from outside the
file, the timeline outranking it.

**`vars:` may not reference its own key.** It already is the fallback layer, so
`- TYPE: '{{ .TYPE | default "general" }}'` is an error; write
`- TYPE: general`.

### Env files

`env:` lists files to source, resolved relative to the hobnob file:

```yaml
env:
  - .env:
      secret: true # mask this file's variables
  - ./credentials.sh
  - defaults.txt
```

- Anything not ending in `.sh` is parsed as `KEY=VALUE` lines, allowing blank
  lines, `#` comments, and an optional `export` prefix.
- `.sh` files are sourced in a subshell. Only variables the script newly sets or
  changes are pulled in.
- A missing file warns and is skipped rather than failing the run.
- Nothing is masked by default, whatever the filename. Opt in with
  `secret: true`.
- Later entries override earlier ones; masking follows whichever value won.

### Built-in variables

- `HOBNOB_FILE_DIR`: directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR`: directory hobnob was run from.

## Types

A variable holds a string, number, bool, array or object. Structure enters
scope from exactly three places:

1. a `set:`/`with:`/`into:`/`const:`/`vars:` map or list literal
2. `run:` output captured via `into:`, when it decodes cleanly as a JSON array
   or object
3. the explicit `json` filter

Everything else is text, however JSON-shaped it looks. Environment variables,
CLI `KEY=VALUE` args and env file values are never sniffed; parse them
explicitly:

```bash
hobnob report TYPES='["dad","programming"]'
```

```yaml
- loop: .TYPES | json
```

Structure is sniffed once, at capture, and never re-attempted downstream. A
string is never silently re-read as an array by a later accessor or filter,
which is why `keys` and accessors error on a string instead of parsing it.

**Keeping a type.** A field that is exactly one variable reference keeps that
variable's type: a bare `.VAR`, an accessor chain, or a single `{{ }}` action,
optionally through a filter chain. Any surrounding text renders it to a string:

```yaml
- set:
    - A: .TYPES # still an array
    - B: "{{ .TYPES }}" # text (JSON text, here)
    - C: "count: {{ .TYPES | len }}" # text, surrounding text forces it
```

This is what makes `options: .TYPES` and `- run: [curl, .CURL_OPTS, .URL]`
work.

## Accessors

Query a real array or object with the syntax the value itself resembles:

```yaml
- run: curl -s {{.API}}/random_joke
  into:
    - JOKE: stdout
- set:
    - SETUP: .JOKE.setup
```

| Form                    | Meaning                                                         |
| ----------------------- | --------------------------------------------------------------- |
| `.A.b.c`                | object keys                                                     |
| `.A[0]`                 | array index                                                     |
| `.A[-1]`                | negative index, from the end                                    |
| `.A[1:3]`               | slice (bounds clamp, like Go: `[0:99]` on 3 elements returns 3) |
| `.A[*]`                 | every element of an array, or every value of an object          |
| `.A["key with . or /"]` | a literal key that is not a valid identifier                    |
| `.A[.KEY]`              | dynamic key or index, taken from a variable                     |
| `.A[.KEY][0].name`      | any combination, any depth                                      |
| `(.TYPES \| json)[0]`   | a pipeline result as the head                                   |

Dynamic keys are what a quoted path string cannot do. `{{ .LABELS[.TYPE] }}` is
a plain reference, not string concatenation, so a key containing `.` or `[`
(`knock-knock.v2`) is looked up literally rather than mis-parsed as a path.

**Multiplicity.** A slice or `[*]` yields many nodes, and a step after one maps
over them, producing an array:

```yaml
- run: curl -s {{.API}}/jokes/random/5
  into:
    - JOKES: stdout
- set:
    - SETUPS: .JOKES[*].setup # ["Did you hear the news?", …]
    - FIRST_TWO: .JOKES[0:2].id # [99, 32]
```

- Elements with no match, or of the wrong kind, are **dropped**, not kept as
  nil and not an error. Same convention as `lines` and `split`.
- A slice or `[*]` matching nothing yields an **empty array**. Multiplicity is
  not absence.
- `[*]` on an object yields values in **sorted-key order**, the order `keys`
  uses.

**Absence is an error, caught by `default`.** A missing key, an out-of-range
index, or a value that is not an array or object errors:

```yaml
{{ .JOKE.author }}                      # error: path not found
{{ .JOKE.author | default "anonymous" }}  # "anonymous"
```

`default` catches absence only. A wrong-kind access, indexing a string or
slicing an object, is never caught by it, even with `| default` right after:
that is a taskfile bug with one fix (`| json`), not a fact about the data.

Strictness applies everywhere a chain lands, argv elements included, so
`- run: [curl, -s, .JOKE.pucnhline]` aborts on the typo instead of passing an
empty argument.

## Filters

Usable anywhere templates are, and in `into:` pipes. Chain with `|`.

| Filter            | Does                                                       |
| ----------------- | ---------------------------------------------------------- |
| `default "x"`     | Falls back when the value is empty or a missing path.      |
| `trim`            | Strips leading and trailing whitespace.                    |
| `upper` / `lower` | Changes case.                                              |
| `split ","`       | Splits a string into a list, dropping empty parts.         |
| `lines`           | Splits on newlines, trimming each and dropping blank ones. |
| `json`            | Parses a string into a real array or object.               |
| `string`          | Forces a value to text; compact JSON for a list or object. |
| `keys`            | Sorted list of an object's top-level keys.                 |
| `len`             | Length of a string (in runes), array, or object.           |
| `quote`           | Wraps in POSIX single quotes for a `run:` string command.  |
| `jsonEscape`      | Escapes a string for embedding inside JSON text.           |

```yaml
- set:
    - TYPE: '{{ .JOKE_TYPE | default "general" }}'
- run: curl -s {{.API}}/jokes/ten | jq -r '.[].setup'
  into:
    - SETUPS: stdout | lines
- run: echo {{ .JOKE.punchline | quote }}
```

- **`json`** is identity on something already structured, so it is always safe
  to add defensively before an accessor or `keys`.
- **`keys`** errors on anything that is not a real object, naming `| json`. For
  values instead of keys, use `[*]`. On one joke object, `.JOKE | keys` gives
  `["id","punchline","setup","type"]` and `.JOKE[*]` gives
  `[1,"Dam.","What did the fish say…","general"]`, the same order.
- **`quote`** is the escape hatch for the string `run:` form, which the
  [argv list form](#argv-list-form) mostly removes the need for.

For more than one filter, name the intermediate value instead of stacking pipes
inline:

```yaml
- set:
    - TYPES: "{{ .TYPES_TEXT | json }}"
- run: [curl, -s, "{{.API}}/jokes/{{ .TYPES[0] }}/random"]
```

### Comparisons

`eq` / `ne` / `lt` / `le` / `gt` / `ge` compare typed values. Handy in `if:`,
which runs its rendered `true`/`false` as a shell condition:

```yaml
- run: echo "nerd humour incoming"
  if: '{{ eq .TYPE "programming" }}'
- run: echo "that is a lot of jokes"
  if: "{{ gt (.JOKES | len) 5 }}"
```

- **JSON number text on both sides** (`-3`, `1.5`, `1e9`), whatever kind
  carries it, compares numerically and exactly, never through a lossy float.
  So `{{ lt .A .B }}` with CLI args `A=9` and `B=10` is `true`, not what a
  lexical `"10" < "9"` would give.
- **Text that only looks numeric to a human** (`Inf`, `NaN`, `0x10`, `007`,
  `+3`) compares as text.
- **A missing variable** equals `""`.
- **Anything else** is exact text equality and lexical ordering.

Bools support `eq`/`ne` only; arrays and objects are not comparable at all, so
access the field you meant. `eq` takes several right-hand arguments and is true
if any match: `eq .TYPE "dad" "knock-knock"`.

## Modules

Import tasks from other files.

```yaml
modules:
  - jokes: ./jokes.yml # short form: key and path
  - _drafts: ./drafts.yml # `_` key marks the module internal
  - report:
      path: ./reporting.yml
      show: [daily, weekly] # or hide: [scratch]
      flatten: true
```

| Key        | Meaning                                              |
| ---------- | ---------------------------------------------------- |
| `path:`    | File to import, relative to the importing taskfile.  |
| `show:`    | Whitelist. Only these task names are imported.       |
| `hide:`    | Blacklist. Everything else is imported.              |
| `flatten:` | Also register tasks under their bare name.           |

- **Namespaces.** Imported tasks are prefixed with their module key, so
  `report:daily`. A `_` prefix on the key makes the module internal: hidden
  from `--list` and from parent files.
- **Flattening.** With `flatten: true`, `call: daily` reaches it too. Native
  tasks win conflicts.
- **Scoping.** A module's `env:`/`const:`/`vars:` apply to its own subtree and
  never leak to the parent. Its tasks are checked against its own `const:`, not
  the parent's.

Inside that subtree the two kinds of block differ:

- **`const:` always wins**, even over a parent `const:` of the same name or a
  CLI arg. Nearest declaration wins, like any lexical scope.
- **`env:`/`vars:` only fill a gap** the caller has not set, acting as the
  module's own lowest layer rather than an override.
