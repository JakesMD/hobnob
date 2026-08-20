# Architecture

Hobnob is a single Go binary: a YAML task runner built around a timeline
execution model rather than a static dependency graph. Variables, conditions,
and prompts resolve sequentially as execution reaches each step.

## Setup

```bash
go build -o hobnob ./cmd/hobnob   # or: hobnob go:build
go test ./...                     # or: hobnob go:test
go test ./internal/e2e            # or: hobnob go:test:e2e — end-to-end CLI suite only
go vet ./...                      # or: hobnob go:vet
```

Requires Go 1.24+. No external services, no codegen, no build step beyond
`go build`. The project self-hosts its own dev tasks in `hobnob.yml` — see
[CONTRIBUTING.md](CONTRIBUTING.md).

Dependencies: `gopkg.in/yaml.v3` (parsing),
`github.com/charmbracelet/{bubbletea,bubbles,lipgloss}` (interactive prompts
and styled output).

## Package layout

```
cmd/hobnob/       thin entry point: signal handling, os.Exit
internal/app/     CLI body — flag dispatch, App.Run(ctx, args)
internal/config/  YAML → typed structs (parse time)
internal/eval/    Go template rendering, shell conditions, typed evaluation
internal/value/   the typed scope value (string/bool/number/array/object) and its filters
internal/cli/     scope construction, --list/--help formatting, completions
internal/runner/  step execution (the interpreter loop)
internal/tui/     bubbletea prompts, styled terminal output
internal/e2e/     end-to-end CLI test suite (test-only package, no production code)
```

Dependency direction is one-way: `app` depends on `runner`, `config`, `cli`,
`tui`; `runner` depends on `config`, `eval`, `cli`, `tui`; none of those import
`app` or `runner` back. `value` sits at the bottom with zero internal
dependencies; `eval` and `config` both import it (and `config` imports `eval`,
never the reverse) but don't depend on each other otherwise. `tui` imports
`value` (its prompts return typed results) but not `eval` — `PromptText` takes
a validator closure instead of evaluating a `check:` expression itself, so
`tui` never needs to know how one is evaluated. `internal/e2e` imports `app`
and `runner` (for the prompt-fake seam) but nothing imports it back — it's a
leaf, `_test.go` files only.

### `internal/config` — parsing

`ParseConfig` walks a `yaml.Node` tree into a `ConfigFile`: tasks, module
imports, env-file references, `const:`/`vars:` entries. An unrecognized
top-level key is a load-time error, not a silent no-op.

Every field that can contain a `{{ }}` template is stored as a raw string —
**nothing is evaluated at parse time**, only at runtime once the scope exists.
This is the load-bearing invariant of the whole system. `const:`/`vars:`
entries are the one exception to "not evaluated" in spirit but not in fact:
their *values* still defer to `BuildScope`, same as everything else — only
their *reference structure* is inspected at parse time (`constvars.go`), to
enforce that a `const:` entry names only earlier `const:` entries and the two
builtins, and that a `vars:` entry doesn't reference itself.

Parsing is split by step kind: `config.go` (`ParseConfig`, task/step-sequence
parsing), `types.go` (the `ConfigFile`/`Task`/`Step`/... structs), `yaml.go`
(shared `yaml.Node` helpers), `steps.go` (per-step-kind dispatch, loop
parsing), `vars.go` (`set:`/`with:` entries, `into:`), `get.go`
(`get:` entries), `constvars.go` (the `const:`/`vars:` closed-world/
self-reference/reserved-name checks, run once per file after the whole tree
is parsed); `modules.go`/`envfiles.go` each also parse their own block
(`modules:`/`env:`) alongside the loading logic below.
`modules.go`/`envfiles.go` handle _loading_ rather than parsing — resolving and
merging imported files, run after `BuildScope` since their paths can be
templated. A module's own `env:`/`vars:`/`const:` are also folded into a
`ModuleLayer`/`ModuleConstLayer` delta on its `ConfigFile` here — see
`internal/runner` below for where that delta actually reaches a module task's
scope. `collect.go` statically walks parsed steps to find the `get:` params
a task would prompt for, without running it (powers `--list`'s param hints).

### `internal/eval` — template and shell evaluation

Four primitives everything else is built from:

- `EvalTemplate(tmpl, vars)` — renders `{{ .VAR }}` to a **string** via Go's
  `text/template` plus hobnob's filters (`default`, `trim`, `split`,
  `keys`, ...), adapted from the single registry in `internal/value` at
  package init since this runs on nearly every step. Before parsing,
  `rewriteAccessors` (`accessor.go`) rewrites any `.A.b[0][.KEY][1:3][*]`
  accessor chain into a call to hobnob's own `hbpath` template func — a
  source-to-source lexer pass, not a departure from `text/template`.
- `EvalValue(expr, vars)` — the type-preserving counterpart: when `expr` is
  exactly one template action (a bare `.VAR`, an accessor chain, optionally
  through a filter chain), it walks the parsed template's tree and evaluates
  it directly against `value.Filters`/`value.Path` instead of executing it,
  so the result keeps its `value.Value` kind (Array, Object, ...) rather than
  flattening to text. Anything else falls back to `EvalTemplate` and comes
  back wrapped in a String. Backs `set:`/`with:` scalar leaves and `loop:`'s
  target.
- `EvalCondition(ctx, expr, vars, dir)` — renders then runs via `sh -c` with
  `exec.CommandContext`, exit code 0 = true. Backs `if:`/`check:`; ctx
  cancellation (CTRL+C) kills a hung condition outright.
- `EvalRunIntoPipe(expr, stdout, stderr)` — the `run: ... into:` pipe syntax
  (`stdout | trim`, `stdout[0].name`, `stderr | lines`). Captures
  stdout/stderr into a `value.Value` via `value.Capture` (structure only if
  it decodes cleanly as a JSON array/object), then runs any accessor/filter
  chain typed from there.

A scope variable is a typed `value.Value` — string, bool, number, array, or
object — never JSON-as-text. Structure enters from exactly three places:
`set:`/`with:` map/list literals, `run: into:` capture, and the
explicit `json` filter; env vars, CLI args, and env-file values are always
strings, never sniffed. `keys` and an accessor step both
require an Array/Object and error (naming `| json`) rather than silently
re-parsing a string — see `internal/value/filter.go` and `internal/value/path.go`.
A missing accessor path is deferred instead: `value.Path` returns a sentinel
(`Value.IsMissing`) rather than an error, so a later `| default` in the same
pipeline can still catch it; every other consumer raises it as a real error
first.

### `internal/cli` — scope and presentation

`Scope` is `{Vars map[string]value.Value, Secrets map[string]bool, Ambient
map[string]bool}` — the variable environment a task executes in. `Ambient`
marks a key whose value still comes only from the OS-environment base layer;
it's cleared the moment any higher layer touches that key, and it's what lets
a module's own `env:`/`vars:` block tell a genuinely-inherited default from
something more specific the caller already set, without being able to see
which layer actually produced it. `BuildScope` layers it in strict precedence
order (env → system vars → `vars:` → env-file vars → CLI args → `const:`)
before any task runs; every source but `const:`/`vars:` is wrapped in
`value.Str`, never sniffed for JSON shape — `const:`/`vars:` route through
`config.EvalSetEntry` instead and stay typed. Above `const:`, precedence is
execution order, not a rule: a task's own `set:`/`get:`/`loop:`/`call:` steps
run afterward and see everything before them. `Scope.Set` always overwrites;
`Scope.SetIfDefault` only fills a key that's unset or still `Ambient` — how a
module's own `env:`/`vars:` supply a default without risking clobbering
something the caller already committed. `Scope.Copy()` deep-copies all three
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

- **`run:`** — a scalar execs `sh -c`; a YAML list (`Step.Argv`) resolves each
  element (typed, an Array element splicing into multiple arguments) and execs
  argv directly, no shell involved. Either way, runs in the step's resolved
  `dir:`, streams stdout/stderr live, optionally captures both via `into:`.
  Scope vars override same-named vars in the inherited environment.
- **`set:`** — evaluates each template top-to-bottom into scope.
- **`get:`** — no-ops if the var is already in scope (how CLI `KEY=VALUE` args
  satisfy prompts). Otherwise uses `default:`, fails, or prompts via
  `tui.PromptText`/`PromptSelect`.
- **`call:`** — deep-copies scope, applies `with:`, runs the target task, pulls
  results back via `into:`. If the target has `once: true`, it's memoized per
  invocation: keyed on the resolved `Task`'s `Steps` slice identity (not the
  name it was reached by, so a module task called via its qualified name and
  its bare name share one entry), a `callMemo` carried through `execCtx` —
  including across a `call:`'s own scope swap, which is what lets the cache
  replay into sibling sandboxes. The whole target child scope is cached, not
  a delta — each call site's own `into:` independently decides what it pulls
  out — and a cache hit is logged, naming what the first run produced.
  Without `once:`, a target re-runs on every `call:`, same as before `once:`
  existed. A task skipped by its own `if:` is cached as having produced
  nothing; a failure under `soft: true` is never cached.
- **`applyModuleLayer`** (not a step kind — runs inside `executeTask`) —
  whenever the resolved task belongs to a module (`Task.Cfg != nil`), writes
  that module's own `env:`/`vars:` layer (`Scope.SetIfDefault`, a default)
  and `const:` layer (`Scope.Set`, an override) into the scope the task is
  about to execute with. This is what makes a module's own `env:`/`const:`/
  `vars:` reach its tasks at all — `internal/config`'s loading step only ever
  computed the delta; nothing applied it to a live scope before this existed.
- **`loop:`** — dispatches on the target's `value.Kind()` (Object → map form,
  else list form) rather than sniffing text, then runs
  `execForList`/`execForMap`/`execForMatrix`, setting loop vars (`ITEM`,
  `KEY`/`VALUE`, or matrix vars, each still typed) and restoring prior values
  on exit.

Split by step kind — `run.go`, `set.go`, `get.go`, `call.go`, `use.go`,
`loop.go` — mirroring `internal/config`'s convention; `runner.go` keeps only
the shared core (`execCtx`, `ExecuteTask`/`executeTask`/`executeSteps`,
dir/mask helpers).
`runner_unix.go`/`runner_windows.go` handle process-group signaling: 1st
CTRL+C sends a graceful signal to the whole group (catching child processes a
single-command signal would miss), 2nd force-kills.

### `internal/app` — the CLI body

`App{Version, IsTerminal, SelectTask}` is the whole CLI: `IsTerminal` and
`SelectTask` (the interactive task picker) are struct fields rather than
package-level vars specifically so `internal/e2e` can substitute a fake
terminal/picker per test run without touching global state. `App.Run(ctx,
args)` does flag parsing, taskfile discovery (`findTaskfile` walks up
directories), `loadConfig` (parse + `BuildScope` + `LoadModules`), and routes
to `--list`/`--help`/`execTask`. It never reads `os.Args` or calls `os.Exit`,
so it's safe to invoke repeatedly from a single test process — that's what
makes `internal/e2e` an in-process harness rather than a subprocess-spawning
one. `cmd/hobnob/main.go` is the only caller: it builds a real `*App` via
`app.New(version)`, adds signal handling, and translates `Run`'s returned
error into an exit code.

### `internal/tui`

Split by widget — `linewriter.go` (the `run:` stdout/stderr line-prefixing
writer), `prompt_text.go`/`prompt_select.go`/`prompt_taskselect.go` (the three
Bubbletea models), `styles.go` (shared lipgloss styles like `SLabel`, `SInfo`,
`SError`, ... plus `TaskPrefix`/`SecretMask`). `PromptText` and `PromptSelect`
are the two entry points the runner calls.

## Data flow, end to end

```
os.Args[1:]
   │  App.Run (app/app.go)
   ▼
hobnob.yml
   │  ParseConfig (config.go)          — YAML → ConfigFile, all templates raw
   ▼
ConfigFile
   │  BuildScope (cli.go)              — env → sysvars → vars: → env-files →
   │                                      CLI args → const:
   │  LoadModules (modules.go)         — needs scope for templated module paths
   ▼
Scope{Vars, Secrets, Ambient}
   │  ExecuteTask → executeSteps (runner.go)
   │    each step: EvalTemplate/EvalCondition against *current* scope
   │    applyModuleLayer on entering a module task (its own env:/vars:/const:)
   │    mutates scope.Vars as it goes (set/get/run into/call into, once: memo)
   ▼
process exit code
```

## Testing conventions

The suite is end-to-end first: most behavior is proven by writing a
`hobnob.yml` fixture, running it through `internal/e2e`'s harness
(`e2e.Yml`/`e2e.Run`, which drives a real `app.New(...).Run(ctx, args)`
in-process), and asserting on what the user would actually see — printed
output, exit code, which prompts fired — via `e2e.Result`'s matchers
(`.OK`/`.Fails`/`.Out`/`.Lines`/`.Masked`/`.Prompted`/...). One file per
feature (`set_test.go`, `call_test.go`, `loop_test.go`, `modules_test.go`,
...); see the mutation checklist atop `internal/e2e/harness_test.go` for the
behaviors that guard specifically. `internal/e2e` tests never run in parallel
— the harness swaps `os.Stdout`, the process environment, and the cwd, all of
which are process-global — and every test drives real interactive prompts via
`runner.SetPrompts`, never `--no-input`-only paths, so the `get:` grammar's
prompt mechanics stay covered.

Reach for a package-local unit test instead only for what an e2e run can't
observe: signals and process groups (`cmd/hobnob/main_signal_test.go`,
`internal/runner`'s `TestKillRunningStep_*`/`TestExecRun_CtxCancelled_*`),
`--upgrade`'s network/tarball handling (`internal/app/upgrade_test.go`),
bubbletea model `Update`/`View` behavior (`internal/tui`), and combinatorial
pure functions like the accessor evaluator's step semantics
(`internal/value/path_test.go`) or dotenv/shell-sourcing edge cases
(`internal/eval/shell_test.go`). Those still follow table-driven `t.Run`
subtests per package; see [CONTRIBUTING.md](CONTRIBUTING.md) for
naming/structure conventions (Given/When/Then/Why, Arrange/Act/Assert).

`runner.SetPrompts(text, sel) (restore func())` is the one seam production
code exposes purely for tests — see the `promptTextFn`/`promptSelectFn`
invariant in [CLAUDE.md](CLAUDE.md). `App.IsTerminal`/`App.SelectTask` are the
other two (struct fields, not package vars — see `internal/app` above).

## See also

- [GUIDE.md](GUIDE.md) — user-facing reference: every YAML field, template
  filter, and CLI flag.
- [CLAUDE.md](CLAUDE.md) — condensed version of this file plus repo-specific
  invariants, aimed at AI coding agents.
- [CONTRIBUTING.md](CONTRIBUTING.md) — PR checklist, testing conventions.
