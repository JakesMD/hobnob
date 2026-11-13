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

## Prompt options

Each entry under `get:` is a variable name. On its own, as in
`get: [CATEGORY]`, it gives a plain text box. These options shape it:

| Option      | Does                                                                                   |
| ----------- | -------------------------------------------------------------------------------------- |
| `info:`     | The question to show.                                                                  |
| `default:`  | The answer filled in to start with.                                                    |
| `options:`  | Turns it into a menu. A list, or a variable holding one.                               |
| `multi:`    | Lets the person pick several options. The answer is a list.                            |
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

- **A value that's already set still has to pass `check:`**, or the step
  fails. When prompts are off, it must also be one of the `options:`.
- **With `optional: true` and no answer**, the variable is set but empty
  (`[]` with `multi:`), and `check:` is skipped.
- **Put `get:` steps early**, so nobody waits on a slow command before
  they're asked anything.
