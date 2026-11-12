# Task results

A task that exists to produce a value, typically a prompt like "pick a USB stick", forced every caller to repeat `into: [{ DISK: .DISK }]`. A task now declares `results: [DISK]`, and a `call:` with no `into:` copies those variables into the caller. `results:` is a promise: after any call, each result must exist in the task's final scope, however it got there (set by a step, inherited from the caller, or kept through an `if:` skip), and a missing one fails the call even if `into:` has a `| default`. An `into:` on the call, even `into: []`, replaces the results instead of adding to them, so a call site can always choose exactly what it takes.

## Considered Options

- **Bare `into:` names only.** `into: [DISK]` alone shortens every call site, but still repeats it everywhere. Kept as shorthand alongside `results:`.
- **`into:` adds to results.** Shorter for "results plus one more", but a call site could never opt out of a result that overwrites a variable it already holds.
- **Results default, `into:` replaces (chosen).** A bare call means "give me what this task produces"; an `into:` means "give me exactly this".
- **A `return:` step.** Puts the declaration where the value is set, but a task skipped by `if:` never reaches it, its branches can return different names, and `return` reads as an early exit.
- **Lazy `vars:` backed by a task.** No step at all, but prompts would fire mid-template, and it breaks the top-down resolution of [ADR-0001](0001-upward-reads-between-scope-layers.md).

## Consequences

Values now leave a call two ways: through `into:`, or through the target's `results:`. A result can't overwrite a `const:` of the caller's file. For a target in the same file that fails at load; a module or templated target's results are only known when the call runs, so there it fails then. Running a task directly, rather than through `call:`, checks no promise.
