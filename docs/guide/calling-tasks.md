# Calling tasks

A task can run another task as one of its steps, hand it values, and get
values back, much like calling a function. `call:` names the task, `with:`
passes values in, and `into:` brings values back:

```yaml
tasks:
  _fetch-question:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
        into:
          - RESPONSE: stdout

  ask-science:
    steps:
      - call: _fetch-question
        with:
          - CATEGORY: science
        into:
          - QUESTION: .RESPONSE[0].question.text
      - run: echo "{{ .QUESTION }}"
```

```
$ hobnob ask-science
…
[ask-science] As what is infantile paralysis more commonly known?
✓ [ask-science] done
```

The `_` at the start of `_fetch-question` hides it from the task list, since
it's only meant to be called (see [Helper tasks](./tasks.md#helper-tasks)).

The called task starts with a copy of the caller's variables. Whatever it
sets or changes stays with it. Only what `into:` lists comes back.

## Passing values in

`with:` sets variables in the called task. Its entries are written like
[`set:`](./variables.md#setting-variables-in-a-step) entries, and each can
use the ones above it.

The called task can also see everything the caller had, but it's clearer to
pass what it needs through `with:` than to rely on that.

## Getting values back

Each `into:` entry reads one of the called task's variables. It can reach
into it and use filters, just like `stdout` on a `run:` step:

```yaml
tasks:
  ask-science:
    steps:
      - call: _fetch-question
        with:
          - CATEGORY: science
        into:
          - QUESTION: .RESPONSE[0].question.text
          - ANSWER: .RESPONSE[0].correctAnswer
          - RESPONSE # same as RESPONSE: .RESPONSE
          - CARD: # a map built from several values
              question: .RESPONSE[0].question.text
              answer: .RESPONSE[0].correctAnswer
```

A plain name, like `RESPONSE`, brings the variable back under the same name.

## Returning values with `results:`

A task that exists to produce a value can declare it with `results:`. A
`call:` without `into:` then brings it back under the same name:

```yaml
tasks:
  _pick-category:
    results: [CATEGORY]
    steps:
      - get:
          - CATEGORY:
              options: [general_knowledge, science]

  ask-trivia:
    steps:
      - call: _pick-category # CATEGORY is set from here on
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
```

`results:` is a promise: each variable it lists exists once the call ends. It
doesn't matter where the value came from. With
`hobnob ask-trivia CATEGORY=history`, the prompt is skipped because
`CATEGORY` is already set, and the promise still holds. If a listed variable
isn't set, the call fails.

## Private variables

Start a variable's name with `_`, like `_RAW`, to keep it from being brought
back by a caller's `into:`. It suits scratch values a task only uses itself.

## Rules

### `with:`

- **`with:` can't pass a name the calling file declares in
  [`const:`](./variables.md#variables-for-the-whole-file).**

### `into:`

- **Reading a variable the called task never set is an error**, unless you add
  `| default`, as in `.AUTHOR | default "anonymous"`.
- **Templates read the caller's variables, not the called task's.** In
  `LABEL: "{{ .CATEGORY }} question"`, `CATEGORY` is the caller's, and so is
  the key in `.RESPONSE[.KEY]`.

### `results:`

- **A missing result fails the call**, even if `into:` has a `| default`
  for it.
- **Running the task directly checks nothing.** The promise only applies to
  `call:`.
- **`into:` replaces `results:`.** A call with its own `into:` takes exactly
  what `into:` lists, and `into: []` takes nothing. Use it to rename a
  result, or to avoid overwriting a variable the caller already has.
- **List plain names only**: no `_` names, no repeats, no paths or filters.
  Reshape at the call site with `into:` instead.
- **A call without `into:` can't bring back a `const:` name** of the calling
  file. Give the call an `into:`, even `into: []`, to call such a task.

### Private variables

- **`| default` doesn't hide the error** when `into:` reads a `_` variable.
- **Privacy only works upwards.** A called task still sees the caller's `_`
  variables, and `with:` can pass them down.

### Other step options

- **A `call:` step also takes [`dir:`](./commands.md#running-in-another-directory) and
  [`soft:`](./errors.md)**, as `run:` does.
- **A task from another file is called as `module:task`** (see
  [Modules](./modules.md)).
- **Secrets stay masked** across a call in both directions (see
  [Secrets](./secrets.md#what-gets-masked)).
