# Architecture

Hobnob is a single Go binary: a YAML task runner built around a timeline
execution model rather than a static dependency graph. Variables, conditions,
and prompts resolve sequentially as execution reaches each step.

## Setup

```bash
go build -o hobnob ./cmd/hobnob   # or: hobnob go:build
go test ./...                     # or: hobnob go:test
go vet ./...                      # or: hobnob go:vet
```

Requires Go 1.24+. No external services, no codegen, no build step beyond
`go build`. The project self-hosts its own dev tasks in `hobnob.yml` — see
[CONTRIBUTING.md](CONTRIBUTING.md).

Dependencies: `gopkg.in/yaml.v3` (parsing), `github.com/theory/jsonpath` (the
`pluck` filter), `github.com/charmbracelet/{bubbletea,bubbles,lipgloss}`
(interactive prompts and styled output).

## Package layout

```
cmd/hobnob/       entry point, CLI dispatch, self-upgrade
internal/config/  YAML → typed structs (parse time)
internal/eval/    Go template rendering, shell conditions, typed evaluation
internal/value/   the typed scope value (string/bool/number/array/object) and its filters
internal/cli/     scope construction, --list/--help formatting, completions
internal/runner/  step execution (the interpreter loop)
internal/tui/     bubbletea prompts, styled terminal output
```

Dependency direction is one-way: `runner` depends on `config`, `eval`, `cli`,
`tui`; none of those import `runner` back. `value` sits at the bottom with zero
internal dependencies; `eval` and `config` both import it (and `config` imports
`eval`, never the reverse) but don't depend on each other otherwise. `tui`
imports `value` (its prompts return typed results) but not `eval` —
`PromptText` takes a validator closure instead of evaluating a `check:`
expression itself, so `tui` never needs to know how one is evaluated.

### `internal/config` — parsing

`ParseConfig` walks a `yaml.Node` tree into a `ConfigFile`: tasks, global vars,
module imports, env-file references.

Every field that can contain a `{{ }}` template is stored as a raw string —
**nothing is evaluated at parse time**, only at runtime once the scope exists.
This is the load-bearing invariant of the whole system.

Parsing is split by step kind: `config.go` (`ParseConfig`, task/step-sequence
parsing), `types.go` (the `ConfigFile`/`Task`/`Step`/... structs), `yaml.go`
(shared `yaml.Node` helpers), `steps.go` (per-step-kind dispatch, loop
parsing), `vars.go` (`set:`/`with:`/`vars:` entries, `into:`), `get.go`
(`get:` entries); `modules.go`/`envfiles.go` each also parse their own block
(`modules:`/`env:`) alongside the loading logic below.
`modules.go`/`envfiles.go` handle _loading_ rather than parsing — resolving and
merging imported files, run after `BuildScope` since their paths can be
templated. `collect.go` statically walks parsed steps to find the `get:` params
a task would prompt for, without running it (powers `--list`'s param hints).

### `internal/eval` — template and shell evaluation

Four primitives everything else is built from:

- `EvalTemplate(tmpl, vars)` — renders `{{ .VAR }}` to a **string** via Go's
  `text/template` plus hobnob's filters (`default`, `trim`, `split`, `pluck`,
  `keys`, ...), adapted from the single registry in `internal/value` at
  package init since this runs on nearly every step.
- `EvalValue(expr, vars)` — the type-preserving counterpart: when `expr` is
  exactly one template action (a bare `.VAR`, optionally through a filter
  chain), it walks the parsed template's tree and evaluates it directly
  against `value.Filters` instead of executing it, so the result keeps its
  `value.Value` kind (Array, Object, ...) rather than flattening to text.
  Anything else falls back to `EvalTemplate` and comes back wrapped in a
  String. Backs `set:`/`with:`/`vars:` scalar leaves and `loop:`'s target.
- `EvalCondition(ctx, expr, vars, dir)` — renders then runs via `sh -c` with
  `exec.CommandContext`, exit code 0 = true. Backs `if:`/`check:`; ctx
  cancellation (CTRL+C) kills a hung condition outright.
- `EvalRunIntoPipe(expr, stdout, stderr)` — the `run: ... into:` pipe syntax
  (`stdout | trim`, `stderr | lines`). Captures stdout/stderr into a
  `value.Value` via `value.Capture` (structure only if it decodes cleanly as a
  JSON array/object), then runs any filter chain typed from there.

A scope variable is a typed `value.Value` — string, bool, number, array, or
object — never JSON-as-text. Structure enters from exactly three places:
`set:`/`with:`/`vars:` map/list literals, `run: into:` capture, and the
explicit `json` filter; env vars, CLI args, and env-file values are always
strings, never sniffed. `pluck`/`keys`/`values`/`first` require an
Array/Object and error (naming `| json`) rather than silently re-parsing a
string — see `internal/value/filter.go`. `pluck` uses RFC 9535 JSONPath.

### `internal/cli` — scope and presentation

`Scope` is `{Vars map[string]value.Value, Secrets map[string]bool}` — the
variable environment a task executes in. `BuildScope` layers it in strict
precedence order (env → system vars → env-file vars → CLI args → global
`vars:`) before any task runs; every source but `vars:` is wrapped in
`value.Str`, never sniffed for JSON shape. `Scope.Copy()` deep-copies both
maps, giving every `call:` step an isolated sandbox.

Secrecy is a property of where a value came from, not of where it's used:
`Secrets` rides along on `Copy()`, and `runner.maskSecrets` matches on _value_
rather than key. A secret therefore stays masked when a `with:` entry passes it
into a child under a different name, which is why `secret:` on a `with:` entry
is rejected at parse time (`config.rejectSecretCallVars`) instead of honored —
it would be redundant at best, and would over-mask a composed value at worst.

Everything else here is output formatting: `--list`/`--help` rendering,
task-selector data, and the zsh/bash/fish completion scripts (`go:embed`-ed from
`internal/cli/completions/`).

### `internal/runner` — the interpreter

`ExecuteTask` → `executeSteps` is the core loop: iterate a task's `[]Step`,
dispatch on `StepKind`, check `if:` before each step, evaluate templates against
the _current_ scope (which earlier steps may have just mutated).

State threaded through the call graph (ctx, config, task name, prompt flag,
working dir) is bundled into a small `execCtx` struct. `*cli.Scope` stays a
separate argument since it's the thing being mutated — a `call:` step swaps in a
fresh child scope while everything else in `execCtx` carries forward.

Step kinds, briefly:

- **`run:`** — execs `sh -c` in the step's resolved `dir:`, streams
  stdout/stderr live, optionally captures both via `into:`. Scope vars override
  same-named vars in the inherited environment.
- **`set:`** — evaluates each template top-to-bottom into scope.
- **`get:`** — no-ops if the var is already in scope (how CLI `KEY=VALUE` args
  satisfy prompts). Otherwise uses `default:`, fails, or prompts via
  `tui.PromptText`/`PromptSelect`.
- **`call:`** — deep-copies scope, applies `with:`, runs the target task, pulls
  results back via `into:`.
- **`loop:`** — dispatches on the target's `value.Kind()` (Object → map form,
  else list form) rather than sniffing text, then runs
  `execForList`/`execForMap`/`execForMatrix`, setting loop vars (`ITEM`,
  `KEY`/`VALUE`, or matrix vars, each still typed) and restoring prior values
  on exit.

Split by step kind — `run.go`, `set.go`, `get.go`, `call.go`, `loop.go` —
mirroring `internal/config`'s convention; `runner.go` keeps only the shared
core (`execCtx`, `ExecuteTask`/`executeSteps`, dir/mask helpers).
`runner_unix.go`/`runner_windows.go` handle process-group signaling: 1st
CTRL+C sends a graceful signal to the whole group (catching child processes a
single-command signal would miss), 2nd force-kills.

### `internal/tui`

Split by widget — `linewriter.go` (the `run:` stdout/stderr line-prefixing
writer), `prompt_text.go`/`prompt_select.go`/`prompt_taskselect.go` (the three
Bubbletea models), `styles.go` (shared lipgloss styles like `SLabel`, `SInfo`,
`SError`, ... plus `TaskPrefix`/`SecretMask`). `PromptText` and `PromptSelect`
are the two entry points the runner calls.

## Data flow, end to end

```
hobnob.yml
   │  ParseConfig (config.go)          — YAML → ConfigFile, all templates raw
   ▼
ConfigFile
   │  BuildScope (cli.go)              — env → sysvars → env-files → CLI args → vars:
   │  LoadModules (modules.go)         — needs scope for templated module paths
   ▼
Scope{Vars, Secrets}
   │  ExecuteTask → executeSteps (runner.go)
   │    each step: EvalTemplate/EvalCondition against *current* scope
   │    mutates scope.Vars as it goes (set/get/run into/call into)
   ▼
process exit code
```

## Testing conventions

Table-driven `t.Run` subtests per package, `promptTextFn`/`promptSelectFn`
(runner) and `isTerminalFn` (main) swapped for fakes rather than mocking the
terminal. See [CONTRIBUTING.md](CONTRIBUTING.md) for naming/structure
conventions (Given/When/Then/Why, Arrange/Act/Assert).

## See also

- [GUIDE.md](GUIDE.md) — user-facing reference: every YAML field, template
  filter, and CLI flag.
- [CLAUDE.md](CLAUDE.md) — condensed version of this file plus repo-specific
  invariants, aimed at AI coding agents.
- [CONTRIBUTING.md](CONTRIBUTING.md) — PR checklist, testing conventions.
