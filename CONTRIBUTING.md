# Contributing to hobnob

Thanks for contributing! Here's what to keep in mind.

---

## ✅ Pre-flight checklist

Before calling it done or opening a PR:

- [ ] **Tests written/updated** — new behavior has coverage, focused on user-facing functionality.
- [ ] **Tests passing** — all project tests pass.
- [ ] **Docs updated** — if behavior changed, `GUIDE.md`, `REFERENCE.md` and `README.md` reflect it.

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

---

## 🧪 Testing

- **Test behavior, not internals** — focus on inputs and outputs so refactors don't break tests unnecessarily.
- **Naming** — use Given / When / Then / Why:

  ```
  given chunk size over limit, when check evaluated, then returns false (why: guard prevents oversized uploads)
  ```

- **Structure** — use `// Arrange`, `// Act`, `// Assert` blocks.
