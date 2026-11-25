---
title: "hobnob vs Task (go-task): wizards next to your Taskfile"
titleTemplate: false
description: "How hobnob compares with Task: where Task is the better pick, the same wizard written side by side, and how to run both."
aside: false
pageClass: compare-page
---

# hobnob vs Task

[Task](https://taskfile.dev) and hobnob both run tasks written in YAML, and
both are a single Go binary. Task is built for builds: it skips work that's
done, runs tasks in parallel and watches files. hobnob is built for wizards:
tasks that ask, fetch and decide as they go.

You don't have to pick one. Keep the Taskfile for builds, and write the
wizards in hobnob.

## At a glance

|                                     | hobnob                                                  | Task                                                                 |
| ----------------------------------- | ------------------------------------------------------- | -------------------------------------------------------------------- |
| Skipping work that's already done   | No                                                      | `sources:`, `generates:` and `status:`                               |
| Watching files                      | No                                                      | `task --watch`                                                       |
| Running tasks at once               | Only background jobs                                    | `deps:`, with grouped or prefixed output                             |
| Cleaning up after a failure         | `soft:`, then a step that reacts                        | `defer:`                                                             |
| Taskfiles from elsewhere            | Local files only                                        | From a URL or a Git repository                                       |
| Windows                             | No, macOS and Linux                                     | Yes, with a shell built in                                           |
| Asking a question                   | A `get:` step, anywhere in the task                     | Before the task's commands, for any missing `requires:` variable     |
| A question that depends on an answer | The next step                                          | Another task                                                         |
| Using a command's output            | Any later step, through `into:`                         | `sh:` variables, all worked out before the first command             |
| Values back from another task       | `results:`                                              | None. Values only go down to the tasks it calls                      |
| JSON from a command                 | [Data](../guide/structured-data.md), as in `stdout.items[*].name` | Text, cut up with `jq`                                     |

## The same wizard

Roll a service back to an earlier release: ask which service, fetch that
service's releases, then ask which one to go back to.

<div class="side-by-side">
<div>

hobnob

```yaml
tasks:
  rollback:
    info: Roll a service back to an earlier release.
    steps:
      - run: kubectl get deploy -n prod -o json
        quiet: true
        into:
          - SERVICES: stdout.items[*].metadata.name
      - get:
          - SERVICE:
              info: Which service?
              options: .SERVICES
      - run: gh release list -R acme/{{.SERVICE}} --json tagName
        quiet: true
        into:
          - TAGS: stdout[*].tagName
      - get:
          - TAG:
              info: Roll {{.SERVICE}} back to?
              options: .TAGS
      - run: kubectl set image deploy/{{.SERVICE}} {{.SERVICE}}=ghcr.io/acme/{{.SERVICE}}:{{.TAG}} -n prod
```

```
$ hobnob rollback SERVICE=api TAG=v2.4.0
rollback $ kubectl get deploy -n prod -o json # output hidden
rollback ✓ 0.0s
rollback » SERVICE=api # from the command line
rollback $ gh release list -R acme/api --json tagName # output hidden
rollback ✓ 0.0s
rollback » TAG=v2.4.0 # from the command line
rollback $ kubectl set image deploy/api api=ghcr.io/acme/api:v2.4.0 -n prod
rollback │ deployment.apps/api image updated
rollback ✓ 0.0s
rollback ✓ done after 0.0s
```

</div>
<div>

Task

```yaml
version: '3'

tasks:
  rollback:
    desc: Roll a service back to an earlier release.
    vars:
      SERVICES:
        sh: kubectl get deploy -n prod -o json | jq -r '.items[].metadata.name'
    requires:
      vars:
        - name: SERVICE
          enum:
            ref: .SERVICES | splitLines | compact
    cmds:
      - task: _rollback-service
        vars:
          SERVICE: '{{.SERVICE}}'

  _rollback-service:
    internal: true
    vars:
      TAGS:
        sh: gh release list -R acme/{{.SERVICE}} --json tagName --jq '.[].tagName'
    requires:
      vars:
        - name: TAG
          enum:
            ref: .TAGS | splitLines | compact
    cmds:
      - kubectl set image deploy/{{.SERVICE}} {{.SERVICE}}=ghcr.io/acme/{{.SERVICE}}:{{.TAG}} -n prod
```

```
$ task rollback SERVICE=api TAG=v2.4.0
task: [_rollback-service] kubectl set image deploy/api api=ghcr.io/acme/api:v2.4.0 -n prod
deployment.apps/api image updated
```

</div>
</div>

Pass the answers and neither asks, so both run in CI. Leave them out and
hobnob asks. Here it's on the second question, with the first answered:

```
$ hobnob rollback
rollback $ kubectl get deploy -n prod -o json # output hidden
rollback ✓ 0.0s
rollback » SERVICE=api
rollback $ gh release list -R acme/api --json tagName # output hidden
rollback ✓ 0.0s
╭─ rollback » TAG ─────────────────────────────────────────────────────────╮
│                                                                          │
│  Roll api back to?                                                       │
│                                                                          │
│  ▶ v2.5.0                                                                │
│    v2.4.0                                                                │
│    v2.3.1                                                                │
│                                                                          │
╰──────────────────────────────────────────────────────────────────────────╯
 type to filter • ↑↓ move • enter select
```

Task can show menus for missing variables too, when run with
`--interactive` or with `interactive: true` in `~/.taskrc.yml`. Otherwise it
stops and names the missing variable.

The differences:

- **In Task, the second question needs a second task.** Task works out all
  of a task's variables before it asks anything. The releases depend on
  which service you pick, so they can't be fetched in the same task as the
  question. The rest of the work moves into `_rollback-service`, and the
  answer is passed along. Every further question that depends on an answer
  adds another task. In hobnob, each question sits where it's needed.
- **In Task, a command's output has to be a variable declared up front.**
  `sh:` is the only way to keep output, and Task runs every `sh:` before the
  task's first command. So no variable can hold what an earlier command made,
  such as an address that only exists after `terraform apply`. hobnob's
  [`into:`](../guide/capturing-output.md) keeps the output of any `run:`
  step, for the steps after it.
- **A helper can't hand an answer back.** A Task task can send values down
  to the tasks it calls, but not back up. A hobnob helper lists what it
  returns in
  [`results:`](../guide/calling-tasks.md#returning-values-with-results), so
  one "pick a service" helper can serve every wizard that needs it.
- **hobnob's log shows every command.** Task doesn't print its `sh:`
  commands, so its log above shows only the last one. When `gh` fails,
  hobnob's log says which step it was.

The split also goes away if the whole rollback is one shell script in a
single `cmds:` entry, with [gum](https://github.com/charmbracelet/gum) or
[fzf](https://github.com/junegunn/fzf) for the menus. That has the costs of
any [wizard written as a script](./bash.md).

## Where Task is the better pick

- **Builds.** With `sources:` and `generates:`, Task skips a task when its
  files haven't changed, and `status:` skips it when a check says it's done.
  `task --watch` reruns a task when its files change.
- **Running tasks at the same time.** Task runs a task's `deps:` in parallel,
  and can group or prefix each task's output to keep it readable. hobnob runs
  steps one after another, and overlaps work only through
  [background jobs](../guide/commands.md#background-jobs), which keep running
  while later steps run.
- **Cleaning up after a failure.** A `defer:` command runs when the task
  ends, even if it failed.
- **Windows.** Task runs on Windows and has a shell built in. hobnob is
  released for macOS and Linux, and runs its commands with `sh`.
- **Taskfiles from elsewhere.** Task can include a taskfile from a URL or a
  Git repository. hobnob's [modules](../guide/modules.md) are local files.

## Running both

Each can call the other. A Taskfile task can hand over to a hobnob wizard,
passing along anything after `--`:

```yaml
version: '3'

tasks:
  rollback:
    desc: Roll a service back to an earlier release.
    cmds:
      - hobnob rollback {{.CLI_ARGS}}
```

```bash
task rollback                                # asks
task rollback -- SERVICE=api TAG=v2.4.0      # doesn't
```

And a hobnob wizard can use a Task build, which still skips what's done:

```yaml
tasks:
  release:
    steps:
      - get:
          - ENV:
              options: [staging, production]
      - run: task build
      - run: ./scripts/deploy.sh {{.ENV}}
```

## If you move a task over

| Task                            | hobnob                                                                         |
| ------------------------------- | ------------------------------------------------------------------------------ |
| `desc:`                         | [`info:`](../guide/tasks.md#describing-a-task)                                 |
| `cmds:`                         | `steps:`, each a [`run:`](../guide/commands.md)                                |
| `vars:` at the top              | [`vars:`](../guide/variables.md#variables-for-the-whole-file), or `const:` for values nothing may override |
| A variable with `sh:`           | A `run:` step with [`into:`](../guide/capturing-output.md)                     |
| `dotenv:`                       | [`env:`](../guide/variables.md#loading-variables-from-a-file)                  |
| `includes:`                     | [`modules:`](../guide/modules.md)                                              |
| `internal: true`                | A task name starting with `_` (see [Helper tasks](../guide/tasks.md#helper-tasks)) |
| `deps:`                         | [`call:`](../guide/calling-tasks.md) steps, which run one after another        |
| `- task: other` with `vars:`    | `- call: other` with `with:`                                                   |
| `requires:`                     | A [`get:`](../guide/prompts.md) step                                           |
| `run: once`                     | [`once: true`](../guide/running-once.md)                                       |
| `ignore_error: true`            | [`soft: true`](../guide/errors.md)                                             |
| `for:`                          | [`loop:`](../guide/loops.md)                                                   |
| `if:`                           | [`if:`](../guide/conditions.md), a shell condition                             |
| `secret: true`                  | [`secret: true`](../guide/secrets.md)                                          |
| `task build ENV=prod`           | `hobnob build ENV=prod`                                                        |

<p class="compare-checked">Last checked November 2026.</p>
