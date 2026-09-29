# hobnob reference

Every YAML field, template filter and CLI flag. New to hobnob? Start with the
[guide](GUIDE.md).

---

## CLI

```
hobnob [--file <path> | --demo] [<task>] [KEY=VALUE ...] [--no-input]
hobnob [--file <path> | --demo] (--list | --select | --help)
hobnob (--version | --upgrade | completion <shell>)
```

### `<task>`

The task to run, always the first argument. Without one, hobnob runs the task
named `default`, or opens the picker if there is none.

```bash
hobnob tell-joke
```

A `_`-prefixed task is internal: only `call:` can reach it.

### `KEY=VALUE`

Sets a variable for the run, above `vars:` and env files, below `const:` (see
[Precedence](#precedence)). Repeatable. It also answers a `get:` for the same
name, which is what makes an interactive task scriptable. Values are always
text; see [Types](#types).

```bash
hobnob tell-joke TYPE=programming COUNT=3
```

### `--no-input`

Skips every prompt: a `get:` uses its `default:`, or aborts the run without
one. Implied when `CI` is set or stdin is not a terminal, so scripts and AI
agents get it for free.

```bash
hobnob tell-joke --no-input
```

### `--file <path>`

Uses a specific taskfile.

```bash
hobnob --file ops/jokes.yml tell-joke
```

Without it, hobnob searches upward from the current directory for
`hobnob.yml`, then `hobnob.yaml`. Relative paths inside a file resolve against
that file's directory, not where you ran from.

### `--demo`

Runs a small built-in taskfile. It replaces `--file` (passing both is an
error) and combines with every other flag.

```bash
hobnob --demo tell-joke
hobnob --demo --list
```

### `--list`

Prints every public task with its `info:`. Internal tasks and modules are
omitted.

### `--select`

Opens the picker even when a `default` task exists. With no terminal, under
CI, or with `--no-input`, it prints the task list instead.

### `--help`

Prints usage, the docs links, and the task list.

### `--version`

Prints the version. Works outside any taskfile.

### `--upgrade`

Replaces the running binary with the latest release. Works outside any
taskfile.

### `completion <shell>`

Prints the completion script for `bash`, `zsh` or `fish`. The installer wires
it up; run it yourself only to regenerate.

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

Any other top-level key is a load error, so a typo like `taks:` fails loudly.

## Tasks

| Key      | Meaning                                                 |
| -------- | ------------------------------------------------------- |
| `info:`  | One-line description, shown in `--list` and the picker. |
| `steps:` | The sequence to run.                                    |
| `if:`    | Shell condition; non-zero skips the whole task.         |
| `dir:`   | Working directory for the task's `run:` steps.          |
| `once:`  | Memoize: run at most once per invocation.               |

A `_` prefix (`_fetch-joke`) makes a task internal: it runs, but is hidden
from `--list`, the picker and the command line.

### `if:`

```yaml
tasks:
  explain-joke:
    if: '[ "{{.TYPE}}" = "programming" ]'
    steps:
      - run: echo "it is funny because it is true"
```

Exit 0 proceeds, non-zero skips. A skipped `call:` target is not an error, but
it sets nothing, so an `into:` entry pulling from it needs `| default`:

```yaml
- call: explain-joke
  into:
    - EXPLAINED: .EXPLAINED | default "no"
```

### `dir:`

Relative paths resolve against the hobnob file's directory, and steps run
there by default.

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

Call-step `dir:` beats task `dir:`, which beats the inherited one. A `run:`
step's `dir:` applies to that step alone. `if:` always evaluates in the task's
`dir:`, even on a step that overrides it.

### `once:` (memoized tasks)

`once: true` runs a task at most once per invocation. Later `call:`s replay
the first run's result, each pulling what it wants through its own `into:`.

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

- **A hit is announced:** `call: [count-jokes] _types (cached — TYPE=dad …)`.
- **The memo is the whole scope** the first run produced, so sibling calls in
  separate sandboxes all get its results.
- **A task skipped by its own `if:`** caches as producing nothing; use
  `| default` in `into:`. A failure under `soft: true` is not cached, so the
  next call retries.
- **Replay overwrites.** If a caller changes `TYPE` between calls, the second
  call's `into:` restores the cached value. Only names that `into:` asks for
  are touched.
- **`once:` belongs to the task**, not the call site.

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
it is valid JSON; `exit` captures the exit code as a number. Each takes an
[accessor](#accessors) and [filter](#filters) chain:

```yaml
- run: curl -s {{.API}}/jokes/random/5
  into:
    - JOKES: stdout # the whole array
    - FIRST: stdout[0].setup # one field out of it
    - SETUPS: stdout[*].setup # every setup, as an array
```

A dynamic key (`stdout[.KEY]`) and a `{{ }}` entry read the caller's scope,
including entries above it:

```yaml
- run: git rev-parse --short HEAD
  into:
    - SHA: stdout | trim
    - TAG: "build-{{.SHA}}"
```

Captures happen on failure too, so a `soft:` step can report what went wrong:

```yaml
- run: curl -sf {{.API}}/jokes/999999
  soft: true
  into:
    - CODE: exit
    - ERRORS: stderr
- run: echo "no joke with that id (curl exited {{.CODE}})"
  if: "{{ ne .CODE 0 }}"
```

Only a command that never started (binary not found) captures nothing. A
signal kill reports exit `-1`.

An entry can assemble a literal from several pieces:

```yaml
- run: curl -s {{.API}}/random_joke
  into:
    - CARD:
        text: stdout.setup
        id: stdout.id
    # CARD = {"text":"What do you call…","id":451}, id still a real number
```

**`quiet:`** (`run:` only) replaces the output with a one-line message:

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
and stderr first. `into:` captures either way.

#### Argv list form

A YAML sequence executes directly, one element per argument, with no shell, so
no value is ever re-split. Use it whenever an argument holds a variable:

```yaml
- run: [curl, -s, "{{.API}}/jokes/{{.TYPE}}/random"]
```

Each element is a whole field value, so a bare `.VAR` or a filter chain works:

```yaml
- set:
    - CURL_OPTS: ["-s", "--max-time", "10"]
- run: [curl, .CURL_OPTS, "{{.API}}/random_joke"]
# argv: curl -s --max-time 10 https://official-joke-api.appspot.com/random_joke
```

- **Array** splices into several arguments; an empty one splices to nothing.
- **`""`** stays an empty argument, so later positions never shift.
- **Object** is an error. Use an accessor to pick the field you meant.

The list form gives up pipes, redirects, globs, `&&` and builtins like `cd`
(use `dir:`). The string form keeps all of them; neither is deprecated.

In the string form, escape interpolated values with [`quote`](#filters). A
YAML block scalar spares you YAML's own quoting:

```yaml
- run: |
    echo "{{ .JOKE[0].setup }} ... {{ .JOKE[0].punchline }}"
```

> **Unbuffered Python.** Python buffers stdout when not on a terminal, so
> output can arrive late or all at once. Fix with
> `- set: [{PYTHONUNBUFFERED: 1}]`, or `python -u` per script.

### `get`: interactive prompts

Prompts for a variable, skipped when it is already in scope.

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

When prompts are off (see [`--no-input`](#--no-input)), a missing variable
with no `default:` aborts the run.

### `call`: sub-tasks

Runs another task in a deep copy of scope. Nothing the child changes reaches
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

An `into:` entry names one of the child's variables (leading `.` optional),
then any accessor and filter chain, exactly like `stdout` on `run:`. Dynamic
keys and `{{ }}` entries read the caller's scope. A name the child never set
is an error that `| default` catches:

```yaml
- call: _fetch-joke
  into:
    - AUTHOR: .AUTHOR | default "anonymous"
```

An entry can also be a literal built from several child values:

```yaml
- call: _fetch-joke
  into:
    - CARD:
        setup: .RESPONSE[0].setup
        punchline: .RESPONSE[0].punchline
```

`soft: true` continues past a failed call, as on `run:`.

**Secrets cross calls both ways.** Masking matches on value, so a secret stays
masked when `with:` passes it down under a new name, and when `into:` pulls a
child's secret back up in any shape. Mark it `secret:` where it is defined;
`with:` rejects the flag.

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

A plain string runs the body once, with `ITEM` as the whole string, unsplit
(see [Types](#types)).

**Map form.** An object iterates in sorted-key order as `{{.KEY}}` and
`{{.VALUE}}`:

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

Variables are Go templates (`{{ .VAR }}`), evaluated at runtime, never at
parse time.

### Precedence

```
env  <  vars:  <  env files  <  CLI args  <  const:  <  timeline (set / get / loop / call)
```

- **Env is lowest**, so ambient shell state cannot change behaviour between
  machines.
- **`const:` beats even CLI args.** That is what makes it a constant.
- **Above `const:`, only execution order counts.** Each step sees everything
  before it.

**Upward reads.** A layer's templates can read the _final_ value of any name
set by a layer above it, never below it and never its own. So layers resolve
in reverse: `const:`, CLI args, `env:` files, then `vars:`. Every layer but
`const:` can also read the OS env. See
[ADR-0001](docs/adr/0001-upward-reads-between-scope-layers.md).

### `const:` and `vars:`

File-scoped variables, resolved once at load. Entries take the same shape as
`set:` (including `{ value:, secret: }` and literals) and resolve top to
bottom within their block.

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

Load-time rules:

**`const:` is a closed world.** An entry may reference only earlier `const:`
entries and the [built-ins](#built-in-variables), so it cannot read a lower
layer and still call itself fixed.

```yaml
const:
  - API: https://official-joke-api.appspot.com
  - TYPES_URL: "{{.API}}/types" # ok, earlier entry
  - TYPE: '{{ .TYPE | default "dad" }}' # error, not a file constant
```

**A `const:` name is reserved file-wide.** No `set:`/`get:`/`into:`/`loop:`
may write to it.

**`vars:` may not reference its own key.** It is already the fallback, so
`- TYPE: '{{ .TYPE | default "general" }}'` is an error; write
`- TYPE: general`.

**`vars:` fills gaps only.** An entry can build its default from `env:` files,
CLI args and `const:`:

```yaml
env:
  - .env # API_HOST=staging.example.com

vars:
  - API: "https://{{.API_HOST}}/v1"
```

If a higher layer already set the name, the entry is skipped without being
evaluated. A name set only by the OS env is still a gap.

### Env files

`env:` lists files to source, relative to the hobnob file:

```yaml
env:
  - .env:
      secret: true # mask this file's variables
  - ./credentials.sh
  - defaults.txt
```

- Anything not ending in `.sh` is `KEY=VALUE` lines, with blank lines, `#`
  comments and an optional `export` prefix allowed.
- `.sh` files are sourced in a subshell against the OS env; only variables
  the script sets or changes come in.
- A missing file warns and is skipped.
- Nothing is masked unless the entry sets `secret: true`.
- Later entries override earlier ones; masking follows the winning value.
- **A path can read CLI args, `const:`, the OS env and the built-ins, but not
  `vars:`**, which reads `env:` files itself. Referencing a `vars:` name is a
  load error:

  ```yaml
  vars:
    - STAGE: dev
  env:
    - .env.{{.STAGE}} # error: env: path can't reference a vars: name
  ```

  Pass `STAGE` on the CLI, set it in the OS env, or make it a `const:`.
  (Older versions resolved `vars:` first and allowed this.)

### Built-in variables

- `HOBNOB_FILE_DIR`: directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR`: directory hobnob was run from.

## Types

A variable holds a string, number, bool, array or object. Structure comes from
exactly three places:

1. a `set:`/`with:`/`into:`/`const:`/`vars:` map or list literal
2. `run:` output captured via `into:` that decodes cleanly as a JSON array or
   object
3. the `json` filter

Everything else is text, however JSON-shaped: env vars, CLI args and env file
values included. Parse them explicitly:

```bash
hobnob report TYPES='["dad","programming"]'
```

```yaml
- loop: .TYPES | json
```

Structure is detected once, at capture. A later accessor or filter never
re-reads a string as an array, which is why `keys` and accessors error on a
string.

**Keeping a type.** A field that is exactly one variable reference (a bare
`.VAR`, an accessor chain, or a single `{{ }}` action, with optional filters)
keeps its type. Any surrounding text makes it a string:

```yaml
- set:
    - A: .TYPES # still an array
    - B: "{{ .TYPES }}" # text (JSON text, here)
    - C: "count: {{ .TYPES | len }}" # text, surrounding text forces it
```

This is what makes `options: .TYPES` and `- run: [curl, .CURL_OPTS, .URL]`
work.

## Accessors

Query an array or object with the syntax the value resembles:

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

`{{ .LABELS[.TYPE] }}` is a lookup, not string concatenation, so a key holding
`.` or `[` (`knock-knock.v2`) is matched literally.

**Multiplicity.** After a slice or `[*]`, later steps map over every node,
producing an array:

```yaml
- run: curl -s {{.API}}/jokes/random/5
  into:
    - JOKES: stdout
- set:
    - SETUPS: .JOKES[*].setup # ["Did you hear the news?", …]
    - FIRST_TWO: .JOKES[0:2].id # [99, 32]
```

- Nodes with no match, or of the wrong kind, are **dropped**, as with `lines`
  and `split`.
- Matching nothing yields an **empty array**, not absence.
- `[*]` on an object yields values in **sorted-key order**, matching `keys`.

**Absence is an error, caught by `default`.** A missing key, an out-of-range
index, or indexing into something that is not an array or object errors:

```yaml
{{ .JOKE.author }}                      # error: path not found
{{ .JOKE.author | default "anonymous" }}  # "anonymous"
```

`default` catches absence only. A wrong-kind access (indexing a string,
slicing an object) is a taskfile bug fixed with `| json`, and `default` never
catches it.

This strictness applies wherever a chain lands, argv included:
`- run: [curl, -s, .JOKE.pucnhline]` aborts on the typo rather than pass an
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

- **`json`** leaves structured values alone, so it is always safe before an
  accessor or `keys`.
- **`keys`** errors on anything but an object, naming `| json`. For values,
  use `[*]`: on one joke, `.JOKE | keys` gives
  `["id","punchline","setup","type"]` and `.JOKE[*]` gives
  `[1,"Dam.","What did the fish say…","general"]`, same order.
- **`quote`** escapes for the string `run:` form; the
  [argv list form](#argv-list-form) usually removes the need.

Name intermediate values rather than stacking many pipes inline:

```yaml
- set:
    - TYPES: "{{ .TYPES_TEXT | json }}"
- run: [curl, -s, "{{.API}}/jokes/{{ .TYPES[0] }}/random"]
```

### Comparisons

`eq` / `ne` / `lt` / `le` / `gt` / `ge` compare typed values. `if:` runs the
rendered `true`/`false` as a shell condition:

```yaml
- run: echo "nerd humour incoming"
  if: '{{ eq .TYPE "programming" }}'
- run: echo "that is a lot of jokes"
  if: "{{ gt (.JOKES | len) 5 }}"
```

- **JSON number text on both sides** (`-3`, `1.5`, `1e9`), whatever its kind,
  compares numerically and exactly. So with CLI args `A=9 B=10`,
  `{{ lt .A .B }}` is `true`.
- **Text that only looks numeric** (`Inf`, `NaN`, `0x10`, `007`, `+3`)
  compares as text.
- **A missing variable** equals `""`.
- **Anything else** compares as text, exactly and lexically.

Bools support only `eq`/`ne`; arrays and objects are not comparable, so access
the field you meant. `eq` is true if any right-hand argument matches:
`eq .TYPE "dad" "knock-knock"`.

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

| Key        | Meaning                                             |
| ---------- | --------------------------------------------------- |
| `path:`    | File to import, relative to the importing taskfile. |
| `show:`    | Whitelist. Only these task names are imported.      |
| `hide:`    | Blacklist. Everything else is imported.             |
| `flatten:` | Also register tasks under their bare name.          |

- **Namespaces.** Imported tasks are prefixed with the module key:
  `report:daily`. A `_` key makes the module internal, hidden from `--list`
  and parent files.
- **Flattening.** With `flatten: true`, `call: daily` works too. Native tasks
  win conflicts.
- **Scoping.** A module's `env:`/`const:`/`vars:` apply only to its own tasks,
  never the parent's, and its load-time rules check its own blocks.

Inside a module:

- **`const:` always wins**, even over a parent `const:` or a CLI arg of the
  same name. The nearest declaration wins.
- **`env:`/`vars:` only fill gaps**, as the module's lowest layer. A name only
  the OS env set is still a gap until a `set:`, `with:`, `into:`, `loop:` or
  answered `get:` writes it.

A module resolves its own blocks with the same [upward reads](#precedence) as
the root file, starting from its importer's final values, never a caller's
`with:` or `set:`. So with `STAGE=prod` on the CLI, a module whose `.env` sets
`STAGE=dev` still builds its `vars:` from `prod`: the CLI arg claimed `STAGE`
first.
