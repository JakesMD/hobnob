# hobnob guide

Hobnob is a timeline-based task runner. Variables, conditions, and prompts are
evaluated sequentially as execution reaches each step — not compiled into a
static dependency tree upfront.

---

## CLI

### Commands

```bash
hobnob                               # run the "default" task (or show help)
hobnob deploy                        # run a task
hobnob deploy ENV=production         # pass runtime variables
hobnob --file ops/tasks.yml build    # target a specific file
hobnob --list                        # list all public tasks
hobnob --help                        # show help and available tasks
hobnob deploy --no-input             # skip prompts, fail on missing vars
hobnob --version                     # print version and exit
hobnob --upgrade                     # upgrade to the latest release
```

### Default task

Name a task `default` in your hobnob file and it runs automatically when you
call `hobnob` with no arguments.

```yaml
tasks:
  default:
    steps:
      - run: echo "Hi!"
```

If no `default` task is defined, `hobnob` shows help instead.

### Auto-discovery

No `--file` flag? Hobnob searches up from your current directory for
`hobnob.yml` or `hobnob.yaml`. `.yml` wins if both exist in the same directory.

---

## File structure

A hobnob file has three optional top-level keys:

```yaml
vars: # global variables
  KEY: value

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

### Namespaces

Imported tasks are prefixed with their module key — `call: docker:build`.

The key prefix controls visibility:

- **No `_`** — module is exported. Its tasks appear in `--list` and are visible
  to any parent file that imports this file as a module.
- **`_` prefix** — module is internal. Its tasks are only accessible within this
  file and are hidden from `--list` and parent files.

### Inclusion filters

- `show` — whitelist. Only listed tasks are imported.
- `hide` — blacklist. Imports everything except listed tasks.

### Flattening

`flatten: true` registers tasks under both `docker:build` and `build`. Native
tasks always win on conflicts.

### Scoping rules

- Sub-modules inherit root `vars:` as read-only.
- A module's own `vars:` block never leaks to the parent.

---

## Tasks

A task is a named sequence of steps. The name prefix controls visibility:

- **No `_`** — task is public. Appears in `--list`, runnable from the CLI, and
  visible to parent files that import this file as a module.
- **`_` prefix** (e.g. `_compile`) — task is internal. Hidden from `--list`, not
  visible to parent files, only callable via `call`.

### `dir:` — working directory

A task's `dir:` sets the working directory for every `run` step in that task and
is **inherited down the call chain** — called tasks and their descendants all
run under it, unless they declare a `dir:` of their own.

```yaml
tasks:
  deploy:
    dir: ./infra
    steps:
      - run: terraform apply # runs in ./infra
      - call: _verify # _verify inherits ./infra unless it has its own dir:
```

A `call` step's `dir:` **overrides the called task's own `dir:`** and becomes
the inherited directory for everything that task calls in turn:

```yaml
- call: _verify
  dir: ./staging # _verify runs here, ignoring its own dir: if any
```

A `run` step's `dir:` overrides the active directory for that one step only:

```yaml
- run: go test ./...
  dir: ../tests # only this step runs in ../tests
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

Env is lowest priority because a variable name in `vars:` could collide with one
already set in the caller's shell. If env won, the same task could silently
behave differently on two machines depending on what's exported. Putting env at
the bottom makes tasks deterministic regardless of the ambient environment.

CLI args sit above env so callers can explicitly override env values when needed
— `hobnob deploy HOST=remotehost` is clear intent.

Global vars sit above CLI args because the `vars:` block is internal wiring —
hardcoded endpoints, derived paths, computed defaults. A caller silently
overriding those would make the task unpredictable.

The intended input mechanism for callers is `get:` steps. A `get:` prompt is
automatically skipped when its variable already exists in scope, so passing
`HOST=remotehost` on the CLI answers the prompt without requiring interaction.

</details>

Use `{{ .VAR | default "fallback" }}` to let a lower-priority layer supply the
value when a higher-priority one isn't set:

```yaml
vars:
  HOST: "{{ .HOST | default "localhost" }}"
```

Or in a `set` step:

```yaml
- set:
    - HOST: "{{ .HOST | default "localhost" }}"
```

### Scope isolation

`call` gives the child task a deep copy of the current scope. It can read
everything, but mutations stay sandboxed unless you pull them back with `into:`.

### Top-to-bottom resolution

Variables in a `set` block resolve top-to-bottom, so you can reference a key
defined just above:

```yaml
- set:
    - BASE_URL: https://api.example.com
    - AUTH_URL: "{{.BASE_URL}}/v1/auth"
```

### Built-in variables

Two variables are automatically injected into every task's scope:

- `HOBNOB_FILE_DIR` — directory containing the hobnob file.
- `HOBNOB_INVOCATION_DIR` — directory from which hobnob was run.

### Template filters

`default`, `trim`, `upper`, `lower`, `lines`, `split` — usable anywhere
templates are supported.

```yaml
- run: echo "Targeting {{.ENV | upper}}"
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

> ⚠️ With `--no-input` or a `CI` env var set, prompts are skipped. Missing
> variables with no `default` will abort.

Bare form — prompts for a value with no configuration:

```yaml
- get: [MY_VAR]
```

Object form — full configuration:

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

By default, a non-zero exit halts the timeline. Use `soft: true` to let
execution continue past a failed call:

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

### Task naming

Use kebab-case for task names.

```yaml
# bad
tasks:
  deploy_production:

# good
tasks:
  deploy-production:
```

### Prompt placement

Put `get:` steps as early as possible. Prompts buried after slow `run` steps
make the user wait before they can answer.

```yaml
# bad — user waits for the build before being prompted
tasks:
  publish:
    steps:
      - run: ./build.sh
      - get:
          - ENV:
              options: [staging, production]
      - run: ./deploy.sh {{.ENV}}

# good — all prompts first, then execution
tasks:
  publish:
    steps:
      - get:
          - ENV:
              options: [staging, production]
      - run: ./build.sh
      - run: ./deploy.sh {{.ENV}}
```
