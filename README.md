<div align="center">
  <a href="https://hobnob.jakesmd.dev/">
    <img src=".github/assets/logo.svg" alt="hobnob logo" width="128">
  </a>

<h1>hobnob</h1>

<p>
    Build wizards in YAML.
  </p>

<p>
    <a href="https://hobnob.jakesmd.dev/">Home</a> &bullet;
    <a href="https://hobnob.jakesmd.dev/docs/quick-start">Quick start</a> &bullet;
    <a href="https://hobnob.jakesmd.dev/docs/guide/">Docs</a> &bullet;
    <a href="https://github.com/jakesmd/hobnob/releases">Releases</a>
  </p>

<p>
    <a href="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml"><img src="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/jakesmd/hobnob/releases/latest"><img src="https://img.shields.io/github/v/release/jakesmd/hobnob" alt="Release"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-BSD--3--Clause-blue" alt="License"></a>
  </p>
</div>

A task runner where prompts are steps, JSON stays data and tasks return values.
It reads like a book, and runs like one.

- **Prompts are just steps.** Ask anywhere in a task: a text box, a menu or a
  multi-select, built from an earlier step's output.
- **JSON stays data.** Dot into it ten steps later, at any depth. No `jq`, no
  parsing step.
- **Tasks return values.** Call a helper and use what it returns in the next
  step, like a function.
- **Runs in CI too.** Pass the answers as `NAME=value` and the same wizard runs
  straight through.

[See it in action →](https://hobnob.jakesmd.dev/)

## 📦 Install

```bash
curl -fsSL https://github.com/jakesmd/hobnob/releases/latest/download/install.sh | bash
```

One static Go binary, nothing else to install.

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
