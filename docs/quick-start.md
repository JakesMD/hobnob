# Quick start

## Write a taskfile

Create `hobnob.yml` in your project. A task is a name and a list of steps:

```yaml
tasks:
  ask-trivia:
    info: Ask a random trivia question.
    steps:
      - get:
          - CATEGORY:
              info: Which category?
              options: [general_knowledge, history, science]
      - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.CATEGORY}}"
        into:
          - QUESTIONS: stdout
      - run: echo "{{ .QUESTIONS[0].question.text }}"
```

## Run it

```bash
hobnob ask-trivia                     # asks for a category
hobnob ask-trivia CATEGORY=history    # no prompt
hobnob                                # the picker
```

## Next steps

- The [guide](./guide/index.md) builds on this one topic at a time.
- The [reference](./reference/index.md) has every field, filter and flag.
