# Contributing to hobnob

Thanks for contributing! Here's what to keep in mind.

---

## ✅ Pre-flight checklist

Before calling it done or opening a PR:

- [ ] **Tests written/updated** — new behavior has coverage, focused on user-facing functionality.
- [ ] **Tests passing** — all project tests pass.
- [ ] **Docs updated** — if behavior changed, the docs site (`docs/`) and `README.md` reflect it.

---

## 🤖 AI tools

AI is welcome for brainstorming and drafting code. Just a few ground rules:

- **Review everything** — you own the code, AI-generated or not.
- **Write comments yourself** — we want your perspective, not a generated summary.
- **No automated PRs or issues** — keep that communication human.

---

## 💡 Draft PRs

Stuck or want early feedback? Open a draft PR. Great for getting a second opinion before you're knee-deep in code.

---

## 🛠️ Project tasks

Build, test, and everything else lives in `hobnob.yml`. Run `hobnob <task>` to get started.

Run `hobnob go:check` before pushing: it vets, lints and tests. Linting needs [golangci-lint](https://golangci-lint.run) v2 installed, and its findings, including each test's shape, are the rules.

---

## 💬 Code comments

Comment only where absolutely necessary. Good names and small functions come first. If code needs a comment to be understood, try renaming or extracting before writing one.

- **Doc comments are welcome** — on any package, func, type, field, var or const, exported or not.
- **Inside a body, explain *why*, never *what*** — a platform quirk, a workaround, a subtle invariant, a reason something is deliberately absent. If the comment restates the code, delete it.
- **No duplicated docs** — behaviour is specified in `docs/guide/`; don't re-describe it in code. Link to the guide or an ADR instead.
- **No signposting or history** — no section banners, "see also" maps of where other tests live, or notes about what code used to be. That's what git is for.
- **Lint-required comments are exempt** — the test comments `hobnob go:lint` asks for stay.

---

## 🧪 Testing

- **Test behavior, not internals** — focus on inputs and outputs so refactors don't break tests unnecessarily.
