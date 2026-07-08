# hobnob guide

Hobnob is a timeline-based task runner. Variables, conditions, and prompts are
evaluated sequentially as execution reaches each step — not compiled into a
static dependency tree upfront.

---

## Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

Detects OS/architecture, installs to `~/.local/bin`, configures shell completion
(bash, zsh, fish).

---

## CLI

```bash
hobnob                               # run the "default" task (or select)
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

**Default task** — name a task `default` and it runs when you call `hobnob` with
no arguments. Without one, an interactive selector opens instead. Use `--select`
to force the selector even when a default exists.

**Auto-discovery** — no `--file`? Hobnob searches up from your current directory
for `hobnob.yml` or `hobnob.yaml`. `.yml` wins if both exist in the same
directory.

**Stopping a run** — CTRL+C (or `SIGTERM`) starts a graceful shutdown: the
currently running `run:` step is sent a termination signal and hobnob waits for
it to exit on its own, however long that takes; no further steps start. Press
CTRL+C again (or send a 2nd signal) to force-kill the running step immediately
instead of waiting.

---

## File structure

A hobnob file has three optional top-level keys:

```yaml
vars: # global variables
  - KEY: value

modules: # imported hobnob files
  - utils: ./utils.yml

tasks: # task definitions
  my-task:
    steps:
      - run: echo hello
```

---

## Modules

Import tasks from other files with the root-level `modules:` block.

```yaml
modules:
  - _infra: terraform.yml
  - docker:
      path: ./docker/tasks.yml
      show: [build, push]
      flatten: true
```

**Namespaces** — imported tasks are prefixed with their module key
(`call:
docker:build`). A `_` prefix makes the module internal — its tasks are
hidden from `--list` and parent files.

**Filters** — `show:` whitelists, `hide:` blacklists which tasks to import.

**Flattening** — `flatten: true` registers tasks under both `docker:build` and
`build`. Native tasks always win on conflicts.

**Scoping** — sub-modules inherit root `vars:` as read-only. A module's own
`vars:` block never leaks to the parent.

---

## Tasks

A task is a named sequence of steps. A `_` prefix (e.g. `_compile`) makes the
task internal — hidden from `--list` and parent files, only callable via `call`.

### `if:` — conditional execution

Skips the entire task when the shell condition exits non-zero. When a called
task is skipped, execution continues in the caller — no error.

```yaml
tasks:
  deploy:
    if: '[ "{{.ENV}}" = "production" ]'
    steps:
      - run: ./deploy.sh
```

### `interactive:` — disable prompts

`interactive: false` disables all `get:` prompts for the entire sub-tree —
prompts with a `default:` use it automatically, required prompts without a
default abort with an error. Once disabled, a called task cannot re-enable them.

Works on tasks and on `call` steps:

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

Paths are always resolved relative to the hobnob file. When no `dir:` is set
anywhere, steps run in the hobnob file's directory.

- **Task `dir:`** — sets the working directory for all `run` steps and is
  inherited down the call chain, unless a descendant declares its own.
- **Call step `dir:`** — overrides the called task's own `dir:` and becomes the
  inherited directory for its descendants.
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

---

## Variables

Evaluated at runtime using Go templates (`{{ .VAR }}`).

### Precedence (highest to lowest)

| Priority | Scope     | Description                               |
| -------- | --------- | ----------------------------------------- |
| **1**    | Local     | Set via `set`, `get`, or loop iterators.  |
| **2**    | Passed    | Injected explicitly via `call`'s `with:`. |
| **3**    | Inherited | Copied from the calling task.             |
| **4**    | Global    | Declared in the root `vars:` block.       |
| **5**    | CLI args  | `KEY=VALUE` args on the command line.     |
| **6**    | Env       | OS environment variables.                 |

<details>
<summary>Rationale</summary>

Env is lowest because a var name in `vars:` could collide with one already in
the caller's shell — env winning would make tasks behave differently across
machines. CLI args sit above env so callers can explicitly override. Global vars
sit above CLI args because `vars:` is internal wiring — silently overriding it
would make tasks unpredictable. The intended input mechanism for callers is
`get:` steps, which are automatically skipped when their variable already exists
in scope.

</details>

Use `{{ .VAR | default "fallback" }}` to let a lower-priority layer supply the
value when a higher-priority one isn't set:

```yaml
vars:
  - HOST: "{{ .HOST | default "localhost" }}"
```

### Scope isolation

`call` gives the child task a deep copy of the current scope. Mutations stay
sandboxed unless pulled back with `into:`.

### Top-to-bottom resolution

Variables in a `set` block resolve top-to-bottom — you can reference a key
defined just above:

```yaml
- set:
    - BASE_URL: https://api.example.com
    - AUTH_URL: "{{.BASE_URL}}/v1/auth"
```

### Built-in variables

- `HOBNOB_FILE_DIR` — directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR` — directory from which hobnob was run.

### Template filters

`default`, `trim`, `upper`, `lower`, `lines`, `split`, `first` — usable anywhere
templates are supported.

```yaml
- run: echo "Targeting {{.ENV | upper}}"
```

Field values that are a single variable reference — optionally with a pipe chain
— can omit `{{ }}`:

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
```

### `run` — shell commands

```yaml
- run: git rev-parse --short HEAD
  dir: ./infra # working directory for this step
  into:
    - GIT_SHA: stdout | trim
    - ERROR_LOG: stderr
```

### `get` — interactive prompts

Prompts for input. Skipped if the variable already exists in scope.

> With `--no-input` or a `CI` env var set, prompts are skipped. Missing
> variables with no `default` will abort.

Bare form — prompts with no configuration:

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

Runs another task in an isolated scope. Pass variables in with `with:`, pull
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

By default, a non-zero exit halts the timeline. Use `soft: true` to continue
past a failed call:

```yaml
- call: flaky_cleanup_script
  soft: true
- call: next_critical_step
```

### `loop` — iteration

**List form** — iterates a sequence, current element available as `{{.ITEM}}`:

```yaml
- loop: .GO_FILES
  steps:
    - run: go fmt {{.ITEM}}
```

**Matrix form** — runs every combination of the given arrays:

```yaml
- loop:
    OS: [linux, darwin]
    ARCH: [amd64, arm64]
  steps:
    - run: echo "Compiling for {{.OS}} on {{.ARCH}}"
```

### `if:` — conditional steps

Any step can be conditionally skipped. Exit `0` proceeds, non-zero skips.

```yaml
- run: echo "Purging production cache..."
  if: '[ "{{.ENV}}" = "production" ]'
```

---

## Best practices

- **Task names** — use kebab-case (`deploy-production`, not
  `deploy_production`).
- **Variable names** — use `ALL_CAPS` with underscores.
- **Field ordering** — put `info:` before `steps:` in tasks, and `info:` first
  in `get:` entries.
- **Prompt placement** — put `get:` steps as early as possible. Prompts buried
  after slow `run` steps make the user wait before they can answer.
