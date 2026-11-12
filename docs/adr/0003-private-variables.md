# Private variables

A task that fetches a value usually builds scratch values on the way (raw command output, a list to pick from), and with `results:` the line between a task's answer and its working is worth stating. A `_`-prefixed variable is now private: `into:` can't read it out of a called task, `| default` doesn't hide that error, and `results:` can't list it. Privacy is upward only. A called task still sees the caller's `_` variables, `with:` can pass them down, and the CLI and env files can still set them.

## Considered Options

- **Restrict `into:` to declared results.** A task with `results:` would expose nothing else. Encapsulating, but it removes the escape hatch for a caller that needs one extra value, and makes tasks with and without `results:` behave differently.
- **Hide `_` variables everywhere (CLI, env files, `with:`).** Stricter, but a much larger change for no clear gain: the risk being guarded is a caller depending on a task's internals, which only happens upward.
- **`_` prefix, upward only (chosen).** Reuses the meaning `_` already has for tasks and modules, so there is nothing new to learn.

## Consequences

Existing files that pulled a `_` variable through `into:` now fail and must rename it. Brace-free references (`options: ._TYPES`) accept a leading `_`, so private values read as easily as public ones.
