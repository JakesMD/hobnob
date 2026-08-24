# Architecture

Hobnob is a single Go binary: a YAML task runner built on a **timeline** rather
than a dependency graph. There is no build-target resolution and no "is this out
of date" check. A task is a list of steps, and variables, conditions and prompts
resolve in order as execution reaches each one.

Two decisions shape everything below:

1. **Nothing is evaluated at parse time.** Every field that can hold a `{{ }}`
   template is stored as raw text and rendered later, against the scope as it
   exists at that moment.
2. **A variable is a typed value, not text.** A scope var is nil, a string, a
   bool, a number, an array or an object. JSON captured from a command stays
   structured for the rest of the run.

## Setup

```bash
go build -o hobnob ./cmd/hobnob   # or: hobnob go:build
go test ./...                     # or: hobnob go:test
go test ./internal/e2e            # or: hobnob go:test:e2e
go vet ./...                      # or: hobnob go:vet
```

Requires Go 1.24+. No codegen, no external services, no build step beyond
`go build`. The project self-hosts its dev tasks in `hobnob.yml`; see
[CONTRIBUTING.md](CONTRIBUTING.md).

Dependencies: `gopkg.in/yaml.v3` for parsing, and
`github.com/charmbracelet/{bubbletea,bubbles,lipgloss,x/term}` for the
interactive prompts and styled output.

## Package layout

```
cmd/hobnob/       entry point: signal handling, exit codes
internal/app/     the CLI body: flag dispatch, App.Run(ctx, args)
internal/runner/  step execution (the interpreter loop)
internal/cli/     scope construction, --list/--help, completions
internal/tui/     bubbletea prompts, lipgloss styles, output line writers
internal/config/  YAML into typed structs, module and env-file loading
internal/eval/    template rendering, accessor rewriting, shell evaluation
internal/value/   the typed scope value, its filters and accessor engine
internal/e2e/     end-to-end CLI suite (test files only, no production code)
```

Imports run one way, bottom to top:

```
value  <- eval  <- config  <- cli  <- runner  <- app  <- cmd/hobnob
   ^        ^         ^        ^        ^         ^
   +--------+---------+--------+--------+---------+   (tui: value only)
```

- `value` sits at the bottom with stdlib imports only.
- `eval` imports `value`; `config` imports `eval` and `value`, never the
  reverse.
- `tui` imports `value` but not `eval`. `PromptText` takes a validator closure
  instead of evaluating a `check:` expression itself, so the prompt layer never
  learns how a shell condition works.
- `cli` imports `config`, `eval`, `tui`, `value`. `runner` imports all of those.
- `app` imports `cli`, `config`, `eval`, `runner`, `tui`. Nothing imports `app`,
  except `internal/e2e`, which is a leaf of `_test.go` files.

### `internal/value`: the typed scope value

`Value` wraps one of: nil, `string`, `bool`, `json.Number`, `[]any`,
`map[string]any`. Numbers are `json.Number` and never `float64`, which avoids
both precision loss and stray `.0` suffixes. Nested containers use the same
element types recursively, so `Any()` feeds the accessor engine with no
adaptation.

There is a seventh kind, `KindMissing`, that exists only transiently inside a
single accessor evaluation. See "Absence" below.

**`Capture(text)` is the one sniff point.** Text becomes structure only if it
decodes cleanly as a JSON array or object; otherwise it stays a string.
`run: into:` is the only caller. Env vars, CLI args, env-file values and prompt
answers all go through `Str` and are never sniffed, however JSON-shaped they
look. The explicit `json` filter is the other way in.

**`Filters` is the single filter registry**: `default`, `trim`, `upper`,
`lower`, `split`, `lines`, `keys`, `json`, `string`, `len`, `quote`,
`jsonEscape`. One definition serves three callers: text/template execution
(adapted into a `FuncMap` in `eval`), the type-preserving evaluator, and
`run: into:` pipes.

**`Path` (`path.go`) evaluates an accessor chain** such as
`.A.b[0][.KEY][1:3][*]` against a `Value`. It dispatches on the container's
kind, not the key's shape. A star or slice step turns on multiplicity for every
later step: subsequent steps map over the nodes, dropping non-matching ones
rather than failing. Results come back wrapped as `PathCall`, `StarCall` or
`SliceCall` for `eval`'s two evaluators.

**Absence is deferred; a wrong kind is not.** A missing key or out-of-range
index yields a `Missing` sentinel rather than a Go error, so a later `| default`
in the same pipeline can still catch it. Every other consumer raises it as a
real error first: the `adaptFilter`/`evalFilterCommand` guard, the comparison
helpers, `EvalValue`'s final check, and `EvalTemplate`'s poison-marker scan
(`ScanMissing`), which exists because `Value.String()` is an `fmt.Stringer` and
cannot return an error. Indexing a string or slicing an object is always an
immediate error, never deferred and never catchable by `default`.

`ShellQuote` (the POSIX single-quote escaper behind the `quote` filter) lives
here rather than in `eval`, because `eval` imports `value` and not the reverse.

### `internal/eval`: templates, accessors, shell

Everything renders through Go's `text/template` against
`map[string]value.Value`. The public surface:

- **`EvalTemplate(tmpl, vars)`** renders to a string. `templateFuncs` is built
  once at package init, since this is the hottest path in the codebase, and
  rebuilding the map per call showed up under loop-heavy tasks. It also
  overrides `eq`/`ne`/`lt`/`le`/`gt`/`ge`, because text/template's builtins
  compare on `reflect.Kind`, which for a `Value` is always `Struct`.
- **`EvalValue(expr, vars)`** is the type-preserving counterpart. When `expr` is
  exactly one action referencing a var, optionally through an accessor and/or a
  filter chain, it walks the parsed tree and evaluates directly against
  `value.Filters`/`value.Path` instead of executing the template, so the result
  keeps its kind. Anything else falls back to `EvalTemplate` and comes back as a
  String. This is what makes `options: .TYPES` and `- run: [curl, .CURL_OPTS]`
  keep their types.
- **`EvalCondition(ctx, expr, vars, dir)`** renders, then runs the result
  through `sh -c`; exit 0 is true. Backs `if:` and `check:`. Context
  cancellation kills the shell outright, so a condition cannot hang past a
  CTRL+C. `EvalCheckWithOverride` is the re-prompt variant, evaluating a
  candidate answer without committing it to scope.
- **`EvalRunIntoPipe(expr, stdout, stderr, exit, vars)`** backs `run: into:`. It
  resolves one of three sources (`stdout`/`stderr` through `value.Capture`,
  `exit` as a typed number), then runs any accessor and filter chain from there
  through the same typed evaluator `EvalValue` uses. The caller's own vars are
  layered underneath, so a dynamic key (`stdout[.KEY]`) resolves against scope.
  Capturing at the source rather than at the end of the chain is deliberate: a
  chain landing on a string leaf whose text looks like JSON must stay a string.
- **`ResolveArgv(tmpls, vars)`** assembles the `run:` list form. An array
  element splices into one argument per item (an empty array splices to
  nothing); an object is an error naming the accessor fix; an empty element is
  preserved as one empty argument, since dropping it would silently shift every
  later position.
- **`ReferencedVars(expr)`** collects every top-level var name a template
  touches at any depth, including inside a dynamic key. `config`'s `const:` and
  `vars:` checks are built on it.
- Small shared helpers live here too because several packages need the same
  rule: `ResolvePath` (relative paths resolve against the taskfile dir),
  `SplitKV` (the `KEY=VALUE` rule shared by CLI args, `os.Environ()` and env
  files), `CloneMap`, `ResolveItems`/`ItemsFromValue` (`loop:` and `options:`),
  `IsBareRef` and `SplitSourceAccessor`.

**The accessor rewriter (`accessor.go`) is the one piece of real machinery.**
text/template has no bracket-subscript grammar, and hobnob does not fork it.
Instead, a source-to-source lexer pass runs before every parse and rewrites each
accessor chain inside a `{{ }}` action into a call to hobnob's own
`hbpath`/`hbstar`/`hbslice` template funcs. Bytes outside actions, and the
contents of string, rune and raw-string literals, come through unchanged, so a
`}}` or a `[` inside a quoted string is never mistaken for syntax.

Those three funcs are registered twice over. `chainvalue.go`'s typed evaluator
calls `value.PathCall`/`StarCall`/`SliceCall` directly, while the `FuncMap`
registers reflection wrappers so that shapes the typed evaluator does not
recognize (a bare `{{ .A.b }}` with no filter, or an accessor used as an `eq`
argument) still evaluate correctly.

`shell.go` also holds `SourceShellFile`, which sources a `.sh` env file in a
subshell and diffs the result against a baseline `env` snapshot, so ambient
noise like `SHLVL` is not pulled in.

### `internal/config`: parsing and loading

`ParseConfig(path)` reads a file; `ParseConfigData(data, filePath, dir)` parses
bytes that never came from disk, which is how the embedded `--demo` taskfile is
loaded. Both walk a `yaml.Node` tree into a `ConfigFile`: tasks, modules,
env-file entries, `const:` and `vars:`. An unrecognized top-level key is a
load-time error rather than a silent no-op, so `taks:` fails loudly.

Every template-bearing field is stored raw. `const:`/`vars:` are the apparent
exception but not a real one: their _values_ still defer to `BuildScope`, and
only their _reference structure_ is inspected at parse time.

**Parse-time rules** (`constvars.go`, run per file once the whole tree is
parsed):

- `checkConstClosedWorld`: a `const:` entry may reference only earlier `const:`
  entries and the two built-in vars. Otherwise a constant could read a
  lower-priority layer and still call itself fixed.
- `checkVarsNoSelfReference`: a `vars:` entry may not reference its own key. It
  already is the fallback layer.
- `checkConstNamesNotShadowed`: no task's `set:`/`get:`/`into:`/`loop:` target
  may collide with a `const:` name. Checked per file, so a module's tasks are
  checked against the module's own `const:`, never the parent's.

`use:` and `rerun:` are parse-time errors that name `call:`/`once:` as the
replacement.

**Literal assembly.** A `set:`/`with:`/`into:`/`const:`/`vars:` map or list
literal parses into a `JSONNode` tree whose string leaves are unevaluated text.
`EvalJSONNode` walks that tree, calls the caller's `evalLeaf` on every leaf, and
assembles a real Go tree wrapped in a `Value`. It never marshals to JSON text
and back, which is what stops an evaluated leaf containing a quote or backslash
from corrupting the structure around it. The leaf grammar varies by caller
(`set:` leaves are Go templates, `into:` leaves are the `stdout | filter`
grammar, `call: into:` leaves are a template or a bare child reference) and
`EvalJSONNode` is agnostic to all of it. `EvalSetEntry` is the scalar-or-literal
wrapper shared by `set:`, `with:`, `const:` and `vars:`.

**Loading** happens after `BuildScope`, since module and env-file paths can
themselves be templates:

- `envfiles.go`: `.sh` files are sourced in a subshell, anything else is parsed
  as `KEY=VALUE` lines with optional `export` prefixes and `#` comments. A
  missing file warns on stderr and is skipped rather than failing the run. Later
  entries win.
- `modules.go`: resolves and merges imported files recursively, namespacing
  tasks by their module key, applying `show:`/`hide:`/`flatten:`. A module's own
  `env:`/`vars:`/`const:` is evaluated against a module-local scope and also
  recorded as a delta on its own `ConfigFile` (`ModuleLayer`/`ModuleConstLayer`,
  via `applyModuleSetEntries`). Parsing and loading only ever _build_ that
  delta. Applying it to a live scope is `runner`'s job, described below.

File split: `config.go` (root parse, tasks, step sequences), `types.go` (the
structs), `yaml.go` (node helpers, `normalizeTmpl`), `steps.go` (per-kind
dispatch, `loop:` parsing), `vars.go` (`set:`/`with:`/`into:` entries and
literals), `get.go`, `constvars.go`, `jsonvalue.go`, `modules.go`,
`envfiles.go`.

`normalizeTmpl` is what lets a field whose whole value is one reference drop the
braces: `options: .VAR[0].name` is wrapped in `{{ }}` at parse time when
`eval.IsBareRef` recognizes it.

### `internal/cli`: scope and presentation

`Scope` is three maps:

```go
type Scope struct {
    Vars    map[string]value.Value
    Secrets map[string]bool
    Ambient map[string]bool
}
```

`Ambient` marks a key whose value still comes only from the OS-environment base
layer. It is cleared the moment any higher layer touches that key. This is what
lets a module's own `env:`/`vars:` block tell a genuinely inherited default from
something a more specific layer already committed, without being able to see
which layer produced it.

`BuildScope` layers in strict precedence order:

```
env  <  system vars  <  vars:  <  env files  <  CLI args  <  const:
```

Every source except `const:`/`vars:` is wrapped in `value.Str` and never
sniffed. `const:`/`vars:` route through `config.EvalSetEntry` and stay typed.
Above `const:` there is no ranking at all, only execution order: a task's own
`set:`/`get:`/`loop:`/`call:` steps run afterwards and each sees everything
before it.

Three accessors carry the write rules:

- `Set` always overwrites, propagating the secret flag.
- `SetIfDefault` only fills a key that is absent or still `Ambient`, and the
  value it writes is not itself ambient.
- `Copy` deep-copies all three maps, giving every `call:` step an isolated
  sandbox.

**Secrecy is a property of origin, not of use.** `Secrets` rides along on
`Copy()`, and `runner.maskSecrets` matches on _value_ rather than key. A secret
therefore stays masked when a `with:` entry passes it into a child under a
different name, which is exactly why `secret:` on a `with:` entry is rejected at
parse time instead of honored: it would be redundant at best, and would
over-mask a composed value like `postgres://{{.USER}}:{{.PASS}}@db` at worst.

The rest of the package is output: `--list`/`--help` rendering, task-selector
data, the docs-URL helpers that pin links to the running version, and the
bash/zsh/fish completion scripts embedded from `internal/cli/completions/`.

### `internal/runner`: the interpreter

`ExecuteTask` to `executeTask` to `executeSteps` is the core loop: iterate a
task's `[]Step`, check `if:` before each one, dispatch on `StepKind`, evaluate
templates against the _current_ scope that earlier steps may have just mutated.

State threaded through the call graph (context, config, task name, prompt flag,
working dir, and the `once:` memo) is bundled into an `execCtx` struct.
`*cli.Scope` stays a separate argument, because it is the thing being mutated: a
`call:` swaps in a fresh child scope while everything else in `execCtx` carries
forward.

**`run:` (`run.go`).** Dispatches on `Step.Argv` versus `Step.Command`. A YAML
sequence resolves each element through `eval.ResolveArgv` and execs argv
directly with no shell; a scalar renders the template and runs `sh -c`. Both
branches share process-group setup, output plumbing, `into:` capture and
interrupt handling.

- Output normally streams live through a `tui.LineWriter` per stream, teed into
  a buffer when `into:` needs it.
- `quiet:` swaps that for buffers only, printing `tui.RunQuietLine` in place of
  the command's own output. Any non-nil error from `Wait`, an ordinary failure
  or an interrupt, replays both buffers through the line writers before
  returning, so a hidden step is never silently invisible on failure.
- `captureRunInto` runs after `Wait` regardless of outcome, so a `soft: true`
  step can still capture `exit`, `stdout` and `stderr` from a failing command.
  Only a `Start` failure (no process ever ran, so there is no exit status) or an
  interrupt (already mid-shutdown) skips capture.
- `envWithScopeOverrides` strips any inherited env var that scope also defines
  before appending scope's own, because `os/exec` uses first-occurrence-wins and
  scope must win.

**`call:` (`call.go`).** Deep-copies scope, evaluates `with:` into the child,
runs the target task, then pulls results back through `into:`. An `into:` leaf
is either an explicit `{{ }}` template evaluated against the _caller's_ evolving
scope, so later entries can reference earlier ones, or a bare child key with an
optional accessor and filter chain read straight out of the child scope, typed.
Only a plain bare key propagates its secret flag; every other shape loses the
annotation, the same way passing a value through a filter does.

A `once: true` target is memoized per invocation through a `callMemo` carried in
`execCtx`, which survives the scope swap so a shared prologue replays into
sibling sandboxes. Three things about it are load-bearing:

1. **Keyed on `Task.Steps` slice identity, not name.** `registerModuleTasks` can
   register one task under several names (module prefix, `flatten:` alias, its
   own bare name from inside the module file) that all share one backing array,
   so every route to it collapses to one cache entry.
2. **The whole child scope is cached, not a delta.** Each call site's own
   `into:` independently projects what it wants out, which a delta would have to
   guess in advance.
3. **A hit is announced.** `tui.CallCacheHitLine` names what the first run
   produced or changed (`summarizeCallDelta`, masked and sorted), so a memoized
   call is never invisible.

A `running` set guards against a `once:` task reaching itself before its first
run completes, which would otherwise recurse forever with nothing yet in the
cache.

**`get:` (`get.go`).** No-ops when the var is already in scope, which is exactly
how a CLI `KEY=VALUE` arg satisfies a prompt. Otherwise it uses `default:`,
aborts, or prompts through the package-level `promptTextFn` / `promptSelectFn`,
which tests substitute via `SetPrompts`.

**`loop:` (`loop.go`).** Dispatches on the target's `value.Kind()` rather than
sniffing text: an Object runs the map form setting `KEY`/`VALUE`, anything else
runs the list form setting `ITEM`, and a matrix runs the cartesian product
recursively. Iterator vars stay typed, and prior values are restored on exit.

**`set:` (`set.go`).** Evaluates entries top to bottom into scope, each seeing
the ones before it.

**`applyModuleLayer` (`runner.go`)** is not a step kind. It runs inside
`executeTask` whenever the resolved task belongs to a module (`Task.Cfg != nil`)
and writes that module's own layers into the scope the task is about to run
with: `env:`/`vars:` through `SetIfDefault` (a default for the subtree), then
`const:` through `Set` (an override, because the nearest declaration wins).
Re-running it on a nested call into the same module is idempotent. This is the
step that makes a module's own `env:`/`const:`/ `vars:` reach its tasks at all;
`config` only ever computes the delta.

**`soft:`** is shared by `run:` and `call:`. `executeSteps` swallows a
non-interrupt error from either kind when the step sets it, so the timeline
continues.

**Signals.** `runner_unix.go` / `runner_windows.go` handle process groups: the
first CTRL+C signals the whole group, catching children a single-process signal
would miss, and waits. A second CTRL+C force-kills through `KillRunningStep`.
The running PID is guarded by a mutex so a second CTRL+C racing the instant a
step exits cannot signal a PID the OS has since reused.

### `internal/tui`

Split by widget: `linewriter.go` (the prefixing writer that fronts `run:`
output), `prompt_text.go` / `prompt_select.go` / `prompt_taskselect.go` (three
bubbletea models), and `styles.go` (lipgloss styles plus the line builders
`TaskPrefix`, `SkipLine`, `RunSkipLine`, `RunQuietLine`, `CallCacheHitLine`,
`RunDisplayLines`, and `SecretMask`). `PromptText` and `PromptSelect` are the
two entry points the runner calls; `PromptTaskSelect` is the picker `app`
injects.

### `internal/app`: the CLI body

```go
type App struct {
    Version    string
    IsTerminal func() bool
    SelectTask func(ctx context.Context, tasks []tui.TaskItem) (string, error)
}
```

`IsTerminal` and `SelectTask` are struct fields rather than package vars
specifically so `internal/e2e` can substitute a fake terminal and picker per
test run without touching global state.

`App.Run(ctx, args)` is the whole dispatch. It never reads `os.Args` and never
calls `os.Exit`, so it is safe to invoke repeatedly in one process, which is
what makes the e2e suite in-process rather than subprocess-spawning.

Order of business in `Run`:

1. Extract `--file <path>` and `--demo`. Passing both is an error, since they
   are alternative taskfile sources.
2. Answer `--version`, `--upgrade` and `completion <shell>` before any taskfile
   is looked for, so they work from anywhere. Reject a leading `_` task name as
   internal.
3. Pick a source: the embedded demo (`demo.go`, `//go:embed demo.yml`, parsed
   through `ParseConfigData` as though it sat in the invocation directory), an
   explicit `--file`, or `findTaskfile` walking up from the current directory
   for `hobnob.yml` then `hobnob.yaml`.
4. `loadConfig` parses, then `buildScopeFor` runs `cli.BuildScope` followed by
   `config.LoadModules`.
5. Route to `--list`/`--help`/`--select` through `runListingFlag`, so the real
   and demo paths cannot drift, or to `execTask`.

With no task argument and no `default` task, `selectAndRun` opens the picker,
falling back to printing the task list when there is no terminal, `CI` is set,
or `--no-input` was passed. `defaultNoPrompts` is the single place that
CI/terminal detection is decided.

`upgrade.go` replaces the running binary from the latest release tarball.

### `cmd/hobnob`

Thin: build a real `*app.App` via `app.New(version)`, install SIGINT/SIGTERM
handling (graceful cancel on the first signal, `runner.KillRunningStep` on the
second), call `Run`, translate the error into an exit code.

## Data flow, end to end

```
os.Args[1:]
   |  App.Run (app/app.go)
   v
hobnob.yml  (or --file, or the embedded demo)
   |  ParseConfig / ParseConfigData    all templates stored raw
   v
ConfigFile
   |  BuildScope (cli/scope.go)        env -> sysvars -> vars: -> env files
   |                                        -> CLI args -> const:
   |  LoadModules (config/modules.go)  needs scope: module paths are templates
   v
Scope{Vars, Secrets, Ambient}
   |  ExecuteTask -> executeSteps (runner/runner.go)
   |    applyModuleLayer on entering a module task
   |    per step: if: check, then dispatch on StepKind
   |    each step evaluates against the *current* scope and may mutate it
   v
process exit code
```

## Testing conventions

The suite is **end-to-end first**. Most behavior is proven by writing a
`hobnob.yml` fixture, running it through `internal/e2e`'s harness (`e2e.Yml` /
`e2e.Run`, driving a real `app.New(...).Run(ctx, args)` in process), and
asserting on what the user would actually see: printed output, exit code, which
prompts fired. Matchers live on `e2e.Result` (`.OK`, `.Fails`, `.Out`, `.Lines`,
`.Masked`, `.Prompted`, ...), one file per feature.

Two constraints on that package:

- **Never parallel.** The harness swaps `os.Stdout`, the process environment and
  the working directory, all process-global.
- **Real prompts.** Tests drive interactive paths through `runner.SetPrompts`
  rather than relying on `--no-input`, so the `get:` grammar's prompt mechanics
  stay covered.

`internal/e2e/harness_test.go` opens with a **mutation checklist**: each line is
a one-statement break that some test in the package must catch. Walk it after
changing the harness and in full before a release. It is a stronger signal than
a coverage percentage, because it tests the assertions rather than just
execution.

Reach for a package-local unit test only for what an e2e run cannot observe:

- signals and process groups (`cmd/hobnob/main_signal_test.go`,
  `internal/runner`'s kill and cancellation tests)
- `--upgrade`'s network and tarball handling (`internal/app/upgrade_test.go`)
- bubbletea `Update`/`View` behavior (`internal/tui`)
- combinatorial pure functions: the accessor evaluator
  (`internal/value/path_test.go`), the rewriter
  (`internal/eval/accessor_test.go`), dotenv and shell sourcing
  (`internal/eval/shell_test.go`)

Those are table-driven `t.Run` subtests. See [CONTRIBUTING.md](CONTRIBUTING.md)
for naming and structure (Given/When/Then/ Why, Arrange/Act/Assert).

`runner.SetPrompts(text, sel) (restore func())` is the one seam production code
exposes purely for tests. `App.IsTerminal` and `App.SelectTask` are the other
two.

## Invariants worth stating once

- Variables are never evaluated at parse time.
- `Scope.Copy()` happens before every `call:`; nothing leaks back except through
  `into:`.
- Structure is sniffed exactly once, in `value.Capture`, and never re-attempted
  downstream. `keys` and accessor steps error on a string rather than re-parsing
  it.
- An accessor's absence is a deferred sentinel, catchable by `default` alone. A
  wrong-kind access is always an immediate error.
- `const:` is a closed world at load time, and a `const:` name is reserved
  file-wide.
- A module's own `env:`/`const:`/`vars:` never leak to the parent. Inside its
  subtree, `const:` overrides and `env:`/`vars:` only fill gaps.
- `Task.Hidden` is set for `_`-prefixed tasks and tasks from `_`-prefixed
  modules. They run, but stay out of `--list` and the picker.

## See also

- [GUIDE.md](GUIDE.md): user-facing tutorial, builds one working taskfile from
  scratch.
- [REFERENCE.md](REFERENCE.md): every YAML field, template filter and CLI flag.
- [CONTRIBUTING.md](CONTRIBUTING.md): PR checklist, testing conventions.
