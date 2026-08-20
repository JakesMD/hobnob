# hobnob guide

Hobnob is a timeline-based task runner. Variables, conditions, and prompts
evaluate sequentially as execution reaches each step — not compiled into a
static dependency tree upfront.

---

## Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

Detects OS/architecture, installs to `~/.local/bin`, sets up shell completion
(bash, zsh, fish).

---

## CLI

```bash
hobnob                               # run "default" task (or select)
hobnob deploy                        # run a task
hobnob deploy ENV=production         # pass runtime variables
hobnob deploy --no-input             # skip prompts, fail on missing vars
hobnob --file ops/tasks.yml build    # target a specific file
hobnob --list                        # list all public tasks
hobnob --select                      # interactively select a task to run
hobnob --help                        # show help and available tasks
hobnob --version                     # print version and exit
hobnob --upgrade                     # upgrade to the latest release
```

- **Default task** — a task named `default` runs on bare `hobnob`. Without one,
  the interactive selector opens. `--select` forces the selector even when a
  default exists.
- **Auto-discovery** — no `--file`? Hobnob searches up from cwd for
  `hobnob.yml`/`hobnob.yaml` (`.yml` wins if both exist).
- **Stopping a run** — CTRL+C sends the running `run:` step a termination signal
  and waits for it to exit; no further steps start. A 2nd CTRL+C force-kills it
  immediately.

---

## File structure

Three optional top-level keys:

```yaml
env: # files to source (.env or .sh)
  - .env

modules: # imported hobnob files
  - utils: ./utils.yml

tasks: # task definitions
  my-task:
    steps:
      - run: echo hello
```

---

## Modules

Import tasks from other files via `modules:`.

```yaml
modules:
  - _infra: terraform.yml
  - docker:
      path: ./docker/tasks.yml
      show: [build, push]
      flatten: true
```

- **Namespaces** — imported tasks are prefixed with their module key
  (`call: docker:build`). A `_` prefix makes the module internal — hidden from
  `--list` and parent files.
- **Filters** — `show:` whitelists, `hide:` blacklists.
- **Flattening** — `flatten: true` also registers tasks under their bare name
  (`build` as well as `docker:build`). Native tasks win on conflicts.
- **Scoping** — a module's own `env:` block is sourced for that module's own
  subtree only; it never leaks to the parent.

---

## Tasks

A task is a named sequence of steps. A `_` prefix (`_compile`) makes it internal
— hidden from `--list`, only reachable via `call` or `use`.

### `if:` — conditional execution

Skips the whole task when the shell condition exits non-zero. A skipped called
task doesn't error — execution just continues in the caller.

```yaml
tasks:
  deploy:
    if: '[ "{{.ENV}}" = "production" ]'
    steps:
      - run: ./deploy.sh
```

### `interactive:` — disable prompts

`interactive: false` disables all `get:` prompts for the whole sub-tree: prompts
with a `default:` use it automatically, required prompts without one abort. Once
disabled, a called task can't re-enable it. Works on tasks and `call` steps:

```yaml
tasks:
  deploy:
    interactive: false
    steps:
      - get:
          - ENV:
              default: staging # used automatically
      - call: _setup # also runs with prompts disabled

  release:
    steps:
      - call: deploy
        interactive: false # only this call site disables prompts
```

### `dir:` — working directory

Paths resolve relative to the hobnob file. No `dir:` set anywhere → steps run in
the hobnob file's directory.

- **Task `dir:`** — working dir for all `run` steps, inherited down the call
  chain unless a descendant sets its own.
- **Call step `dir:`** — overrides the called task's own `dir:`, becomes the
  inherited dir for its descendants.
- **Run step `dir:`** — overrides for that one step only.

```yaml
tasks:
  deploy:
    dir: ./infra
    steps:
      - run: terraform apply # runs in ./infra
      - run: go test ./...
        dir: ../tests # only this step runs in ../tests
      - call: _verify # inherits ./infra
      - call: _verify
        dir: ./staging # overrides _verify's own dir:
```

`if:` always evaluates in the inherited task `dir:`, even on a step with its own
`dir:` override.

---

## Variables

Evaluated at runtime with Go templates (`{{ .VAR }}`).

### Precedence (highest to lowest)

```
env  <  env files  <  CLI args  <  timeline (set / get / loop / use)
```

Env is lowest so ambient shell state can't silently change task behavior across
machines. Env files rank above OS env (explicit project config, not ambient) but
below CLI args (caller's explicit override wins). Above CLI args, precedence is
no longer a rule but execution order — a task's own `set:`/`get:`/`loop:`/`use:`
steps run after the initial scope is built, and each sees everything before it.

The two roles a variable can play each have their own verb:

- **A known value the task controls** — use `set:`. It always wins over a CLI
  arg with the same name; nothing should silently override it.
- **An overridable default** — use `get:` with a `default:`. A caller's CLI arg
  satisfies the prompt (so it's skipped) and wins; with no arg, the default is
  used:

```yaml
tasks:
  deploy:
    steps:
      - get:
          - HOST:
              default: localhost
      - run: echo host={{.HOST}}
```

### Env files

`env:` lists files to source, resolved relative to the hobnob file:

```yaml
env:
  - .env:
      secret: true # opt this file into masking
  - ./secrets.sh
  - config.txt
```

- **Plain files** (anything not ending in `.sh`) are parsed as `KEY=VALUE` lines
  — blank lines, `#` comments, and an optional `export` prefix are supported.
- **`.sh` files** are sourced in a subshell. Only variables the script newly
  sets or changes are pulled into scope — inherited-but-unchanged vars are
  ignored.
- A referenced file that doesn't exist prints a warning and is skipped, rather
  than failing the run.
- Vars are not masked in output by default, regardless of filename. Add
  `secret: true` to an entry, using the expanded `path: { secret: true }` form
  shown above, to mask its vars.
- Later entries override earlier ones (same ordering rule as `set:`); if two
  entries set the same var, masking follows whichever value won.

### Scope isolation

`call` gives the child task a deep copy of scope. Mutations stay sandboxed
unless pulled back with `into:`. `use` is the opposite — see
[`use` — shared prologues](#use--shared-prologues) below.

### Top-to-bottom resolution

A `set` block resolves top-to-bottom — reference a key defined just above:

```yaml
- set:
    - BASE_URL: https://api.example.com
    - AUTH_URL: "{{.BASE_URL}}/v1/auth"
```

### Typed values

A variable holds what its data actually is — text, a number, `true`/`false`,
a list, or an object — not JSON squeezed into a string. Structure only ever
enters scope from three places:

- a `set:`/`with:` map or list literal
- capturing `run:` output via `into:`, when the output decodes cleanly as a
  JSON array/object (plain text, or text that merely starts with `{` or `[`
  without being valid JSON, stays text)
- the explicit `json` filter

Environment variables, CLI `KEY=VALUE` args, and `env:` file values are
always text, however JSON-shaped they look — nothing is guessed. Pipe a
value through `| json` to parse it explicitly:

```yaml
- loop: .TAGS | json # TAGS='["a","b"]' on the command line
```

A field that's exactly one variable reference — a bare `.VAR`, an accessor
chain (`.VAR.field[0]`), or a single `{{ }}` action, optionally through a
filter chain — keeps that variable's type. Any surrounding text renders it
to a plain string, same as always:

```yaml
- set:
    - A: .USERS # keeps USERS' type (an Array stays an Array)
    - B: "{{ .USERS }}" # renders to text — JSON text, if USERS is structured
    - C: "count: {{ .USERS | len }}" # text either way — surrounding text forces it
```

### Accessors

Query a variable holding a real array or object — a map/list literal (see
[`set`](#set--assign-variables)), JSON captured via `into:`, or anything
already run through `| json` — with the same dot/bracket syntax the value
itself resembles, instead of a quoted path string:

```yaml
- run: curl -s https://api.example.com/user
  into:
    - RESP: stdout
- set:
    - NAME: .RESP.profile.name
```

| Form | Meaning |
| --- | --- |
| `.A.b.c` | object keys |
| `.A[0]` | array index |
| `.A[-1]` | negative index, from the end |
| `.A[1:3]` | slice |
| `.A[*]` | every element (array) or every value (object) |
| `.A["key with . or /"]` | a literal key that isn't a valid identifier |
| `.A[.KEY]` | dynamic key or index, taken from a variable |
| `.A[.KEY][0].name` | any combination, any depth |

Dynamic keys are what accessors solve that a path string can't: looking up a
key held in another variable is a plain reference, not string concatenation
— so a key containing `.` or `[` (`app.kubernetes.io/name`, say) is looked up
literally rather than silently mis-parsed as a nested path:

```yaml
- set:
    - KEY: app.kubernetes.io/name
- run: echo {{ .LABELS[.KEY] }}
```

A `(pipeline)` result can be the head of an accessor too, when there's no
variable to hang it on: `(.TAGS | json)[0]`.

#### Multiplicity

A slice or `[*]` yields multiple nodes. A step after one **maps over the
results** and yields an Array:

```yaml
.ITEMS[*].name # ["ada", "grace"]
.ITEMS[1:3].name # the same, over a slice
```

Elements with no match, or the wrong kind, are **dropped**, not held as nil
placeholders or a hard failure — matching hobnob's existing convention
(`lines` drops blank lines, `split` drops empty parts). A slice or `[*]` that
matches nothing yields an empty Array, not an error: multiplicity is not
absence.

Slice bounds clamp like Go's own slice semantics — `.ITEMS[0:99]` on a
3-element array returns all 3, it doesn't error. `[*]` on an object yields
every value, in sorted-key order — the same order `| values` uses, so
`.CFG[*]` and `.CFG | values` give the same result.

#### Absence is an error, caught by `default`

A missing key, an out-of-range index, or a value that isn't an array/object
(a plain string, even one holding valid-looking JSON text — see [Typed
values](#typed-values)) errors — before every step but a slice/`[*]` has
run, at least. Pipe through `default` to opt out per accessor, same as any
other empty value:

```yaml
{{ .RESP.profile.name | default "Andrew" }} # "Andrew" if missing
{{ .RESP.profile.name }} # error: path not found
```

`default` only ever catches *absence*. A wrong-kind access — indexing a
string, slicing an object — is never caught by it, even with `| default`
right after: that's a task-file bug with a specific remedy (`| json`), not a
fact about the data, and hobnob won't blur the two. This is why structure is
sniffed exactly once (see [Typed values](#typed-values)) — a String is never
silently re-interpreted as an array or object by an accessor either.

Because an accessor errors by default, it's strict everywhere a filter
chain's result lands, including [argv elements](#argv-list-form) — a typo
aborts the run instead of passing an empty argument:

```yaml
- run: [aws, s3, rm, --recursive, .CONFIG.bucket] # a typo'd bucket key fails loudly
```

> Migrating from `pluck` (removed in v0.3.0): rewrite the path string as an
> accessor chain.
>
> ```
> {{ .RESP | pluck "profile.name" }}      →  {{ .RESP.profile.name }}
> {{ .RESP | pluck "items[0]" }}          →  {{ .RESP.items[0] }}
> {{ .RESP | pluck "items[1:3]" }}        →  {{ .RESP.items[1:3] }}
> {{ .RESP | pluck "items[*].name" }}     →  {{ .RESP.items[*].name }}
> {{ .RESP | pluck "a.b" "unknown" }}     →  {{ .RESP.a.b | default "unknown" }}
> {{ .RESP | pluck "items[?@.x]" }}       →  no direct equivalent — filter expressions
>                                             are gone; a shell step with jq covers
>                                             the rare case that needs one
> ```

### Built-in variables

- `HOBNOB_FILE_DIR` — directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR` — directory hobnob was run from.

### Template filters

Usable anywhere templates are supported.

#### `default`

Falls back to a lower-priority layer when the piped value is empty — see
[Variables](#variables) for the precedence chain this is typically used with:

```yaml
- set:
    - REGION: '{{ .AWS_REGION | default "us-east-1" }}'
```

#### `trim`

Strips leading/trailing whitespace:

```yaml
- run: echo "{{ .NAME | trim }}"
```

#### `upper` / `lower`

Changes case:

```yaml
- run: echo "Targeting {{.ENV | upper}}"
```

#### `split`

Splits a string on a separator into a list, dropping empty parts:

```yaml
- set:
    - PARTS: '{{ .PATH | split ":" }}' # "a:b:c" -> ["a","b","c"]
```

#### `lines`

Splits a string on newlines into a list, trimming each line and dropping
blank ones — handy for turning multi-line `stdout` into a list:

```yaml
- run: ls
  into:
    - FILES: stdout | lines
```

#### `first`

Returns the first element of an array, typed:

```yaml
- set:
    - LATEST: "{{ .VERSIONS | first }}"
```

Requires an actual array — a string, even one holding JSON array text, errors
naming `| json` rather than being auto-parsed. See [Typed
values](#typed-values).

#### `json`

Parses a string as JSON, producing a real array or object. The one place
you'll reach for it explicitly is a var that was never captured through
`into:` — a `set:` scalar, a CLI arg, an env var:

```yaml
- set:
    - TAGS_TEXT: '["a","b"]' # a plain string — its text merely looks like JSON
- loop: .TAGS_TEXT | json # parse it, then iterate the array
```

It's identity on a value that's already structured, so `| json` is always
safe to add defensively before an [accessor](#accessors)/`keys`/`values`
without checking the source first. An optional fallback swallows a parse
failure instead of erroring: `.TEXT | json "fallback"`.

#### `keys` / `values`

Query a variable holding a real object — a map literal or JSON captured via
`into:`, same sources an [accessor](#accessors) reads:

```yaml
- set:
    - FIELDS: "{{ .RESP | keys }}" # sorted list of top-level keys
    - VALUES: "{{ .RESP | values }}" # values in that same sorted-key order, still typed
```

Results are lists, so they chain into other filters: `{{ .RESP | keys | first
}}`. A value that isn't a real object — including a string, even one holding
object-shaped text — is an error naming `| json`.

#### `string`

The reverse of `json` — forces a value back to text, compact JSON for a list
or object:

```yaml
- run: echo '{{ .RESP | string }}' # always text, regardless of RESP's type
```

#### `quote`

Wraps a value in POSIX single quotes for safe interpolation into a `run:`
string command, escaping any embedded single quote:

```yaml
- run: echo {{ .NAME | quote }}
```

The escape hatch for the string form — see [Argv list
form](#argv-list-form), which removes the need for it in the common case by
avoiding the shell entirely. Arrays and objects are rendered as compact JSON
first, matching `string`, then quoted.

#### `len`

Length of a string (in runes), array, or object (its key count):

```yaml
- get:
    - CONFIRM:
        check: "[ {{ .TAGS | len }} -gt 0 ]" # at least one tag selected
```

A field value that's a single variable reference (optionally with a pipe chain)
can omit `{{ }}`:

```yaml
- get:
    - RELEASE:
        options: .VERSIONS
        default: .VERSIONS | first
```

#### Comparisons

`eq` / `ne` / `lt` / `le` / `gt` / `ge` compare two typed values directly —
handy in `if:`, which runs its rendered `true`/`false` as a shell condition:

```yaml
- run: ./deploy-prod.sh
  if: '{{ eq .OS "darwin" }}'
- run: echo "batch too large"
  if: '{{ gt (.ITEMS | len) 100 }}'
```

Comparison is lenient about representation: if both sides are plain JSON
number text — `-3`, `1.5`, `1e9` — regardless of which `Kind` actually
carries it, they compare numerically (exactly, at any magnitude — never
through a lossy float64), so `{{ lt .A .B }}` with `A=9` and `B=10` (CLI
args, always text) is `true`, not `false` as a lexical `"10" < "9"` would
give. Text that merely looks numeric to a human but isn't valid JSON number
grammar — `Inf`, `NaN`, `0x10`, a leading-zero `007`, a leading-plus `+3` —
is compared as text instead, not parsed as a lenient float would. A missing
var compares equal to `""`, matching the `IsEmpty` rule used everywhere
else. Otherwise comparison falls back to exact text equality / lexical
ordering.

Bools compare for `eq`/`ne` only — `lt`/`le`/`gt`/`ge` on a bool is an error,
same as text/template's own builtins. Arrays and objects aren't comparable at
all; access the field you actually mean to compare. `eq` (like
text/template's own) accepts multiple right-hand arguments and is true if
any match: `eq .ENV "staging" "qa"`.

---

## Steps

Every task is a sequence of six step types.

### `set` — assign variables

```yaml
- set:
    - TARGET_HOST: localhost
    - APPLICATION_KEY:
        value: .VAULT_TOKEN
        secret: true # masked in terminal output
    - REGION_MAP: { us: us-east-1, eu: eu-west-1 } # map literal -> JSON object
```

Any variable can hold nested structure — write it as a YAML map/list literal
(`REGION_MAP` above). A plain string that merely looks like JSON stays a
string — see [Typed values](#typed-values) — pipe it through `| json` to
parse it.

### `run` — shell commands

```yaml
- run: git rev-parse --short HEAD
  dir: ./infra # working directory for this step
  into:
    - GIT_SHA: stdout | trim
    - ERROR_LOG: stderr
```

`stdout`/`stderr` accepts a trailing [accessor](#accessors)
(`stdout[0].name`) and/or a pipe into any [template filter](#template-filters),
chained the same way as in `{{ }}` templates — `stdout | lines | first`, and
so on.

```yaml
- run: curl -s https://api.example.com/user
  into:
    - CUSTOM:
        id: stdout.id
        name: stdout.profile.name
    # -> CUSTOM = {"id":42,"name":"Ada"} — id stays a real number
```

> Unbuffered Python: scripts buffer stdout when not attached to a terminal, so
> `run:` output can appear late or all at once. Fix with `- set: [{PYTHONUNBUFFERED: 1}]`,
> or `python -u` per script. See
> [Python `-u` docs](https://docs.python.org/3/using/cmdline.html#cmdoption-u).

#### Argv list form

A YAML sequence executes directly, one element per argument — no `sh -c`, so
no shell parses the rendered command:

```yaml
- run: [docker, push, "{{ .IMAGE }}"]
```

`IMAGE` can hold spaces, quotes, `$`, or a semicolon and still arrives as
exactly one argument — the string form can't make that guarantee, since a
rendered value is spliced into a command *string* that the shell then parses.
Use the list form whenever a command's arguments include a variable, unless
you specifically need the shell features below.

Each element is a whole field value, so it can be a bare `.VAR` (or a filter
chain) without `{{ }}`, same as `loop:`/`options:`/`into:`. An element that
resolves to an Array splices into multiple arguments — one real payoff of
typed values a plain task runner can't offer:

```yaml
- set:
    - FLAGS: ["-ldflags", "-s -w"]
- run: [go, build, .FLAGS, ./cmd/hobnob]
# argv: go build -ldflags "-s -w" ./cmd/hobnob — "-s -w" arrives as one argument
```

An empty array splices to nothing; an element resolving to `""` is preserved
as an empty argument (dropping it would shift every later positional
argument). An Object element is an error — use an [accessor](#accessors)
(`.VAR.field`) to select the field you mean to pass instead of letting it
stringify by accident.

What it gives up: no pipes, redirects, globs, `&&`, or shell builtins. `cd` in
particular is unavailable — that's what [`dir:`](#dir--working-directory) is
for. The string form stays for all of it; neither form is deprecated.

#### The `quote` filter, and block scalars

The string form still needs a shell, so use the [`quote` filter](#quote) to
escape a value going into it.

Separately, a block scalar (`|`) removes YAML's own quoting layer — inside it,
quotes are literal, so nested `"` no longer need escaping:

```yaml
- run: |
    echo "{{ .JOKE[0].setup }} ... {{ .JOKE[0].punchline }}"
```

For any command using more than one filter, name the intermediate value
instead of stacking pipes inline — one layer per line, and the value has a
name when it needs debugging:

```yaml
- set:
    - FIRST_TAG: '{{ .TAGS_TEXT | json | first }}'
- run: [echo, .FIRST_TAG]
```

### `get` — interactive prompts

Prompts for input; skipped if the variable already exists in scope.

> With `--no-input`, a `CI` env var, or non-terminal stdin (AI agents, other
> non-interactive callers), prompts are skipped. Missing vars with no `default`
> abort.

Bare form:

```yaml
- get: [MY_VAR]
```

Object form:

```yaml
- get:
    - PORT:
        info: Enter deployment port
        default: 8080
        check: "[ {{.PORT}} -gt 1024 ]" # must exit 0 to pass
    - ENVIRONMENT:
        info: Select target stage
        options: [staging, production]
    - TAGS:
        info: Select applicable tags
        options: [backend, frontend, infra]
        multi: true # multi-select prompt
    - API_TOKEN:
        info: Paste your API token
        secret: true # masked in terminal output
    - NOTES:
        info: Any notes? (optional)
        optional: true # skipped silently if missing, leaves variable empty
```

### `call` — sub-tasks

Runs another task in an isolated scope. Pass vars in with `with:`, pull results
back with `into:`.

```yaml
- call: deploy_pipeline
  dir: ./infra
  with:
    - TARGET_ENV: production
    - TIMEOUT_SECS: 90
  into:
    - DEPLOY_STATUS: .STATUS
    - ARTIFACT_PATH: .LOG_FILE
```

A non-zero exit halts the timeline by default. `soft: true` continues past a
failed call:

```yaml
- call: flaky_cleanup_script
  soft: true
- call: next_critical_step
```

`with:` entries take no `secret:` flag — it's rejected at parse time. Masking
matches on value, so a secret stays masked once passed down, even under a new
name; mark it secret where it's defined (`set:`, `get:`, or `env:`) instead.

### `use` — shared prologues

Runs another task's steps directly against the caller's scope: no sandbox, no
`with:`/`into:` (both are rejected at parse time — there's nothing to pass in
or pull back when the scope is already shared).

```yaml
tasks:
  _setup:
    steps:
      - get:
          - ENV:
              options: [staging, production]
              default: staging
      - run: aws sts get-caller-identity
        into:
          - ACCOUNT: stdout.Account

  deploy:
    steps:
      - use: _setup
      - run: docker push app:{{.ENV}}

  rollback:
    steps:
      - use: _setup # cached: no second prompt, no second aws call
      - run: ./rollback.sh {{.ACCOUNT}}
```

`call` sandboxes. `use` does not. That is the whole distinction.

**Memoized** — a task reached via `use:` runs at most once per hobnob
invocation. The first run's variables are snapshotted; a later `use:` of the
same task replays that snapshot into scope instead of re-executing. This is
what makes the `rollback` example above skip the prompt and the API call the
second time, and what makes replay correct across `call:`'s sandbox boundary:

```yaml
main:
  steps:
    - call: _a # sandboxed; uses _setup, ACCOUNT lands in _a's copy
    - call: _b # sandboxed; uses _setup — replays the snapshot, gets ACCOUNT
```

Caching only the fact that a task ran would leave `_b` with nothing — the
memo replays the actual result, not just a "done" flag. A task skipped by its
own `if:` is cached as having produced nothing; a failure under `soft: true`
is not cached and is retried by the next `use:`.

**`rerun: true`** opts one `use:` step out of memoization — needed inside a
`loop:`, where a `use:` would otherwise run on the first iteration only:

```yaml
- loop: .SERVICES
  steps:
    - use: _check_service
      rerun: true
```

**Sharp edge** — replay overwrites. `use:` asserts a task's results are in
scope; it is not "maybe run something":

```yaml
- use: _setup # TOKEN=abc
- set:
    - TOKEN: xyz
- use: _setup # replays the snapshot — TOKEN is abc again
```

`dir:` behaves as it does for `call:` — the used task's own `dir:` applies to
its own `run:` steps and does not leak into the caller's later steps; a
step-level `dir:` overrides it. `if:` and `interactive:` also work as they do
for `call:`.

> Migrating a `vars:` block (removed in v0.3.0): move each entry into a task
> and `use:` it wherever it's needed.
>
> ```yaml
> # before
> vars:
>   - REGISTRY: ghcr.io
>   - HOST: '{{ .HOST | default "localhost" }}'
>
> # after
> tasks:
>   _setup:
>     steps:
>       - set:
>           - REGISTRY: ghcr.io
>       - get:
>           - HOST:
>               default: localhost
> ```
>
> Each task that needs the prologue adds `- use: _setup` as its first step.
> Nothing enforces that — a task that omits it runs with the variables unset,
> rendering as empty rather than erroring.

### `loop` — iteration

List form — iterates an Array, current element (typed) as `{{.ITEM}}`. A
plain string source — one never captured or built as structure, see [Typed
values](#typed-values) — runs the body once with `ITEM` set to the whole
string, rather than being split or parsed:

```yaml
- loop: .GO_FILES
  steps:
    - run: go fmt {{.ITEM}}
```

Matrix form — runs every combination of the given arrays:

```yaml
- loop:
    OS: [linux, darwin]
    ARCH: [amd64, arm64]
  steps:
    - run: echo "Compiling for {{.OS}} on {{.ARCH}}"
```

Map form — when the variable is an Object rather than an Array, iterates its
entries in sorted-key order as `{{.KEY}}` / `{{.VALUE}}`, both typed:

```yaml
- loop: .REGION_MAP
  steps:
    - run: deploy --region {{.VALUE}} # e.g. KEY=eu, VALUE=eu-west-1
```

### `if:` — conditional steps

Any step can be conditionally skipped. Exit `0` proceeds, non-zero skips.

```yaml
- run: echo "Purging production cache..."
  if: '[ "{{.ENV}}" = "production" ]'
```

Runs in the inherited task `dir:`, even on a step with its own `dir:` override —
see [`dir:`](#dir--working-directory) above.

---

## Best practices

- **Task names** — kebab-case (`deploy-production`, not `deploy_production`).
- **Variable names** — `ALL_CAPS` with underscores.
- **Field ordering** — `info:` before `steps:` in tasks; `info:` first in `get:`
  entries.
- **Prompt placement** — put `get:` steps early. Prompts buried after slow `run`
  steps make the user wait before they can answer.
