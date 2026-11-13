# Guide

## Basics

What a task is, how to run one, and how its commands run.

- [Tasks](./tasks.md): what a task is, the kinds of step, helper tasks and naming.
- [Running tasks](./running-tasks.md): running by name, the menu, passing variables and `--file`.
- [Running commands](./commands.md): text and list commands, other directories, hiding output and background jobs.

## Values

What a value is, and how to use one.

- [Variables](./variables.md): `set:`, `const:`, `vars:`, env files and precedence.
- [Templates](./templates.md): `{{ }}`, filters and bare references.
- [Structured data](./structured-data.md): arrays and objects, accessors and spreading.
- [Secrets](./secrets.md): mask a value wherever hobnob prints it.

## Getting values

Three ways a step produces a value: from a command, from another task, or from the user.

- [Capturing output](./capturing-output.md): keep what a command printed with `into:`.
- [Calling tasks](./calling-tasks.md): run another task and bring values back.
- [Prompts](./prompts.md): ask for a value with `get:`, and run without asking.

## Control flow

Deciding which steps run, how often, and what happens when one fails.

- [Conditions](./conditions.md): skip a task or a step with `if:`.
- [Loops](./loops.md): run steps once per item, entry or combination.
- [Errors](./errors.md): keep going past a failure with `soft:`, and react to it.

## Scaling up

Sharing work between callers, and splitting tasks across files.

- [Running once](./running-once.md): share one run of a task between every caller.
- [Modules](./modules.md): import another taskfile under a namespace.
