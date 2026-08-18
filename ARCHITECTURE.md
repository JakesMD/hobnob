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
internal/eval/    Go template rendering, shell conditions, JSON filters
internal/cli/     scope construction, --list/--help formatting, completions
internal/runner/  step execution (the interpreter loop)
internal/tui/     bubbletea prompts, styled terminal output
```

Dependency direction is one-way: `runner` depends on `config`, `eval`, `cli`,
`tui`; none of those import `runner` back. `config` and `eval` don't depend on
each other or on anything above them.

### `internal/config` — parsing

`ParseConfig` walks a `yaml.Node` tree into a `ConfigFile`: tasks, global vars,
module imports, env-file references.

Every field that can contain a `{{ }}` template is stored as a raw string —
**nothing is evaluated at parse time**, only at runtime once the scope exists.
This is the load-bearing invariant of the whole system.

Parsing is split by step kind: `config.go` (root types, `ParseConfig`,
module/env field parsing), `steps.go` (per-step-kind dispatch, loop parsing),
`vars.go` (`set:`/`with:`/`vars:` entries, `into:`), `get.go` (`get:` entries).
`modules.go`/`envfiles.go` handle _loading_ rather than parsing — resolving and
merging imported files, run after `BuildScope` since their paths can be
templated. `collect.go` statically walks parsed steps to find the `get:` params
a task would prompt for, without running it (powers `--list`'s param hints).

### `internal/eval` — template and shell evaluation

Three primitives everything else is built from:

- `EvalTemplate(tmpl, vars)` — renders `{{ .VAR }}` via Go's `text/template`
  plus hobnob's filters (`default`, `trim`, `split`, `pluck`, `keys`, ...). The
  filter `FuncMap` is built once at package init since this runs on nearly every
  step.
- `EvalCondition(expr, vars, dir)` — renders then runs via `sh -c`, exit code 0
  = true. Backs `if:`.
- `EvalRunIntoPipe(expr, stdout, stderr)` — the `run: ... into:` pipe syntax
  (`stdout | trim`, `stderr | lines`).

JSON is hobnob's native data shape: lists and map literals in YAML are stored as
JSON-string scope values, and `pluck`/`keys`/`values`/`loop:` all operate on
that encoding. `pluck` uses RFC 9535 JSONPath.

### `internal/cli` — scope and presentation

`Scope` is `{Vars map[string]string, Secrets map[string]bool}` — the variable
environment a task executes in. `BuildScope` layers it in strict precedence
order (env → system vars → env-file vars → CLI args → global `vars:`) before any
task runs. `Scope.Copy()` deep-copies both maps, giving every `call:` step an
isolated sandbox.

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
- **`loop:`** — list/map/matrix forms dispatch to `execForList`/
  `execForMap`/`execForMatrix`, setting loop vars (`ITEM`, `KEY`/`VALUE`, or
  matrix vars) and restoring prior values on exit.

`runner_unix.go`/`runner_windows.go` handle process-group signaling: 1st CTRL+C
sends a graceful signal to the whole group (catching child processes a
single-command signal would miss), 2nd force-kills.

### `internal/tui`

Bubbletea/Bubbles-based `PromptText` and `PromptSelect`, plus lipgloss styles
used for task output (`SLabel`, `SInfo`, `SError`, ...) and line prefixing so
concurrent-looking output from `run:` steps stays attributable to the task that
produced it.

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
