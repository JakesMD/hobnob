---
title: hobnob compared
description: How hobnob compares with a bash script, a custom CLI, Task, just and make, and when each is the better pick.
---

# hobnob compared

Each page builds the same wizard both ways, side by side. It shows where they
differ, and says when the other option is the better pick.

## Building a wizard

You'd otherwise write it as a script or a program:

- [hobnob vs a bash script with gum](./bash.md): the usual way, until half
  the script is flag parsing for CI.
- [hobnob vs a custom CLI](./cli.md): a real language and tests, and a
  program to build and install.

## Already using a task runner

You don't have to switch. Keep your task runner for what it's good at, and
write the wizards in hobnob. For the short version, see
[hobnob for task-runner users](/make-just-task). Each page ends with how to run
both:

- [hobnob vs Task](./task.md): keep Task for builds.
- [hobnob vs just](./just.md): keep just for shortcuts.
- [hobnob vs make](./make.md): keep make for building files.
