<div align="center">
  <a href="https://jakesmd.github.io/hobnob/">
    <img src=".github/assets/logo.svg" alt="hobnob logo" width="128">
  </a>

  <h1>hobnob</h1>

  <p>
    A YAML task runner that reads like a book, and runs like one.
  </p>

  <p>
    <a href="https://jakesmd.github.io/hobnob/docs/installation">Installation</a> &bullet;
    <a href="https://jakesmd.github.io/hobnob/docs/quick-start">Quick start</a> &bullet;
    <a href="https://jakesmd.github.io/hobnob/docs/guide/">Guide</a> &bullet;
    <a href="https://jakesmd.github.io/hobnob/docs/reference/">Reference</a> &bullet;
    <a href="https://github.com/jakesmd/hobnob/releases">Releases</a>
  </p>

  <p>
    <a href="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml"><img src="https://github.com/jakesmd/hobnob/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
    <a href="https://github.com/jakesmd/hobnob/releases/latest"><img src="https://img.shields.io/github/v/release/jakesmd/hobnob" alt="Release"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/license-BSD--3--Clause-blue" alt="License"></a>
  </p>
</div>

Another YAML task runner, except:

- **Tasks return values.** Query an API from any step, use the response for the
  rest of the run.
- **Prompts are just steps.** A text box, a menu or a multi-select, built from
  an earlier step's output.
- **JSON is data, not text.** Dot into it ten steps later, at any depth.
- **Nothing to memorize.** Bare `hobnob` opens a menu of tasks, and whatever
  a task needs, it prompts for.

[See it in action →](https://jakesmd.github.io/hobnob/)

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
