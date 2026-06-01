# The hobnob guide

Hobnob is a timeline-based task runner. Unlike tools that build static
dependency trees upfront, Hobnob evaluates variables, checks conditions, and
processes inputs sequentially at the exact millisecond execution reaches that
step.

---

## 1. Core Mechanics & CLI

### Auto-Discovery

Running `hobnob` without a `--file` flag triggers a bottom-up directory search:

1. Searches the **current working directory** for `hobnob.yml` or `hobnob.yaml`.
2. Walks up **parent directories** to the filesystem root until a match is
   found.
3. If both extensions exist in the same directory, `hobnob.yml` takes priority.

### Command Reference

```bash
# Execute a public task
hobnob deploy

# Pass runtime variables (Highest priority)
hobnob deploy ENV=production TIMEOUT=60

# Target a specific configuration file
hobnob --file ops/tasks.yml build

# List all available public tasks
hobnob --list

# Force non-interactive mode (skips prompts, fails on missing required vars)
hobnob deploy --no-input
```

### Public vs. Internal Tasks

- **Public Tasks:** Standard named blocks. Visible in `hobnob --list` and
  executable via CLI.
- **Internal Tasks:** Names prefixed with an underscore (e.g., `_compile`).
  Hidden from `--list`, cannot be run from CLI, and must be executed via a
  `call` step.

---

## 2. Variable Scope & Precedence

Variables are evaluated dynamically at runtime using Go templates
(`{{ .VAR }}`).

### Precedence Hierarchy (Highest to Lowest)

| Priority | Scope                 | Description                                                      |
| -------- | --------------------- | ---------------------------------------------------------------- |
| **1**    | **Local Scope**       | Mutated inline via `set`, `get`, or loop iterators.              |
| **2**    | **Passed Parameters** | Explicitly injected via a `call` step's `with:` list.            |
| **3**    | **Inherited Scope**   | Copied over from the calling parent task.                        |
| **4**    | **Global Block**      | Declared in the root-level `vars:` block.                        |
| **5**    | **Environment / CLI** | Host OS environment variables or trailing `KEY=VALUE` arguments. |

### Scope Isolation

When Task A invokes Task B via `call`, Task B receives a **deep copy**
(Read-Copy) of Task A's scope.

- Task B can read any variable from Task A.
- Mutations (`set`, `get`) inside Task B are completely sandboxed and do not
  modify Task A unless explicitly returned using the `into:` list.

### Top-to-Bottom Resolution

Within blocks defining multiple variables, assignments resolve sequentially. A
variable can immediately reference a key declared right above it:

```yaml
- set:
    - BASE_URL: "https://api.example.com"
    - AUTH_URL: "{{.BASE_URL}}/v1/auth" # Valid: References item above
```

### Template Filters

Standard pipeline filters can be used anywhere templates are supported:
`default`, `trim`, `upper`, `lower`, `lines`, `split`.

```yaml
- run: echo "Targeting {{.ENV | upper}}"
```

---

## 3. Step Reference (The 5 Verbs)

Every task timeline is constructed using five explicit sequential actions.

### `set` — Data Assignment

Assigns or calculates local variables. Supports an expanded form for masking
sensitive terminal logs.

```yaml
- set:
    - TARGET_HOST: "localhost"
    - APPLICATION_KEY:
        value: "{{.VAULT_TOKEN}}"
        secret: true
```

### `run` — Shell Execution

Spawns an OS shell command. Text can be extracted from standard streams into
scope variables using the `into:` list.

```yaml
- run: git rev-parse --short HEAD
  into:
    - GIT_SHA: stdout | trim
    - ERROR_LOG: stderr
```

### `get` — Interactive Prompts

Pauses execution for user input. **If the variable already exists in the current
scope, the prompt is skipped automatically.**

> ⚠️ **Non-Interactive Mode:** If `--no-input` is passed or a `CI` environment
> variable is present, prompts are skipped. If a variable is missing and lacks a
> `default`, execution aborts.

```yaml
- get:
    - PORT:
        info: "Enter deployment port"
        default: "8080"
        check: "[ {{.PORT}} -gt 1024 ]" # Must exit 0 to pass validation
    - ENVIRONMENT:
        info: "Select target stage"
        options: ["staging", "production"]
```

### `call` — Sub-routines

Invokes another task within a deep-copied child scope. Input parameters and
output returns are passed as explicit lists.

```yaml
- call: "deploy_pipeline"
  with:
    - TARGET_ENV: "production"
    - TIMEOUT_SECS: "90"
  into:
    - DEPLOY_STATUS: STATUS
    - ARTIFACT_PATH: LOG_FILE
```

### `for` — Loops

Iterates over lists or matrices sequentially.

#### List Form

Iterates through a sequence. The current element maps to `{{.ITEM}}`.

```yaml
- loop: .GO_FILES
  steps:
    - run: go fmt {{.ITEM}}
```

#### Matrix Form (Cartesian Product)

Specifying multiple arrays runs every parameter combination.

```yaml
- loop:
    OS: ["linux", "darwin"]
    ARCH: ["amd64", "arm64"]
  steps:
    - run: echo "Compiling for {{.OS}} on {{.ARCH}}"
```

---

## 4. Control Flow & Error Handling

### Step Conditionals (`if:`)

Every individual step supports an inline `if:` modifier. It runs a low-level
shell command; a `0` exit code allows the step to run, while any non-zero code
skips it.

```yaml
- run: echo "Purging production cache..."
  if: '[ "{{.ENV}}" = "production" ]'
```

### Failure Mitigation (`soft:`)

By default, any command returning a non-zero exit code halts the entire
timeline. To allow execution to proceed past a failing step, append `soft: true`
to a `call`.

```yaml
- call: "flaky_cleanup_script"
  soft: true # Timeline continues even if this task crashes
- call: "next_critical_step"
```

---

## 5. Sub-Modules

Break down monolithic configurations by importing external Hobnob files in the
root `modules:` block.

```yaml
modules:
  - _infra: terraform.yml # Internal namespace
  - docker:
      path: "./docker/tasks.yml"
      show: ["build", "push"]
      flatten: true
```

### Prefix Routing & Namespaces

- External tasks are normally namespaced via their module key (e.g.,
  `call: docker:build`).
- If a module key begins with an underscore (e.g., `_infra`), the entire module
  becomes internal, hiding all its tasks from `--list` and the CLI.

### Inclusion Filters

- `show`: An explicit whitelist. Only these tasks are imported.
- `hide`: A blacklist. Imports everything _except_ these tasks.

### Namespace Flattening

When `flatten: true` is set, tasks are registered under **both** their prefixed
identifier (`docker:build`) and their short name (`build`). If a short name
conflicts with a native task in the host file, the native task takes precedence.

### Module Scoping Rules

1. **One-Way Global Flow:** Sub-modules inherit global variables from the root
   file as read-only context.
2. **Isolated Module Vars:** A module's root-level `vars:` block is internal to
   that file and never leaks back to the parent.
3. **Sandbox Restriction:** Tasks within an imported module can call sibling
   tasks inside their own file, but they cannot invoke tasks belonging to the
   parent host file.
