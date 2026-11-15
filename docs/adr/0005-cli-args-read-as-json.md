# Command-line arguments are read as JSON

A `KEY=VALUE` argument used to always be text, so `hobnob deploy SERVICES='["api","web"]'` needed `| json` wherever the task used `SERVICES` as a list. Someone who types a JSON list on the command line means a list, and `run: into:` already reads a command's JSON output as data. Command-line arguments now go through the same rule (`value.Capture`): a value that decodes cleanly as a JSON list or object becomes data, and anything else stays text. Env files and the shell's environment stay text.

## Considered Options

- **Keep every outside value as text.** Predictable, but every task taking a list from the command line needs `| json`, and a `multi:` prompt's answer was a list when picked but text when passed in.
- **Read the shell's environment and env files as JSON too.** The most uniform, but the shell's environment holds names nobody set for hobnob, so a value changing type would surprise its user, and other tools read the same `.env` files as text.
- **Command-line arguments only (chosen).** The value is typed on purpose, for this run, by the person running the task.

## Consequences

A command-line value that happens to be a valid JSON list or object, and that a task treated as text (`eq .X "[1]"`, `len`, `trim`), now gets data instead. It still prints exactly as typed, because `Capture` keeps the raw text, so passing JSON on to another command is unchanged. `| json` on data does nothing, so existing tasks that use it keep working. A `multi:` answer is now always a list. Its preset or default must be a list, empty, or text that parses as a JSON list (as from the shell or an env file); other text is an error rather than a list of one.
