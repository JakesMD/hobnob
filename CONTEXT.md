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

**File scope**:
The variables one taskfile's own `const:`, `env:` files and `vars:` contribute, resolved against its importer's scope (the root file's importer is the OS env plus built-ins, and the root file's own scope also takes the CLI args). A module's file scope reads the importer's resolved values, never a caller's `with:` or `set:`, and is applied only to that module's own tasks. Its `const:` overrides, and its `env:`/`vars:` fill only names nothing higher has set.
_Avoid_: module layer, layer (a layer is one source, a file scope spans several)
