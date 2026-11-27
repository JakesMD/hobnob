---
title: "hobnob vs a bash script with gum: building a terminal wizard"
titleTemplate: false
description: "The same terminal wizard written as a bash script with gum and as a hobnob task, side by side, and when the script is the better pick."
aside: false
pageClass: compare-page
---

# hobnob vs a bash script with gum

The usual way to build a terminal wizard is a bash script, with
[gum](https://github.com/charmbracelet/gum) or
[fzf](https://github.com/junegunn/fzf) for the menus. It works, and you
already know bash.

hobnob builds the same wizard as a task in YAML. The questions are steps,
and every answer can also be passed on the command line, so the wizard runs
in CI without extra code.

## At a glance

|                                  | hobnob                                                     | bash with gum                                                    |
| -------------------------------- | ---------------------------------------------------------- | ---------------------------------------------------------------- |
| Logic                            | Steps, with `if:`, `loop:` and `call:`                     | Anything bash can do                                             |
| Kinds of question                | Text box, menu, multi-select, hidden input, file and folder picker | Those, plus a multi-line editor, spinners and more       |
| Filtering a long list            | Every menu and picker filters as you type                  | `gum filter` or `fzf`, in place of `gum choose`                  |
| Something new to learn           | hobnob's YAML                                              | Nothing, if you know bash                                        |
| Answers from the command line    | Built in: `NAME=value` skips the question                  | Hand-written flag parsing                                        |
| Running without a terminal       | Stops and says which `NAME=` to pass                       | gum fails, unless the script checks first                        |
| For people who never open a terminal | `hobnob --browser` asks on a local web page            | A terminal only                                                  |
| Checking passed answers          | Against `check:`, and in CI against `options:` too         | Hand-written, per answer                                         |
| The log                          | Every command, with secrets shown as `****`                | Nothing, or everything with `set -x`, secrets included           |
| JSON from a command              | [Data](../guide/structured-data.md), as in `stdout[*].name` | Text, cut up with `jq`                                           |
| Installed on every machine       | hobnob                                                     | gum, wherever someone answers the questions                      |

## The same wizard

Deploy some services: pick which ones from the repository, pick an
environment, give a token, then build and restart each one.

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

</div>
<div>

bash with gum

```bash
#!/usr/bin/env bash
# scripts/deploy.sh
# Build services and roll them out to an environment.
# Usage: deploy.sh [--services api,web] [--env staging|production]
# The token comes from $TOKEN, or is asked for.
set -euo pipefail

services="" env="" token="${TOKEN:-}"
while [ $# -gt 0 ]; do
  case "$1" in
    --services) services="$2"; shift 2 ;;
    --env) env="$2"; shift 2 ;;
    *) echo "Unknown option: $1" >&2; exit 1 ;;
  esac
done

# Without a terminal, gum can't ask, so say what to pass instead.
need() { [ -t 0 ] || { echo "Pass $1" >&2; exit 1; }; }

if [ -z "$services" ]; then
  need --services
  services=$(gh api repos/acme/platform/contents/services --jq '.[].name' \
    | gum choose --no-limit --header "Which services?" | paste -sd, -)
fi
if [ -z "$env" ]; then
  need --env
  env=$(gum choose --header "Where to?" staging production)
fi
case "$env" in
  staging | production) ;;
  *) echo "--env must be staging or production" >&2; exit 1 ;;
esac
if [ -z "$token" ]; then
  need TOKEN
  token=$(gum input --password --placeholder "Cluster token")
fi

IFS=, read -ra picked <<< "$services"
for svc in "${picked[@]}"; do
  docker build -t "ghcr.io/acme/$svc:$env" "services/$svc"
  kubectl --token "$token" -n "$env" rollout restart "deploy/$svc"
done
```

</div>
</div>

Run in a terminal, both ask the same three questions. hobnob's first one:

```
$ hobnob deploy
deploy $ gh api repos/acme/platform/contents/services # output hidden
deploy ✓ 0.0s

╭─ deploy » SERVICES ──────────────────────────────────────────────────────╮
│                                                                          │
│  Which services?                                                         │
│                                                                          │
│  ▶ ○ api                                                                 │
│    ○ web                                                                 │
│    ○ worker                                                              │
│                                                                          │
╰──────────────────────────────────────────────────────────────────────────╯
 type to filter • ↑↓ move • tab toggle • enter confirm
```

In CI, both take the answers on the command line, with the token from the
environment:

<div class="side-by-side">
<div>

hobnob

```
$ hobnob deploy SERVICES='["api","web"]' ENV=staging
deploy $ gh api repos/acme/platform/contents/services # output hidden
deploy ✓ 0.0s
deploy » SERVICES=api, web # from the command line
deploy » ENV=staging # from the command line
deploy » TOKEN=**** # from the environment
deploy $ docker build -t ghcr.io/acme/api:staging services/api
deploy │ naming to ghcr.io/acme/api:staging done
deploy ✓ 0.2s
deploy $ kubectl --token **** -n staging rollout restart deploy/api
deploy │ deployment.apps/api restarted
deploy ✓ 0.0s
deploy $ docker build -t ghcr.io/acme/web:staging services/web
deploy │ naming to ghcr.io/acme/web:staging done
deploy ✓ 0.0s
deploy $ kubectl --token **** -n staging rollout restart deploy/web
deploy │ deployment.apps/web restarted
deploy ✓ 0.0s
deploy ✓ done after 0.3s
```

</div>
<div>

bash with gum

```
$ ./scripts/deploy.sh --services api,web --env staging
naming to ghcr.io/acme/api:staging done
deployment.apps/api restarted
naming to ghcr.io/acme/web:staging done
deployment.apps/web restarted
```

</div>
</div>

The differences:

- **Half the script is for CI.** The flag parsing, the `need` checks and the
  `case` on `--env` are there so the script can run without questions. In
  hobnob, a question and its command-line argument are the same thing, so
  there's nothing to write. Add a fourth question and the script grows a
  flag, a check and a `need`. hobnob grows one entry under `get:`.
- **In CI, passed answers are checked like typed ones.** `ENV=prod` fails,
  because it isn't one of the `options:`. So does `SERVICES='["billing"]'`, because
  it isn't in the list from GitHub. That's why hobnob fetches the list even
  when the answer is passed. The script checks `--env` by hand, and doesn't
  check `--services` at all.
- **The log says what ran.** hobnob prints each command before it runs, with
  the token as `****`. The script prints only what the commands print. Turn
  on `set -x` to see the commands, and the token is printed too.
- **The answers are a list.** gum prints one service per line, so the script
  joins them with commas and splits them again to loop. hobnob's `SERVICES`
  is a list, whether it's picked in the menu or passed on the command line
  as JSON (see [Text that looks like JSON](../guide/structured-data.md#text-that-looks-like-json)).
- **The same wizard runs in a browser.** `hobnob --browser deploy` asks the
  same three questions on a local web page, for someone who never opens a
  terminal (see [Browser mode](../guide/browser-mode.md)). The script only
  runs in a terminal.
- **gum has to be installed** on every machine where someone answers the
  questions. In CI, with every answer passed, neither needs anything but the
  commands themselves and, for hobnob, its binary.

## Where a script is the better pick

- **Logic.** Functions, arrays, `while` loops, retries, branches inside
  branches. hobnob's steps run top to bottom, with
  [`if:`](../guide/conditions.md), [`loop:`](../guide/loops.md) and
  [`call:`](../guide/calling-tasks.md). Past a certain point, that's more
  awkward than code.
- **More kinds of question.** gum also has a multi-line editor, spinners,
  styled text and a pager. hobnob has a text box, a menu, a multi-select,
  hidden input and a file or folder picker.
- **Nothing new.** If the script exists and works, and everyone who runs it
  has gum, there's little to gain from rewriting it.

So keep a script when the wizard is mostly logic with a question or two, or
when it's a one-off. Use hobnob when the wizard is mostly questions and
commands, when a team shares it, or when it also has to run in CI.

## Moving a script over

| bash with gum                               | hobnob                                                                          |
| ------------------------------------------- | ------------------------------------------------------------------------------- |
| `gum input`                                 | A [`get:`](../guide/prompts.md) entry                                           |
| `gum input --password`                      | `secret: true` (see [Secrets](../guide/secrets.md))                             |
| `gum choose a b`                            | `options: [a, b]`                                                               |
| `gum choose --no-limit`                     | `multi: true`                                                                   |
| `gum filter`                                | `options:`. Every menu filters as you type                                      |
| `gum file`                                  | `type: file` or `type: dir` (see [Picking a file or directory](../guide/prompts.md#picking-a-file-or-directory)) |
| `… \| gum choose`, from a command           | A `run:` step with [`into:`](../guide/capturing-output.md), then `options: .LIST` |
| `gum confirm`                               | A `get:` with `options: ["no", "yes"]`, then a step with `if:`                  |
| `echo` a result, then wait for enter        | [`say:`](../guide/prompts.md#showing-a-message)                                 |
| A `while`/`case` loop over flags            | Nothing. Pass `NAME=value`                                                      |
| `[ -t 0 ]` before asking                    | Nothing. hobnob [doesn't ask without a terminal](../guide/prompts.md#running-without-prompts) |
| `jq -r '.items[].name'`                     | `into:` with `stdout.items[*].name`                                             |
| A `for` loop                                | [`loop:`](../guide/loops.md)                                                    |
| A function used by several scripts          | A helper task with [`results:`](../guide/calling-tasks.md#returning-values-with-results) |

<p class="compare-checked">Last checked November 2026.</p>
