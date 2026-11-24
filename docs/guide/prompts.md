# Prompts

A `get:` step asks whoever is running the task for a value, and stores the
answer in a variable:

```yaml
tasks:
  ask-trivia:
    steps:
      - get:
          - CATEGORY:
              info: Which category?
              default: general_knowledge
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
```

If the variable is already set, hobnob doesn't ask. So the same task asks a
person, but takes the value straight from a command-line argument:

```bash
hobnob ask-trivia                     # asks for a category
hobnob ask-trivia CATEGORY=history    # doesn't ask
```

It doesn't matter how the variable was set. An earlier step, an env file or
your shell's environment all count.

That includes a [`vars:`](./variables.md#variables-for-the-whole-file)
default, so a prompt for a name in `vars:` never appears. To suggest a value
and still ask, use the prompt's own `default:` instead.

## Menus

`options:` turns the prompt into a menu. Add `multi: true` to let the person
pick several, and the variable holds a list:

```yaml
tasks:
  quiz:
    steps:
      - get:
          - CATEGORIES:
              info: Which categories?
              options: [history, science, music]
              multi: true
```

## Menus from live data

`options:` can be a variable holding a list, so a menu can come from an
earlier step. The trivia API's `/metadata` counts questions per category, and
`keys` turns those counts into a list of category names:

```yaml
tasks:
  ask-trivia:
    steps:
      - run: curl -s https://the-trivia-api.com/v2/metadata
        into:
          - CATEGORIES: stdout.byCategory | keys
      - get:
          - CATEGORY:
              info: Which category?
              options: .CATEGORIES
```

The menu only offers categories the API really has.

## Checking an answer

`check:` is a shell condition the answer must pass. If it fails, the prompt
asks again:

```yaml
tasks:
  quiz:
    steps:
      - get:
          - COUNT:
              info: How many questions?
              default: 3
              check: "[ {{.COUNT}} -le 10 ]"
```

## Narrowing what's offered

`match:` limits what an answer can be. A menu shows only the options that
match, and a text answer that doesn't match is asked for again:

```yaml
tasks:
  release:
    steps:
      - get:
          - TAG:
              info: Which tag?
              options: [v1.0, v1.5, v2.0, main]
              match: "v1.*"
          - BRANCH:
              info: Which branch?
              match: ["feature/*", "!feature/wip*"]
```

`TAG` offers `v1.0` and `v1.5`. `BRANCH` accepts `feature/login` but not
`main` or `feature/wip-login`.

`match:` is one pattern, a list of them, or a variable holding a list
(`match: .PATTERNS`). Each pattern can use `{{ }}`, like `"*.{{.EXT}}"`.

## Prompt options

Each entry under `get:` is a variable name. On its own, as in
`get: [CATEGORY]`, it gives a plain text box. These options shape it:

| Option      | Does                                                                                   |
| ----------- | -------------------------------------------------------------------------------------- |
| `info:`     | The question to show.                                                                  |
| `default:`  | The answer filled in to start with.                                                    |
| `options:`  | Turns it into a menu. A list, or a variable holding one.                               |
| `multi:`    | Lets the person pick several options. The answer is a list.                            |
| `match:`    | Patterns the answer must match. A menu hides the options that don't.                   |
| `check:`    | A shell condition the answer must pass.                                                |
| `secret:`   | Hides the answer as it's typed and wherever hobnob prints it (see [Secrets](./secrets.md)). |
| `optional:` | Allows no answer.                                                                      |

## Running without prompts

hobnob never prompts in a script. It turns prompts off when its input isn't a
terminal, when the `CI` environment variable is set, or when you pass
`--no-input`. Each `get:` then uses its `default:`:

```bash
hobnob ask-trivia --no-input                     # uses general_knowledge
hobnob ask-trivia CATEGORY=history --no-input    # uses history
```

A prompt with no `default:` and no value already set stops the task, and
tells you which variable to pass.

## Rules

- **A value that's already set still has to pass `match:` and `check:`**, or
  the step fails, with prompts on or off. The same goes for a `default:` used
  when prompts are off. When prompts are off, it must also be one of the
  `options:`.
- **`check:` never filters.** It only judges the final answer, and with
  `multi:` it sees the whole list. `match:` is what narrows the menu.
- **A `multi:` value that's already set or comes from `default:` must be a
  list**, or text that's a JSON list, such as `CATEGORIES='["history","music"]'`
  on the command line or in your shell. Other text, like `CATEGORIES=history`,
  stops the task. An empty value means no picks.
- **With `optional: true` and no answer**, the variable is set but empty
  (`[]` with `multi:`), and `match:` and `check:` are skipped.
- **A menu with nothing left after `match:`** stops the task, naming the
  variable and the patterns. With `optional: true` the variable is set empty
  instead, without asking.
- **With `multi:`, each item must match on its own.**
- **Put `get:` steps early**, so nobody waits on a slow command before
  they're asked anything.

### Patterns in `match:`

- **A value is kept** if it matches any plain pattern (or there are none) and
  no pattern starting with `!`. The order doesn't matter.
- **Matching ignores case.** `*` and `?` stay within one `/`-separated part,
  `**` crosses `/`, `[abc]` matches one of the letters, and `{yml,yaml}`
  matches either word.
- **Menu items and text answers are matched whole.** `feature/*` matches
  `feature/login` but not `feature/a/b`, and `v1` doesn't match `v10`.
- **A pattern that isn't valid**, such as `[abc`, stops the task naming the
  variable.
