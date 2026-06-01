# hobnob guide

Hobnob is a timeline-based task runner. Variables, conditions, and prompts are evaluated sequentially as execution reaches each step — not compiled into a static dependency tree upfront.

---

## CLI

### Auto-discovery

No `--file` flag? Hobnob searches up from your current directory for `hobnob.yml` or `hobnob.yaml`. `.yml` wins if both exist in the same directory.

### Commands

```bash
hobnob deploy                        # run a task
hobnob deploy ENV=production         # pass runtime variables
hobnob --file ops/tasks.yml build    # target a specific file
hobnob --list                        # list all public tasks
hobnob deploy --no-input             # skip prompts, fail on missing vars
```

### Public vs. internal tasks

- **Public** — standard named tasks, visible in `--list` and runnable from CLI.
- **Internal** — prefix with `_` (e.g. `_compile`). Hidden from `--list`, only callable via `call`.

---

## Variables

Evaluated at runtime using Go templates (`{{ .VAR }}`).

### Precedence (highest to lowest)

| Priority | Scope | Description |
| --- | --- | --- |
| **1** | Local | Set via `set`, `get`, or loop iterators. |
| **2** | Passed | Injected explicitly via `call`'s `with:`. |
| **3** | Inherited | Copied from the calling task. |
| **4** | Global | Declared in the root `vars:` block. |
| **5** | Env / CLI | OS environment or `KEY=VALUE` args. |

### Scope isolation

`call` gives the child task a deep copy of the current scope. It can read everything, but mutations stay sandboxed unless you pull them back with `into:`.

### Top-to-bottom resolution

Variables in a `set` block resolve top-to-bottom, so you can reference a key defined just above:

```yaml
- set:
    - BASE_URL: "https://api.example.com"
    - AUTH_URL: "{{.BASE_URL}}/v1/auth"
```

### Template filters

`default`, `trim`, `upper`, `lower`, `lines`, `split` — usable anywhere templates are supported.

```yaml
- run: echo "Targeting {{.ENV | upper}}"
```

---

## Steps

Every task is a sequence of five step types.

### `set` — assign variables

```yaml
- set:
    - TARGET_HOST: "localhost"
    - APPLICATION_KEY:
        value: "{{.VAULT_TOKEN}}"
        secret: true    # masked in terminal output
```

### `run` — shell commands

```yaml
- run: git rev-parse --short HEAD
  into:
    - GIT_SHA: stdout | trim
    - ERROR_LOG: stderr
```

### `get` — interactive prompts

Prompts for input. Skipped if the variable already exists in scope.

> ⚠️ With `--no-input` or a `CI` env var set, prompts are skipped. Missing variables with no `default` will abort.

```yaml
- get:
    - PORT:
        info: "Enter deployment port"
        default: "8080"
        check: "[ {{.PORT}} -gt 1024 ]"    # must exit 0 to pass
    - ENVIRONMENT:
        info: "Select target stage"
        options: ["staging", "production"]
```

### `call` — sub-tasks

Runs another task in an isolated scope. Pass variables in with `with:`, pull results back with `into:`.

```yaml
- call: "deploy_pipeline"
  with:
    - TARGET_ENV: "production"
    - TIMEOUT_SECS: "90"
  into:
    - DEPLOY_STATUS: STATUS
    - ARTIFACT_PATH: LOG_FILE
```

### `loop` — loops

**List form** — iterates a sequence, current element available as `{{.ITEM}}`:

```yaml
- loop: .GO_FILES
  steps:
    - run: go fmt {{.ITEM}}
```

**Matrix form** — runs every combination of the given arrays:

```yaml
- loop:
    OS: ["linux", "darwin"]
    ARCH: ["amd64", "arm64"]
  steps:
    - run: echo "Compiling for {{.OS}} on {{.ARCH}}"
```

---

## Control flow

### `if:` conditionals

Any step can be conditionally skipped. Exit `0` proceeds, non-zero skips.

```yaml
- run: echo "Purging production cache..."
  if: '[ "{{.ENV}}" = "production" ]'
```

### `soft:` failure handling

By default, a non-zero exit halts the timeline. `soft: true` on a `call` lets execution continue past failures.

```yaml
- call: "flaky_cleanup_script"
  soft: true
- call: "next_critical_step"
```

---

## Modules

Import tasks from other files with the root-level `modules:` block.

```yaml
modules:
  - _infra: terraform.yml
  - docker:
      path: "./docker/tasks.yml"
      show: ["build", "push"]
      flatten: true
```

### Namespaces

Imported tasks are prefixed with their module key — `call: docker:build`. Prefix the key with `_` to make the entire module internal.

### Inclusion filters

- `show` — whitelist. Only listed tasks are imported.
- `hide` — blacklist. Imports everything except listed tasks.

### Flattening

`flatten: true` registers tasks under both `docker:build` and `build`. Native tasks always win on conflicts.

### Scoping rules

- Sub-modules inherit root `vars:` as read-only.
- A module's own `vars:` block never leaks to the parent.
- Module tasks can call siblings within the same file, but not tasks in the parent.
