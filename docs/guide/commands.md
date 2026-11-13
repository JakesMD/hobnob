# Running commands

A `run:` step runs a command, just as you'd type it in a terminal:

```yaml
tasks:
  fetch-question:
    steps:
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1"
```

What the command prints shows as it runs. If the command fails, the task
stops there (see [Errors](./errors.md) to carry on instead). To keep what it
printed in a variable, see [Capturing output](./capturing-output.md).

## Two ways to write a command

Write a command as text and it runs through the shell, so pipes, redirects,
globs, `&&` and `cd` all work. A YAML `|` block lets one step run several
lines:

```yaml
tasks:
  count-questions:
    steps:
      - run: |
          curl -s "https://the-trivia-api.com/v2/questions?limit=5" > questions.json
          grep -o '"id"' questions.json | wc -l
```

Write it as a list instead and it runs without a shell. Each item is exactly
one argument, whatever it holds.

The difference shows when a variable holds something the shell would read,
such as a `;`:

```yaml
tasks:
  greet:
    steps:
      - set:
          - NAME: "Ada; echo oops"
      - run: echo Hello {{.NAME}}
      - run: [echo, Hello, .NAME]
```

```
$ hobnob greet
run: [greet] echo Hello Ada; echo oops
[greet] Hello Ada
[greet] oops
run: [greet] echo Hello 'Ada; echo oops'
[greet] Hello Ada; echo oops
✓ [greet] done
```

As text, the variable becomes part of the command, so the `;` started a
second one. As a list, it stayed one argument.

So use the list form when a command includes a variable you don't control,
such as a prompt answer or something fetched. Use text when you need the
shell. To use a variable in text safely, add the
[`quote`](./templates.md#filters) filter: `echo Hello {{ .NAME | quote }}`.

A list item can be plain text, a template or a variable without braces, like
`.NAME`. A variable holding a list becomes several arguments (see
[Lists as command arguments](./structured-data.md#lists-as-command-arguments)).

## Running in another directory

Commands run in the directory that holds the taskfile, wherever you ran
hobnob from. `dir:` runs them somewhere else. It works on a whole task, on a
single `run:` step, or on a `call:`:

```yaml
tasks:
  archive-question:
    dir: ./questions
    steps:
      - run: curl -s -o latest.json "https://the-trivia-api.com/v2/questions?limit=1" # in ./questions
      - run: ./lint-answers.sh
        dir: ./scripts # this step only
```

A `dir:` path is relative to the taskfile, even on a step inside a task with
its own `dir:`. So `./scripts` above is next to `./questions`, not inside it.

## Hiding noisy output

Some commands print far more than you want to read. `quiet:` swaps their
output for a one-line message:

```yaml
tasks:
  import-questions:
    steps:
      - run: curl -o questions.json "https://the-trivia-api.com/v2/questions?limit=10"
        quiet: Downloading ten questions
      - run: ./import.sh
        quiet: true # no message
```

```
$ hobnob import-questions
run: [import-questions] curl -o questions.json "https://the-trivia-api.com/v2/questions?limit=10"
⊙ [import-questions] Downloading ten questions… (output hidden)
run: [import-questions] ./import.sh
⊙ [import-questions] output hidden
✓ [import-questions] done
```

If a quiet command fails, hobnob prints everything it hid, so you can see
what went wrong.

## Background jobs

A command can start jobs in the background with `&`. hobnob stops those jobs
when the step ends. To let them finish first, end the step with `wait`:

```yaml
tasks:
  fetch-all:
    steps:
      - run: |
          ./fetch.sh questions &
          ./fetch.sh categories &
          wait
```

To leave a job running after hobnob finishes, such as a server, start it
with `set -m` under `bash`, and send its output to a file:

```yaml
tasks:
  start-server:
    steps:
      - run: bash -c 'set -m; ./server.sh > server.log 2>&1 &'
```

The job keeps running after hobnob exits, so you'll need to stop it
yourself.

## Rules

### Directories

- **When several `dir:`s apply, the most specific wins.** A `call:` step's
  `dir:` beats the called task's own `dir:`, which beats the directory the
  task was called from. A `run:` step's `dir:` applies to that step alone.
- **A called task without a `dir:` runs where its caller runs.**
- **[`if:`](./conditions.md) always runs in the task's directory**, even on a
  step with its own `dir:`.

### Hidden output

- **`into:` captures output whether or not it's hidden.**
- **Python holds back its output when it isn't printing to a terminal**, so
  it can arrive late or all at once. Set `PYTHONUNBUFFERED: 1` with `set:`,
  or run scripts with `python -u`.

### Background jobs

- **When a step ends, hobnob sends its background jobs `SIGTERM`.** What
  they print on the way out still shows, and still reaches `into:`. A job
  still holding the step's output 2 seconds later is killed.
- **`set -m` needs `bash`.** It does nothing under `dash`, which is `sh` on
  Debian and Ubuntu.
- **`setsid cmd &` doesn't work instead.** The step can end before the job
  leaves, so it may be stopped anyway.
- **A job that sends its own output elsewhere and ignores `SIGTERM` keeps
  running.**
- **On systems other than Linux and macOS, nothing is signalled.** Background
  jobs keep running, and the step stops waiting for their output after 2
  seconds.
