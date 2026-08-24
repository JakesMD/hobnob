# hobnob

[![CI](https://github.com/jakesmd/hobnob/actions/workflows/ci.yml/badge.svg)](https://github.com/jakesmd/hobnob/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/jakesmd/hobnob)](https://github.com/jakesmd/hobnob/releases/latest)
[![License](https://img.shields.io/badge/license-BSD--3--Clause-blue)](LICENSE)

Another YAML task runner, except:

- **Tasks return values.** Query an API from any step, use the response for the
  rest of the run.
- **A form is just a step.** Text, single or multi select, built from an earlier
  step's output.
- **JSON is data, not text.** Dot into it ten steps later, at any depth.
- **Nothing to memorize.** Bare `hobnob` opens a menu of tasks, and whatever
  a task needs, it prompts for.

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static Go binary, nothing else to install.

## 👀 Try it

```bash
hobnob --demo tell-joke
```

![Demo](.github/assets/demo.gif)

That runs this file:

```yaml
tasks:
  tell-joke:
    info: Tell a joke from a type you pick.
    steps:
      # Query an API. The response is a value now.
      - run: curl -s https://official-joke-api.appspot.com/types
        into:
          - TYPES: stdout

      # The menu is built from what that step returned.
      - get:
          - TYPE:
              info: Which kind of joke?
              options: .TYPES

      # A sub-task, handing back a value.
      - call: _fetch-joke
        into:
          - JOKE: .RESPONSE

      # Dot straight into the JSON. No parsing step.
      - run: echo "{{ .JOKE[0].setup }} ... {{ .JOKE[0].punchline }}"

  _fetch-joke:
    steps:
      - run: curl -s https://official-joke-api.appspot.com/jokes/{{.TYPE}}/random
        into:
          - RESPONSE: stdout
```

It reads like a book, and runs like one.

## 📖 Docs

The [hobnob guide](GUIDE.md) builds a working file from scratch. The
[reference](REFERENCE.md) has every field, filter and flag.

## 🤖 GitHub Action

Add hobnob to any workflow:

```yaml
steps:
  - uses: jakesmd/hobnob@v0
    with: # Pin any released version, or omit `with:` for the latest
      version: v0.8.0

  - run: hobnob deploy
```

## 🤝 Contributing

Got ideas or fixes? Check out the [Contributing guide](CONTRIBUTING.md) to see
how to get involved and submit a PR.
