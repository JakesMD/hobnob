# Loops

A `loop:` step runs the steps nested under it once for each item in a list.
Each time round, `ITEM` holds the current item:

```yaml
tasks:
  list-categories:
    steps:
      - loop: [history, science]
        steps:
          - run: echo "category {{.ITEM}}"
```

```
$ hobnob list-categories
run: [list-categories] echo "category history"
[list-categories] category history
run: [list-categories] echo "category science"
[list-categories] category science
✓ [list-categories] done
```

The list can come from a variable, so a loop can go over data an earlier
step fetched:

```yaml
tasks:
  sample-categories:
    steps:
      - run: curl -s https://the-trivia-api.com/v2/metadata
        into:
          - CATEGORIES: stdout.byCategory | keys
      - loop: .CATEGORIES
        steps:
          - run: curl -s "https://the-trivia-api.com/v2/questions?limit=1&categories={{.ITEM}}"
```

## Looping over a map

Loop over a map and each entry gives you `KEY` and `VALUE`:

```yaml
tasks:
  show-labels:
    steps:
      - set:
          - LABELS: { history: History buffs, science: Lab coats }
      - loop: .LABELS
        steps:
          - run: echo "{{.KEY}} = {{.VALUE}}"
```

```
$ hobnob show-labels
[show-labels] history = History buffs
[show-labels] science = Lab coats
```

Entries come in order of their keys, sorted.

## Every combination

Give `loop:` several named lists, and the steps run once for every
combination. Each name holds its current value:

```yaml
tasks:
  sample-grid:
    steps:
      - loop:
          CATEGORY: [general_knowledge, science]
          COUNT: [1, 3]
        steps:
          - run: echo "{{.CATEGORY}} {{.COUNT}}"
```

```
$ hobnob sample-grid
[sample-grid] general_knowledge 1
[sample-grid] general_knowledge 3
[sample-grid] science 1
[sample-grid] science 3
```

The last name changes fastest.

## After the loop

Variables set inside a loop are still there after it ends:

```yaml
tasks:
  last-category:
    steps:
      - loop: [history, science]
        steps:
          - set:
              - LAST: "{{.ITEM}}"
      - run: echo "{{.LAST}}" # science
```

`ITEM` itself isn't. Once the loop ends, it goes back to whatever it was
before, or to unset. So do `KEY`, `VALUE` and the names of a combination
loop.

## Rules

- **An empty list runs the steps zero times.**
- **Looping over text doesn't split it.** The steps run once, with `ITEM` as
  the whole text. Turn JSON text into a list with `| json` first (see
  [Text that looks like JSON](./structured-data.md#text-that-looks-like-json)).
- **In a loop inside a loop, the inner `ITEM` hides the outer one** until
  the inner loop ends. To use both values at once, give the loops their own
  names with a combination loop instead.
- **A loop's names can't be [`const:`](./variables.md#variables-for-the-whole-file)
  names**, or the file fails to load. A loop over a written list uses
  `ITEM`, a combination loop uses its own names, and a loop over a variable
  counts as using `ITEM`, `KEY` and `VALUE`, since the variable might hold a
  map.
- **`loop:` takes `if:`**, which skips the whole loop. It doesn't take
  `soft:`; put that on the steps inside (see [Errors](./errors.md)).
