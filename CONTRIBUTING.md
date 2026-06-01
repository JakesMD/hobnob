# Contributing to hobnob

Hey there! Thanks for taking the time to contribute to **hobnob**. We’re excited
to see what you build! To help keep our codebase healthy and happy, please
follow these guidelines.

---

## ✅ Pre-Flight Checklist

Before you mark a task as done, call it ready for review, or open a final Pull
Request, please make sure you've ticked these boxes:

- [ ] **Tests written/updated:** Every new function or behavior change has test
      coverage—focusing on user-facing functionality rather than internal
      implementation details.
- [ ] **Tests passing:** All project tests pass successfully.
- [ ] **Docs updated:** If your changes alter how hobnob behaves or is
      configured, `GUIDE.md` and `README.md` are updated to match.

---

## 🤖 AI Contribution Guidelines

We absolutely welcome the use of AI tools to help you brainstorm and write code!
We just ask for a little human accountability:

- **Review Everything:** You're the captain of your code. Make sure you fully
  understand, review, and test anything an AI generates before committing it.
- **Keep Comments Human:** Please write your code comments yourself rather than
  letting an AI do it. We want your human perspective!
- **Manual PRs and Issues:** Please don't use automated AI tools to open issues
  or Pull Requests. Keep the communication between us human-to-human.

---

## 💡 Need a Hand? Use Draft PRs!

Stuck on a problem or want some architectural feedback before you fully dive in?
No worries at all! Feel free to open a **Draft Pull Request**. It’s a great way
to ask for advice or get an early pair of eyes on your direction before pushing
it across the finish line.

---

## 🛠️ Project Tasks

All of our development tasks—including building and running tests—live inside
`hobnob.yml`. Just run `hobnob <task>` to get things moving!

---

## 🧪 Testing Guide

We love tests! To keep our test suite clean, reliable, and consistent, please
follow these core principles:

- **Test Functionality, Not Implementation:** Focus on expected behaviors,
  inputs, and outputs. Avoid coupling tests to internal code details so we can
  confidently refactor things later without breaking the tests.
- **The Naming Scheme:** Use a strict **Given / When / Then / Why** format for
  test names to keep the intent crystal clear:

  ```
  given chunk size over limit, when check evaluated, then returns false (why: guard prevents oversized uploads)
  ```

- **The AAA Method:** Structure and clearly label your test blocks with
  `// Arrange`, `// Act`, and `// Assert` comments to keep them readable.
