# hobnob

A YAML task runner whose tasks pass typed values between steps. This glossary covers how a variable's value is decided before any task runs.

## Language

### Scope layers

**Layer**:
One source of variables in the scope built before a task runs: OS env, `vars:`, env files, CLI args, `const:`. The timeline (a task's own steps) sits above every layer but is not one.
_Avoid_: tier, level, source

**Precedence**:
Which layer's value wins when two set the same name. A higher layer always overrides a lower one: `env < vars: < env files < CLI args < const:`.
_Avoid_: priority, evaluation order

**Upward read**:
A layer's templates reading the final value of a name set by a layer above it. Allowed for every layer, because higher layers settle first; no layer reads a lower one, except that the OS env is readable by every layer but `const:`.
_Avoid_: lookahead, forward reference

**Closed world**:
The rule that `const:` reads nothing but earlier `const:` entries and the built-in variables. It follows from upward reads, since nothing sits above `const:`, and from `const:` alone being barred from the OS env.
_Avoid_: sealed, isolated

**Default**:
A `vars:` value, used only when no higher layer has set that name.
_Avoid_: fallback, initial value
