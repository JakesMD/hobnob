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
hobnob ask-trivia
```

A `_`-prefixed task is internal: only `call:` can reach it.

### `KEY=VALUE`

Sets a variable for the run, above `vars:` and env files, below `const:` (see
[Precedence](#precedence)). Repeatable. It also answers a `get:` for the same
name, which is what makes an interactive task scriptable. Values are always
text; see [Types](#types).

```bash
hobnob ask-trivia CATEGORY=science COUNT=3
```

### `--no-input`

Skips every prompt: a `get:` uses its `default:`, or aborts the run without
one. Implied when `CI` is set or stdin is not a terminal, so scripts and AI
agents get it for free.

```bash
hobnob ask-trivia --no-input
```

### `--file <path>`

Uses a specific taskfile.

```bash
hobnob --file ops/trivia.yml ask-trivia
```

Without it, hobnob searches upward from the current directory for
`hobnob.yml`, then `hobnob.yaml`. Relative paths inside a file resolve against
that file's directory, not where you ran from.

### `--demo`

Runs a small built-in taskfile. It replaces `--file` (passing both is an
error) and combines with every other flag.

```bash
hobnob --demo ask-trivia
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

Any other top-level key is a load error, so a typo like `taks:` fails loudly.

## Tasks

| Key        | Meaning                                                 |
| ---------- | ------------------------------------------------------- |
| `info:`    | One-line description, shown in `--list` and the picker. |
| `steps:`   | The sequence to run.                                    |
| `if:`      | Shell condition; non-zero skips the whole task.         |
| `dir:`     | Working directory for the task's `run:` steps.          |
| `once:`    | Memoize: run at most once per invocation.               |
| `results:` | Variables a `call:` without `into:` hands back.         |

A `_` prefix (`_fetch-question`) makes a task internal: it runs, but is hidden
from `--list`, the picker and the command line.

### `if:`

```yaml
tasks:
  explain-answer:
    if: '[ "{{.CATEGORY}}" = "science" ]'
    steps:
      - run: echo "here is the science behind it"
```

Exit 0 proceeds, non-zero skips. A skipped `call:` target is not an error, but
it sets nothing, so an `into:` entry pulling from it needs `| default`:

```yaml
- call: explain-answer
  into:
    - EXPLAINED: .EXPLAINED | default "no"
```

### `dir:`

Relative paths resolve against the hobnob file's directory, and steps run
there by default.

```yaml
tasks:
  archive-question:
    dir: ./questions
    steps:
      - run: curl -s -o {{.ID}}.json {{.API}}/question/{{.ID}} # writes into ./questions
      - run: ./lint-answers.sh
        dir: ../scripts # this step only
      - call: _index # inherits ./questions
      - call: _index
        dir: ./archive # overrides _index's own dir:
```

Call-step `dir:` beats task `dir:`, which beats the inherited one. A `run:`
step's `dir:` applies to that step alone. `if:` always evaluates in the task's
`dir:`, even on a step that overrides it.

### `results:` (what a call hands back)

`results:` lists the variables a task promises exist after any call to it. A
`call:` with no `into:` copies them into the caller:

```yaml
tasks:
  _pick-category:
    results: [CATEGORY]
    steps:
      - run: curl -s {{.API}}/metadata
        into:
          - _CATEGORIES: stdout.byCategory | keys # private scratch value
      - get:
          - CATEGORY:
              info: Which category?
              options: ._CATEGORIES

  ask-trivia:
    steps:
      - call: _pick-category # CATEGORY is now in scope
      - run: curl -s "{{.API}}/questions?limit=1&categories={{.CATEGORY}}"
```

- **A promise, however it's kept.** After the call, each result must be in the
  task's scope: set by a step, inherited from the caller, or kept through an
  `if:` skip. `hobnob ask-trivia CATEGORY=history` skips the prompt and still
  keeps it. A missing result fails the call, even when `into:` has a `| default`.
  Running the task directly, rather than through `call:`, checks nothing.
- **`into:` replaces results.** A call with its own `into:` takes exactly what
  `into:` lists, which can be any public variable; `into: []` takes nothing.
  Use it to rename, reshape, or avoid overwriting a variable the caller
  already has.
- **Plain names only.** No `_` names (they're private), no duplicates, no
  expressions; reshape at the call site with `into:`.
- **No `const:` names.** A call without `into:` can't pull a result named like
  one of the calling file's `const:` variables. For a task in the same file
  that fails at load; for a module or templated target, when the call runs.

### `once:` (memoized tasks)

`once: true` runs a task at most once per invocation. Later `call:`s replay
the first run's result, each pulling its results or what its own `into:` asks
for.

```yaml
tasks:
  _categories:
    once: true
    results: [CATEGORIES, CATEGORY]
    steps:
      - run: curl -s {{.API}}/metadata
        into:
          - CATEGORIES: stdout.byCategory | keys
      - get:
          - CATEGORY:
              info: Which category?
              options: .CATEGORIES

  ask-trivia:
    steps:
      - call: _categories # brings back CATEGORIES and CATEGORY
      - run: curl -s "{{.API}}/questions?limit=1&categories={{.CATEGORY}}"

  count-categories:
    steps:
      - call: _categories # cached: no second prompt, no second request
      - run: echo "{{ .CATEGORIES | len }} categories, you picked {{.CATEGORY}}"
```

- **A hit is announced** with what the first run set:
  `call: [count-categories] _categories (cached — CATEGORIES=[…] CATEGORY=history)`.
- **The memo is the whole scope** the first run produced, so sibling calls in
  separate sandboxes all get its results.
- **A task skipped by its own `if:`** caches as producing nothing, so its
  results must have come from the caller (see above), or calls fall back to
  `| default` in their own `into:`. A failure under `soft: true` is not
  cached, so the next call retries.
- **Replay overwrites.** If a caller changes `CATEGORY` between calls, the second
  call restores the cached value. Only the results (or what `into:` asks
  for) are touched.
- **`once:` belongs to the task**, not the call site.

## Steps

Five kinds. Any step takes `if:` to skip itself:

```yaml
- run: echo "lab coats on"
  if: '[ "{{.CATEGORY}}" = "science" ]'
```

### `set`: assign variables

```yaml
- set:
    - API: https://the-trivia-api.com/v2
    - RANDOM_URL: "{{.API}}/questions?limit=1" # sees the line above it
    - LABELS: { history: History buffs, science: Lab coats }
    - WEBHOOK:
        value: .SLACK_URL
        secret: true # masked wherever hobnob shows it
```

Entries resolve top to bottom. A YAML map or list literal becomes a real object
or array (see [Types](#types)).

**What `secret:` masks.** hobnob replaces the value with `****` wherever it
prints it itself: the echoed command, `quiet:` messages, `call:` summaries.
What a command prints is shown as-is, so `echo {{.WEBHOOK}}` reveals it.
Masking matches the exact value, so a copy captured by `into:` stays masked,
but a value derived from it (`| upper`, a substring, an encoding) does not.

### `run`: shell commands

```yaml
- run: curl -s "{{.API}}/questions?limit=1"
  dir: ./questions
  into:
    - QUESTIONS: stdout
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
- run: curl -s "{{.API}}/questions?limit=5"
  into:
    - QUESTIONS: stdout # the whole array
    - FIRST: stdout[0].question.text # one field out of it
    - ANSWERS: stdout[*].correctAnswer # every answer, as an array
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
- run: curl -sf {{.API}}/question/no-such-id
  soft: true
  into:
    - CODE: exit
    - ERRORS: stderr
- run: echo "no question with that id (curl exited {{.CODE}})"
  if: "{{ ne .CODE 0 }}"
```

Only a command that never started (binary not found) captures nothing. A
signal kill reports exit `-1`.

An entry can assemble a literal from several pieces:

```yaml
- run: curl -s "{{.API}}/questions?limit=1"
  into:
    - CARD:
        text: stdout[0].question.text
        niche: stdout[0].isNiche
    # CARD = {"niche":false,"text":"How many limbs…"}, niche still a real bool
```

**`quiet:`** (`run:` only) replaces the output with a one-line message:

```yaml
- run: curl -o questions.json "{{.API}}/questions?limit=10"
  quiet: Downloading ten questions
- run: ./import.sh
  quiet: true # hide output, no custom message
```

```
⊙ [seed] Downloading ten questions… (output hidden)
```

Output is hidden only on success: a failing quiet step replays its full stdout
and stderr first. `into:` captures either way.

**Background jobs.** A step owns every process it starts. When the command
exits, anything it backgrounded with `&` is sent SIGTERM; output it prints on
the way out still shows, and still reaches `into:`. A job still holding the
step's output 2s later is killed. End the command with `wait` to let its jobs
finish first:

```yaml
- run: |
    ./fetch.sh questions &
    ./fetch.sh categories &
    wait
```

To leave a job running after the step, move it out of the step's process group
and redirect its output. It outlives the run too, so stop it yourself:

```yaml
- run: bash -c 'set -m; ./server.sh > server.log 2>&1 &'
```

Run it under `bash`: `set -m` does nothing under dash, the `sh` on Debian and
Ubuntu. `setsid cmd &` is not a substitute: the step can end before the job
leaves its group, so it may be stopped anyway.

A job that has redirected its own output and ignores SIGTERM is not killed. On
platforms other than Linux and macOS nothing is signalled: leftover jobs keep
running, and the step stops waiting on their output after 2s.

#### Argv list form

A YAML sequence executes directly, one element per argument, with no shell, so
no value is ever re-split. Use it whenever an argument holds a variable:

```yaml
- run: [curl, -s, "{{.API}}/questions?limit=1&categories={{.CATEGORY}}"]
```

Each element is a whole field value, so a bare `.VAR` or a filter chain works:

```yaml
- set:
    - CURL_OPTS: ["-s", "--max-time", "10"]
- run: [curl, .CURL_OPTS, "{{.API}}/questions?limit=1"]
# argv: curl -s --max-time 10 https://the-trivia-api.com/v2/questions?limit=1
```

- **Array** splices into several arguments; an empty one splices to nothing.
- **`...`** is accepted and changes nothing, since arrays already splice:
  `[curl, .CURL_OPTS..., .URL]` runs the same as `[curl, .CURL_OPTS, .URL]`.
  Write it if you like lists to read the same everywhere (see
  [Spreading a list](#types)).
- **`""`** stays an empty argument, so later positions never shift.
- **Object** is an error. Use an accessor to pick the field you meant.

The list form gives up pipes, redirects, globs, `&&` and builtins like `cd`
(use `dir:`). The string form keeps all of them; neither is deprecated.

In the string form, escape interpolated values with [`quote`](#filters). A
YAML block scalar spares you YAML's own quoting:

```yaml
- run: |
    echo "{{ .QUESTIONS[0].question.text }} ... {{ .QUESTIONS[0].correctAnswer }}"
```

> **Unbuffered Python.** Python buffers stdout when not on a terminal, so
> output can arrive late or all at once. Fix with
> `- set: [{PYTHONUNBUFFERED: 1}]`, or `python -u` per script.

### `get`: interactive prompts

Prompts for a variable, skipped when it is already in scope.

```yaml
- get: [CATEGORY] # bare form
- get:
    - COUNT:
        info: How many questions?
        default: 3
        check: "[ {{.COUNT}} -le 10 ]"
    - CATEGORIES:
        options: .ALL_CATEGORIES
        multi: true
```

| Key         | Meaning                                                           |
| ----------- | ----------------------------------------------------------------- |
| `info:`     | Prompt text.                                                      |
| `default:`  | Pre-filled value, used as-is when prompts are off.                |
| `options:`  | Turns it into a select. A list literal or a variable holding one. |
| `multi:`    | Multi-select; the result is an array.                             |
| `check:`    | Shell condition the answer must satisfy (exit 0).                 |
| `secret:`   | Mask wherever hobnob shows it; command output is not.             |
| `optional:` | Skip silently if unanswered, leaving the variable empty.          |

When prompts are off (see [`--no-input`](#--no-input)), a missing variable
with no `default:` aborts the run.

### `call`: sub-tasks

Runs another task in a deep copy of scope. Nothing the child changes reaches
the caller except through `into:`, or, when the step has no `into:`, the
target's [`results:`](#results-what-a-call-hands-back).

```yaml
- call: _fetch-question
  dir: ./questions
  with:
    - CATEGORY: science
    - TIMEOUT_SECS: 10
  into:
    - QUESTION: .RESPONSE[0].question.text
    - ANSWER: .RESPONSE[0].correctAnswer
```

An `into:` entry names one of the child's variables (leading `.` optional),
then any accessor and filter chain, exactly like `stdout` on `run:`. Dynamic
keys and `{{ }}` entries read the caller's scope. A bare name keeps the
child's name, and mixes with other entries (`run:` rejects bare names):

```yaml
- call: _pick-disk
  into:
    - DISK # same as DISK: .DISK
    - PARTITION: "{{.DISK}}s1"
```

A name the child never set is an error that `| default` catches:

```yaml
- call: _fetch-question
  into:
    - AUTHOR: .AUTHOR | default "anonymous"
```

An entry can also be a literal built from several child values:

```yaml
- call: _fetch-question
  into:
    - CARD:
        question: .RESPONSE[0].question.text
        answer: .RESPONSE[0].correctAnswer
```

`soft: true` continues past a failed call, as on `run:`.

**`_` variables are private.** `into:` can't read a child's `_`-prefixed
variable, and `| default` doesn't catch the error. Use them for a task's
scratch values. Privacy is upward only: a child still sees the caller's `_`
variables, `with:` can pass them down, and `{{ }}` entries still read the
caller's own.

**Secrets cross calls both ways.** Masking matches on value, so a secret stays
masked when `with:` passes it down under a new name, and when `into:` pulls a
child's secret back up in any shape. Mark it `secret:` where it is defined;
`with:` rejects the flag.

### `loop`: iteration

**List form.** Iterates an array, current element as `{{.ITEM}}`:

```yaml
- run: curl -s {{.API}}/metadata
  into:
    - CATEGORIES: stdout.byCategory | keys
- loop: .CATEGORIES
  steps:
    - run: curl -s "{{.API}}/questions?limit=1&categories={{.ITEM}}"
```

A plain string runs the body once, with `ITEM` as the whole string, unsplit
(see [Types](#types)).

**Map form.** An object iterates in sorted-key order as `{{.KEY}}` and
`{{.VALUE}}`:

```yaml
- set:
    - LABELS: { history: History buffs, science: Lab coats }
- loop: .LABELS
  steps:
    - run: echo "{{.KEY}} = {{.VALUE}}"
```

**Matrix form.** Every combination of the given arrays:

```yaml
- loop:
    CATEGORY: [general_knowledge, science]
    COUNT: [1, 3]
  steps:
    - run: curl -s "{{.API}}/questions?limit={{.COUNT}}&categories={{.CATEGORY}}"
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
  - API: https://the-trivia-api.com/v2
  - LABELS: { history: History buffs, science: Lab coats }

vars:
  - CATEGORY: general_knowledge
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
  - API: https://the-trivia-api.com/v2
  - METADATA_URL: "{{.API}}/metadata" # ok, earlier entry
  - CATEGORY: '{{ .CATEGORY | default "history" }}' # error, not a file constant
```

**A `const:` name is reserved file-wide.** No `set:`/`get:`/`into:`/`loop:`
may write to it.

**`vars:` may not reference its own key.** It is already the fallback, so
`- CATEGORY: '{{ .CATEGORY | default "general_knowledge" }}'` is an error;
write `- CATEGORY: general_knowledge`.

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
      secret: true # mask this file's values wherever hobnob shows them
  - ./credentials.sh
  - defaults.txt
```

- Anything not ending in `.sh` is read as dotenv text, never run.
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
hobnob report CATEGORIES='["history","science"]'
```

```yaml
- loop: .CATEGORIES | json
```

Structure is detected once, at capture. A later accessor or filter never
re-reads a string as an array, which is why `keys` and accessors error on a
string.

**Keeping a type.** A field that is exactly one variable reference (a bare
`.VAR`, an accessor chain, or a single `{{ }}` action, with optional filters)
keeps its type. Any surrounding text makes it a string:

```yaml
- set:
    - A: .CATEGORIES # still an array
    - B: "{{ .CATEGORIES }}" # text (JSON text, here)
    - C: "count: {{ .CATEGORIES | len }}" # text, surrounding text forces it
```

This is what makes `options: .CATEGORIES` and `- run: [curl, .CURL_OPTS, .URL]`
work.

**Spreading a list.** A list literal nests what it holds, as JSON does. A
trailing `...` on an element splices that list's items in instead:

```yaml
- set:
    - CURL_OPTS: [-s, --max-time, "10"]
    - EXTRA: [--retry, "3"]
    - NESTED: [.CURL_OPTS, --verbose] # [["-s", "--max-time", "10"], "--verbose"]
    - FLAT: [.CURL_OPTS..., .EXTRA..., --verbose] # ["-s", "--max-time", "10", "--retry", "3", "--verbose"]
```

- Any number of spreads, in any position. Only Go's `...` spelling is
  borrowed, not its rule of one spread as the last argument.
- It works in every `set:`/`with:`/`into:`/`const:`/`vars:` list literal. In
  `into:`, spread a source the same way: `[stdout..., exit]`.
- `...` covers the whole element, accessors and filters included:
  `.QUESTIONS[*].category...`, `(.TAGS | json)...`.
- An empty list adds nothing.
- Spreading anything but a list (string, number, bool, object, null) is an
  error. A string names `| json`, the way to parse text that should be a list.
- A missing path errors, as anywhere else, unless `| default` catches it.
- `...` outside a list element is an error when the file loads, in any field
  that takes a reference: `B: .A...`, `loop: .A...`, `options:`, `default:`,
  `dir:` and the rest hold one value, so there is no list to splice into.
  Neither has an object field or a bare entry in `set:`/`into:`.
- Only a reference spreads. Plain text such as `Loading...` stays text; write
  a reference followed by literal dots as a template: `"{{ .A }}..."`.

## Accessors

Query an array or object with the syntax the value resembles:

```yaml
- run: curl -s "{{.API}}/questions?limit=1"
  into:
    - QUESTIONS: stdout
- set:
    - QUESTION: .QUESTIONS[0].question.text
```

| Form                       | Meaning                                                         |
| -------------------------- | --------------------------------------------------------------- |
| `.A.b.c`                   | object keys                                                     |
| `.A[0]`                    | array index                                                     |
| `.A[-1]`                   | negative index, from the end                                    |
| `.A[1:3]`                  | slice (bounds clamp, like Go: `[0:99]` on 3 elements returns 3) |
| `.A[*]`                    | every element of an array, or every value of an object          |
| `.A["key with . or /"]`    | a literal key that is not a valid identifier                    |
| `.A[.KEY]`                 | dynamic key or index, taken from a variable                     |
| `.A[.KEY][0].name`         | any combination, any depth                                      |
| `(.CATEGORIES \| json)[0]` | a pipeline result as the head                                   |

`{{ .LABELS[.CATEGORY] }}` is a lookup, not string concatenation, so a key
holding `.` or `[` (`science.v2`) is matched literally.

**Multiplicity.** After a slice or `[*]`, later steps map over every node,
producing an array:

```yaml
- run: curl -s "{{.API}}/questions?limit=5"
  into:
    - QUESTIONS: stdout
- set:
    - TEXTS: .QUESTIONS[*].question.text # ["How many limbs does an octopus have?", …]
    - FIRST_TWO: .QUESTIONS[0:2].difficulty # ["easy", "hard"]
```

- Nodes with no match, or of the wrong kind, are **dropped**, as with `lines`
  and `split`.
- Matching nothing yields an **empty array**, not absence.
- `[*]` on an object yields values in **sorted-key order**, matching `keys`.

**Absence is an error, caught by `default`.** A missing key, an out-of-range
index, or indexing into something that is not an array or object errors:

```yaml
{{ .QUESTIONS[0].author }}                        # error: path not found
{{ .QUESTIONS[0].author | default "anonymous" }}  # "anonymous"
```

`default` catches absence only. A wrong-kind access (indexing a string,
slicing an object) is a taskfile bug fixed with `| json`, and `default` never
catches it.

This strictness applies wherever a chain lands, argv included:
`- run: [echo, .QUESTIONS[0].corectAnswer]` aborts on the typo rather than pass an
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
    - CATEGORY: '{{ .TRIVIA_CATEGORY | default "general_knowledge" }}'
- run: curl -s "{{.API}}/questions?limit=10" | jq -r '.[].question.text'
  into:
    - TEXTS: stdout | lines
- run: echo {{ .QUESTIONS[0].correctAnswer | quote }}
```

- **`json`** leaves structured values alone, so it is always safe before an
  accessor or `keys`.
- **`keys`** errors on anything but an object, naming `| json`. For values,
  use `[*]`: with `/metadata` captured as `META`, `.META.byDifficulty | keys`
  gives `["easy","hard","medium"]` and `.META.byDifficulty[*]` gives
  `[2963,4997,7339]`, same order.
- **`quote`** escapes for the string `run:` form; the
  [argv list form](#argv-list-form) usually removes the need.

Name intermediate values rather than stacking many pipes inline:

```yaml
- set:
    - CATEGORIES: "{{ .CATEGORIES_TEXT | json }}"
- run: [curl, -s, "{{.API}}/questions?limit=1&categories={{ .CATEGORIES[0] }}"]
```

### Comparisons

`eq` / `ne` / `lt` / `le` / `gt` / `ge` compare typed values. `if:` runs the
rendered `true`/`false` as a shell condition:

```yaml
- run: echo "lab coats on"
  if: '{{ eq .CATEGORY "science" }}'
- run: echo "that is a lot of questions"
  if: "{{ gt (.QUESTIONS | len) 5 }}"
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
`eq .CATEGORY "history" "geography"`.

## Modules

Import tasks from other files.

```yaml
modules:
  - trivia: ./trivia.yml # short form: key and path
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
