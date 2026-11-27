---
title: "hobnob vs a custom CLI: when a wizard needs real code"
titleTemplate: false
description: "The same terminal wizard written as a Python CLI and as a hobnob task, side by side, and when it's time to build a real CLI."
aside: false
pageClass: compare-page
---

# hobnob vs a custom CLI

When a team's wizard outgrows a bash script, the next step is often a small
CLI: Python with [questionary](https://github.com/tmbo/questionary), Go with
[huh](https://github.com/charmbracelet/huh), or Node with
[Inquirer](https://github.com/SBoudrias/Inquirer.js). That gives you a real
language, tests, and full control over the screens.

It also gives you a program to build, install and keep working. hobnob sits
in between: the same wizard as a task in YAML, with nothing to build.

## At a glance

|                                  | hobnob                                                  | A custom CLI                                                   |
| -------------------------------- | ------------------------------------------------------- | -------------------------------------------------------------- |
| Logic                            | Steps, with `if:`, `loop:` and `call:`                  | A whole language                                               |
| Screens                          | Text box, menu, multi-select, hidden input, file and folder picker | Whatever the library offers, or you build           |
| Going back to an earlier question | No                                                     | With form libraries such as huh                                |
| Tests                            | Run the task                                            | Unit tests                                                     |
| Who can run it                   | Anyone with the repository and hobnob, on macOS or Linux | Anyone you ship it to, Windows included                        |
| For people who never open a terminal | `hobnob --browser` asks on a local web page         | A web app, written and hosted separately                       |
| Building and installing          | Nothing to build. One binary, one YAML file             | A virtualenv, a package, or a compiled binary per platform     |
| Answers from the command line    | Built in: `NAME=value` skips the question               | A flag per question, wired to its prompt by hand               |
| The log                          | Every command, with secrets shown as `****`             | Whatever you print                                             |

## The same wizard

The deploy wizard from [hobnob vs a bash script](./bash.md): pick services,
pick an environment, give a token, then build and restart each one.

<div class="side-by-side">
<div>

hobnob

```yaml
tasks:
  deploy:
    info: Build services and roll them out to an environment.
    steps:
      - run: gh api repos/acme/platform/contents/services
        quiet: true
        into:
          - ALL_SERVICES: stdout[*].name
      - get:
          - SERVICES:
              info: Which services?
              options: .ALL_SERVICES
              multi: true
          - ENV:
              info: Where to?
              options: [staging, production]
          - TOKEN:
              info: Cluster token
              secret: true
      - loop: .SERVICES
        steps:
          - run: docker build -t ghcr.io/acme/{{.ITEM}}:{{.ENV}} services/{{.ITEM}}
          - run: kubectl --token {{.TOKEN}} -n {{.ENV}} rollout restart deploy/{{.ITEM}}
```

```
$ hobnob deploy SERVICES='["api","web"]' ENV=staging
```

</div>
<div>

Python with questionary

```python
#!/usr/bin/env python3
"""Build services and roll them out to an environment."""
import argparse
import json
import os
import subprocess
import sys

import questionary

ENVS = ["staging", "production"]


def ask(flag, question):
    # Without a terminal, questionary can't ask, so say what to pass instead.
    if not sys.stdin.isatty():
        sys.exit(f"Pass {flag}")
    answer = question.ask()
    if answer is None:  # Ctrl-C
        sys.exit(1)
    return answer


parser = argparse.ArgumentParser(description=__doc__)
parser.add_argument("--services", type=lambda s: s.split(","))
parser.add_argument("--env", choices=ENVS)
args = parser.parse_args()

services = args.services
if not services:
    listing = subprocess.run(
        ["gh", "api", "repos/acme/platform/contents/services"],
        capture_output=True, text=True, check=True,
    ).stdout
    names = [entry["name"] for entry in json.loads(listing)]
    services = ask("--services", questionary.checkbox("Which services?", choices=names))
env = args.env or ask("--env", questionary.select("Where to?", choices=ENVS))
token = os.environ.get("TOKEN") or ask("TOKEN", questionary.password("Cluster token"))

for svc in services:
    subprocess.run(["docker", "build", "-t", f"ghcr.io/acme/{svc}:{env}", f"services/{svc}"], check=True)
    subprocess.run(["kubectl", "--token", token, "-n", env, "rollout", "restart", f"deploy/{svc}"], check=True)
```

```
$ pip install questionary
$ ./scripts/deploy.py --services api,web --env staging
```

</div>
</div>

The differences:

- **Each question is written twice.** Once as a flag for CI, once as a
  prompt for people, with `ask` joining the two and checking for a terminal.
  In hobnob, a question and its command-line argument are the same thing.
- **There's something to install.** questionary has to be installed wherever
  the wizard runs, usually in a virtualenv. A Go CLI has to be built for each
  platform and handed out. hobnob's wizard is a YAML file in the repository,
  run by one binary.
- **The log is up to you.** `subprocess.run` prints nothing about what it
  runs. Print the commands, and the token is in the log unless you hide it
  yourself. hobnob prints each command with the token as `****`.
- **hobnob's wizard also runs in a browser.** `hobnob --browser deploy` asks
  the same questions on a local web page (see
  [Browser mode](../guide/browser-mode.md)). For the CLI, that's a second
  front end to build.
- **JSON is fine in both.** Python's `json` makes `gh`'s output easy to
  work with. That's no advantage either way.

## Where a custom CLI is the better pick

- **Real logic.** Data structures, retries, error handling, branches inside
  branches. A wizard that's mostly code belongs in code.
- **Tests.** You can unit-test the logic and mock the commands. A hobnob task
  is tested by running it.
- **Screens.** Custom layouts, live previews, and going back to an earlier
  question are a library's job. hobnob asks its questions in order, one way.
- **People outside the repository.** A CLI can be published, versioned and
  installed by anyone, on Windows too.

So start with hobnob while the wizard is questions and commands, and move it
into a CLI once it needs logic, tests or its own release. Until then, a
CLI is a program to maintain for what a task file can do.

<p class="compare-checked">Last checked November 2026.</p>
