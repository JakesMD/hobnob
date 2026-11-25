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

## Picking a file or directory

`type: file` turns the prompt into a picker that browses your files, and
`type: dir` does the same for directories. The variable holds the full path
of what was picked:

```yaml
tasks:
  count-questions:
    steps:
      - get:
          - QUESTIONS:
              info: Which questions file?
              type: file
      - run: jq length {{.QUESTIONS}}
```

Type to narrow the list, `/` to step into a directory, ← to go back up and
enter to pick. A path you pass yourself works too, and is turned into a full
path the same way:

```bash
hobnob count-questions QUESTIONS=questions/history.json
```

```
$ hobnob count-questions QUESTIONS=questions/history.json
run: [count-questions] jq length …/questions/history.json
[count-questions] 3
✓ [count-questions] done
```

The picker starts in the task's working directory. Give a directory as
`default:` to start somewhere else, or a file to have it picked already.
With `multi: true`, tab marks each pick and enter takes them all.

## Narrowing what's offered

`match:` limits what an answer can be. A menu shows only the options that
match, a text answer that doesn't match is asked for again, and a picker hides
the files that don't match (directories stay, so you can still browse):

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

In a picker, a pattern with no `/` matches the name, as in `"*.json"`. A
pattern with a `/` matches the end of the path, as in `"lib/*"`. Start it with
`/` or `~/` to match the whole path, or with `./` to match from the task's
working directory. A pattern starting with `!` hides directories too, so
`"!.*"` hides `.git/`. In a `type: dir` picker, directories that don't match
can still be opened but not picked.

`match:` is one pattern, a list of them, or a variable holding a list
(`match: .PATTERNS`). Each pattern can use `{{ }}`, like `"*.{{.EXT}}"`.

## Prompt options

Each entry under `get:` is a variable name. On its own, as in
`get: [CATEGORY]`, it gives a plain text box. These options shape it:

| Option      | Does                                                                                   |
| ----------- | -------------------------------------------------------------------------------------- |
| `info:`     | The question to show.                                                                  |
| `type:`     | `file` or `dir` turns it into a picker (see [Picking a file or directory](#picking-a-file-or-directory)). `text` is the default. |
| `default:`  | The answer filled in to start with. For `type: file`, a directory says where to start; for `type: dir`, a directory is the answer, already selected. |
| `options:`  | Turns it into a menu. A list, or a variable holding one. Not with `type:`.             |
| `multi:`    | Lets the person pick several options or paths. The answer is a list.                   |
| `match:`    | Patterns the answer must match. A menu or picker hides what doesn't.                   |
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
- **Every prompt is a card**: a rounded box titled with the task, `»` and the
  variable name (`deploy » REPLICAS`, plus `(optional)` when allowed). The
  `info:` sits inside it, word-wrapped, and the keys to press are listed under
  it. The card is as wide as the terminal, up to 76 columns. A failing
  `check:` shows `✗` and its message inside the card, which turns the border
  red, and the prompt stays open.
- **CTRL+C is the only key that aborts the run.**
- **A text prompt starts filled in with its `default:`**, so enter keeps it
  (a `secret:` default shows as `••••`). With `optional: true`, enter on an
  empty field skips the prompt, and so does esc.
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

### Files and directories

- **Every `type: file|dir` value is stored as a full path**, each item under
  `multi:`. That includes a value that's already set: it is rewritten, so
  later steps see one form.
- **A relative path resolves by where it came from.** From a command-line
  argument or your shell's environment, it is relative to where you ran
  hobnob. Anywhere written in a taskfile (`vars:`, `const:`, `default:`, an
  earlier step), it is relative to the task's working directory (`dir:`, else
  the taskfile's directory).
- **Paths are tidied**: `./`, `..` and a trailing `/` are dropped, and a
  leading `~` becomes your home directory. Symbolic links are kept as given,
  and judged by what they point to.
- **A picker names existing paths only.** To ask for a new file, ask for a
  `type: dir` and a text prompt for the name.
- **A value nobody picked** (already set, or a `default:` used when prompts
  are off) must exist, be the right kind (a file for `type: file`, a directory
  for `type: dir`), match `match:` and pass `check:`, or the step fails naming
  the variable. This holds with prompts on or off, and the picker never opens
  for it.
- **A directory as the `default:` of `type: file`** only says where to start
  browsing. With prompts off it counts as no default.
- **A `default:` that's missing or the wrong kind** doesn't block the picker:
  it opens in the nearest directory that exists, with nothing picked.
- **With `optional: true` and no answer**, esc in the picker gives no answer,
  and the variable is set empty. An empty value never becomes the working
  directory.
- **A failing `check:` opens the picker again.**
- **The picker lists hidden files** like any other. Only operating-system
  clutter is never listed, whatever `match:` says: `.DS_Store`, `._*`,
  `.Spotlight-V100`, `.Trashes`, `.fseventsd`, `.TemporaryItems`, `Icon\r`,
  `Thumbs.db`, `desktop.ini`, `$RECYCLE.BIN` and `System Volume Information`.
  Names are compared ignoring case.

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
