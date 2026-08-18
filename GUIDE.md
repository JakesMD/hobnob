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

- **Default task** — a task named `default` runs on bare `hobnob`. Without
  one, the interactive selector opens. `--select` forces the selector even
  when a default exists.
- **Auto-discovery** — no `--file`? Hobnob searches up from cwd for
  `hobnob.yml`/`hobnob.yaml` (`.yml` wins if both exist).
- **Stopping a run** — CTRL+C sends the running `run:` step a termination
  signal and waits for it to exit; no further steps start. A 2nd CTRL+C
  force-kills it immediately.

---

## File structure

Four optional top-level keys:

```yaml
vars: # global variables
  - KEY: value

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
  (`call: docker:build`). A `_` prefix makes the module internal — hidden
  from `--list` and parent files.
- **Filters** — `show:` whitelists, `hide:` blacklists.
- **Flattening** — `flatten: true` also registers tasks under their bare
  name (`build` as well as `docker:build`). Native tasks win on conflicts.
- **Scoping** — modules inherit root `vars:` read-only; a module's own
  `vars:` never leaks to the parent. The same rule applies to a module's own
  `env:` block — sourced for that module's own subtree only.

---

## Tasks

A task is a named sequence of steps. A `_` prefix (`_compile`) makes it
internal — hidden from `--list`, only callable via `call`.

### `if:` — conditional execution

Skips the whole task when the shell condition exits non-zero. A skipped
called task doesn't error — execution just continues in the caller.

```yaml
tasks:
  deploy:
    if: '[ "{{.ENV}}" = "production" ]'
    steps:
      - run: ./deploy.sh
```

### `interactive:` — disable prompts

`interactive: false` disables all `get:` prompts for the whole sub-tree:
prompts with a `default:` use it automatically, required prompts without one
abort. Once disabled, a called task can't re-enable it. Works on tasks and
`call` steps:

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

Paths resolve relative to the hobnob file. No `dir:` set anywhere → steps
run in the hobnob file's directory.

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

`if:` always evaluates in the inherited task `dir:`, even on a step with its
own `dir:` override.

---

## Variables

Evaluated at runtime with Go templates (`{{ .VAR }}`).

### Precedence (highest to lowest)

| Priority | Scope     | Description                               |
| -------- | --------- | ------------------------------------------ |
| **1**    | Local     | Set via `set`, `get`, or loop iterators.  |
| **2**    | Passed    | Injected explicitly via `call`'s `with:`. |
| **3**    | Inherited | Copied from the calling task.             |
| **4**    | Global    | Declared in the root `vars:` block.       |
| **5**    | CLI args  | `KEY=VALUE` args on the command line.     |
| **6**    | Env files | Sourced via the root `env:` block.        |
| **7**    | Env       | OS environment variables.                 |

<details>
<summary>Rationale</summary>

Env is lowest so ambient shell state can't silently change task behavior
across machines. Env files rank above OS env (explicit project config, not
ambient) but below CLI args (caller's explicit override wins). Global vars
rank above CLI args because `vars:` is internal wiring that shouldn't be
silently overridden — callers should use `get:` steps instead, which
auto-skip once the variable is already in scope.

</details>

Use `{{ .VAR | default "fallback" }}` to fall back to a lower-priority layer:

```yaml
vars:
  - HOST: "{{ .HOST | default "localhost" }}"
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

- **Plain files** (anything not ending in `.sh`) are parsed as `KEY=VALUE`
  lines — blank lines, `#` comments, and an optional `export ` prefix are
  supported.
- **`.sh` files** are sourced in a subshell. Only variables the script newly
  sets or changes are pulled into scope — inherited-but-unchanged vars are
  ignored.
- A referenced file that doesn't exist prints a warning and is skipped,
  rather than failing the run.
- Vars are not masked in output by default, regardless of filename. Add
  `secret: true` to an entry, using the expanded `path: { secret: true }`
  form shown above, to mask its vars.
- Later entries override earlier ones (same ordering rule as `vars:`); if
  two entries set the same var, masking follows whichever value won.

### Scope isolation

`call` gives the child task a deep copy of scope. Mutations stay sandboxed
unless pulled back with `into:`.

### Top-to-bottom resolution

A `set` block resolves top-to-bottom — reference a key defined just above:

```yaml
- set:
    - BASE_URL: https://api.example.com
    - AUTH_URL: "{{.BASE_URL}}/v1/auth"
```

### Built-in variables

- `HOBNOB_FILE_DIR` — directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR` — directory hobnob was run from.

### Template filters

Usable anywhere templates are supported.

#### `default`

Falls back to a lower-priority layer when the piped value is empty — see
[Variables](#variables) for the precedence chain this is typically used with:

```yaml
vars:
  - HOST: "{{ .HOST | default "localhost" }}"
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

Splits a string on a separator into a JSON array, dropping empty parts:

```yaml
- set:
    - PARTS: '{{ .PATH | split ":" }}' # "a:b:c" -> ["a","b","c"]
```

#### `lines`

Splits a string on newlines into a JSON array, trimming each line and
dropping blank ones — handy for turning multi-line `stdout` into a list:

```yaml
- run: ls
  into:
    - FILES: stdout | lines
```

#### `first`

Returns the first element of a JSON array:

```yaml
- set:
    - LATEST: "{{ .VERSIONS | first }}"
```

#### `pluck`

Queries a variable holding JSON — either a map literal (see
[`set`](#set--assign-variables)) or JSON captured from a command, e.g.
`run: curl ... into: RESP: stdout`:

```yaml
- run: curl -s https://api.example.com/user
  into:
    - RESP: stdout
- set:
    - NAME: '{{ .RESP | pluck "profile.name" }}' # dot/bracket path: a.b[0].c
```

Path is [RFC 9535 JSONPath](https://www.rfc-editor.org/rfc/rfc9535.html)
(via [github.com/theory/jsonpath](https://github.com/theory/jsonpath)), minus
the leading `$` — implied, since pluck always addresses from the root. Beyond
`a.b[0].c`, that also covers slices, negative indices, wildcards, and filters:

```yaml
- set:
    - PAIR: '{{ .RESP | pluck "items[1:3]" }}'          # slice -> ["b","c"]
    - LAST: '{{ .RESP | pluck "items[-1]" }}'            # negative index -> "c"
    - NAMES: '{{ .RESP | pluck "items[*].name" }}'       # wildcard -> ["a","b","c"]
    - ACTIVE: '{{ .RESP | pluck "items[?@.active == true]" }}' # filter
```

One match returns unwrapped; multiple return a JSON array (chains into other
filters and `loop:`, same as `keys`/`values`). In filters, `@` is the node
under test — bare `@.field` only checks existence, so compare explicitly for
a truthy check: `@.active == true`.

Missing key, bad index, or invalid JSON errors by default. Add a fallback to
opt out per call: `pluck "profile.name" "unknown"` returns `"unknown"`
instead of failing.

Also works on plain lists — `pluck "[2]"` grabs the 3rd item. Prefer `first`
for the first item.

#### `keys` / `values`

Query a variable holding a JSON object — either a map literal or JSON
captured from a command, same sources as `pluck`:

```yaml
- set:
    - FIELDS: "{{ .RESP | keys }}" # sorted JSON array of top-level keys
    - VALUES: "{{ .RESP | values }}" # values in that same sorted-key order
```

Results are JSON-array strings, so they chain into other filters:
`{{ .RESP | keys | first }}`. Invalid JSON, or a value that isn't a JSON
object, is an error.

A field value that's a single variable reference (optionally with a pipe
chain) can omit `{{ }}`:

```yaml
- get:
    - RELEASE:
        options: .VERSIONS
        default: .VERSIONS | first
```

---

## Steps

Every task is a sequence of five step types.

### `set` — assign variables

```yaml
- set:
    - TARGET_HOST: localhost
    - APPLICATION_KEY:
        value: .VAULT_TOKEN
        secret: true # masked in terminal output
    - REGION_MAP: { us: us-east-1, eu: eu-west-1 } # map literal -> JSON object
```

A map literal is stored as a JSON object string (same trick lists use —
see [Template filters](#template-filters)), queried with `pluck`/`keys`/
`values` or iterated with `loop:`.

### `run` — shell commands

```yaml
- run: git rev-parse --short HEAD
  dir: ./infra # working directory for this step
  into:
    - GIT_SHA: stdout | trim
    - ERROR_LOG: stderr
```

> Unbuffered Python: scripts buffer stdout when not attached to a terminal,
> so `run:` output can appear late or all at once. Fix with
> `PYTHONUNBUFFERED: 1` in `vars:`, or `python -u` per script. See
> [Python `-u` docs](https://docs.python.org/3/using/cmdline.html#cmdoption-u).

### `get` — interactive prompts

Prompts for input; skipped if the variable already exists in scope.

> With `--no-input`, a `CI` env var, or non-terminal stdin (AI agents, other
> non-interactive callers), prompts are skipped. Missing vars with no
> `default` abort.

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

Runs another task in an isolated scope. Pass vars in with `with:`, pull
results back with `into:`.

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

A non-zero exit halts the timeline by default. `soft: true` continues past
a failed call:

```yaml
- call: flaky_cleanup_script
  soft: true
- call: next_critical_step
```

### `loop` — iteration

List form — iterates a sequence, current element as `{{.ITEM}}`:

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

Map form — when the variable resolves to a JSON object instead of an array,
iterates its entries in sorted-key order as `{{.KEY}}` / `{{.VALUE}}`:

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

Runs in the inherited task `dir:`, even on a step with its own `dir:`
override — see [`dir:`](#dir--working-directory) above.

---

## Best practices

- **Task names** — kebab-case (`deploy-production`, not `deploy_production`).
- **Variable names** — `ALL_CAPS` with underscores.
- **Field ordering** — `info:` before `steps:` in tasks; `info:` first in
  `get:` entries.
- **Prompt placement** — put `get:` steps early. Prompts buried after slow
  `run` steps make the user wait before they can answer.
