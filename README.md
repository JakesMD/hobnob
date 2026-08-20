# hobnob

A yaml task runner built around 3 things others get wrong:

- **JSON, natively.** Query, filter, slice — no `jq` needed.
- **Tasks return values.** Capture a command's output or a sub-task's results,
  explicitly, back into the caller.
- **Prompts.** Text input or select menus, dropped anywhere in a task.

---

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

---

## 👀 Quick look

```yaml
# hobnob.yml

tasks:
  tell-joke:
    info: Tell a joke from a category you pick
    steps:

      # Returning vars: curl's stdout captured straight into a var
      - run: curl -s https://official-joke-api.appspot.com/types
        into:
          - CATEGORIES: stdout

      # Interactive prompts: options pulled from the step above
      - get:
          - CATEGORY:
              options: .CATEGORIES

      # Returning vars: whole JSON blob pulled back from an isolated sub-task
      - call: _fetch-joke
        into:
          - JOKE: .RESP

      # JSON handling: pluck fields out of the blob, no jq
      - set:
          - SETUP: '{{ .JOKE | pluck "[0].setup" }}'
          - PUNCHLINE: '{{ .JOKE | pluck "[0].punchline" }}'

      # Typed argv: no shell, so no quoting to get wrong
      - run: [echo, .SETUP, "...", .PUNCHLINE]

  _fetch-joke:
    steps:
      - run: curl -s https://official-joke-api.appspot.com/jokes/{{.CATEGORY}}/random
        into:
          - RESP: stdout
```

```shell
% hobnob tell-joke

run: [tell-joke] curl -s https://official-joke-api.appspot.com/types
[tell-joke] ["general","knock-knock","programming","dad"]

Select a value for CATEGORY.
▶ general
  knock-knock
  programming
  dad
↑↓ move  enter select
```

---

## ✨ Other features

- **Typed argv** — `run:` also accepts a YAML list, executed directly with no
  shell: no quoting layer, and an Array-typed element splices into multiple
  arguments (`FLAGS: ["-ldflags", "-s -w"]` arrives as one argument, not two).
- **Modules** — import tasks from other files, namespaced by prefix.
- **Shared prologues** — `use:` a task's steps directly into the caller's
  scope, memoized so a shared setup runs once per invocation no matter how
  many tasks reach it.
- **Loops** — over a list, a matrix of arrays, or a map's key/value pairs.
- **Env files** — load vars from `.env` files or sourced shell scripts.
- **Working dir inheritance** — set it once, override per-call or per-step.
- **Secret masking** — flag any var as secret to mask it in output.
- **CI mode** — skip prompts, fail fast on missing vars.
- **Task listing** — see or interactively pick from available tasks.
- ...and much more in the [hobnob guide](GUIDE.md).

Steps run sequentially, always — hobnob has no parallelism, by design: a
timeline can't garble concurrent prompts, and streamed output stays readable.
Shell backgrounding already covers the common case for free:
`go build -o a & go build -o b & wait`.

---

## 📖 Docs

Everything you need to know to write your first file is right here in the
[hobnob guide](GUIDE.md).

## 🤖 GitHub Action

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin a specific version (optional)
      version: v0.2.3

  - run: hobnob deploy
```

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.
