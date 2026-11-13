# hobnob

A YAML task runner whose tasks pass typed values between steps. This glossary covers how a variable's value is decided before any task runs, and how values cross calls.

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
The variables one taskfile's own `const:`, `env:` files and `vars:` contribute, resolved against its importer's scope (for the root file: the OS env, built-ins and CLI args). A module's file scope never reads a caller's `with:` or `set:`, and applies only to that module's tasks: its `const:` overrides, its `env:`/`vars:` fill only unclaimed names.
_Avoid_: module layer, layer (a layer is one source, a file scope spans several)

### Calls

**Result**:
A variable a task lists in `results:`, promising it exists after any call to it, however it got there. A `call:` with no `into:` copies the results into the caller; with an `into:`, the call takes exactly what `into:` lists instead.
_Avoid_: output (that's a `run:` step's stdout/stderr), return value, export

**Private variable**:
A `_`-prefixed variable. `into:` can't read it out of a called task, so it stays the task's own scratch value. Privacy is upward only: it still flows down to called tasks.
_Avoid_: internal variable, hidden variable (a hidden task is one `--list` leaves out)

### Values

**Reference**:
A variable named without template braces, as `.NAME` with optional accessors (`.A.b[0]`) or as a parenthesised pipeline (`(.X | json)`). It keeps the variable's type, where a `{{ }}` template renders text. Only fields that hold a single value or a list element take one; elsewhere `.NAME` is plain text.
_Avoid_: bare ref, variable expression, accessor (an accessor is the `.b[0]` part)

**Literal**:
A list or map written in YAML inside `set:`, `with:`, `into:`, `const:` or `vars:`, whose leaves are references, templates or plain values. It nests what it holds, as JSON does, unless an element is a spread.
_Avoid_: JSON literal, structure, object (an object is one kind of value)

**Spread**:
A list element written with a trailing `...` (`[.A..., d]`), whose items join the surrounding list one by one instead of nesting inside it as a single element. Only a list can be spread, and only into a list.
_Avoid_: append, concat, flatten
