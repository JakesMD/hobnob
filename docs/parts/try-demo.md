<TryDemo>
<template #deploy>

```yaml
tasks:
  deploy:
    info: Build services and roll them out to an environment.
    steps:
      # Ask GitHub which services exist. JSON in, a list out.
      - run: gh api repos/acme/platform/contents/services
        into:
          - ALL_SERVICES: stdout[*].name

      # Three prompts: a multi-select, a menu and a masked token.
      - get:
          - SERVICES:
              info: Which services should be deployed?
              options: .ALL_SERVICES
              multi: true
          - ENV:
              info: Which environment should they go to?
              options: [staging, production]
          - TOKEN:
              info: "Cluster token for {{.ENV}}"
              secret: true

      # Once per service you picked. The token never shows.
      - loop: .SERVICES
        steps:
          - run: docker build -t ghcr.io/acme/{{.ITEM}}:{{.ENV}} services/{{.ITEM}}
          - run: kubectl --token {{.TOKEN}} -n {{.ENV}} rollout restart deploy/{{.ITEM}}
```

</template>
<template #flash>

```yaml
tasks:
  build-and-flash:
    info: Build an OS image with Packer and flash it to a disk.
    steps:
      # Ask everything up front, then leave it to run.
      - call: _pick-image
      - call: _pick-disk
      - call: build
      - call: flash

  build:
    info: Build an OS image with Packer.
    steps:
      - call: _pick-image
      - run: packer build -var out=output/os.img {{.IMAGE}}

  flash:
    info: Flash the built image to a disk.
    steps:
      - call: _pick-disk
      - run: sudo wipefs --all /dev/{{.DISK}}
      - run: sudo dd if=output/os.img of=/dev/{{.DISK}} bs=4M status=progress conv=fsync

  _pick-image:
    once: true
    results: [IMAGE]
    steps:
      # Opens in images/ and offers only Packer templates.
      - get:
          - IMAGE:
              info: Which Packer template should be built?
              type: file
              default: images/
              match: "*.pkr.hcl"

  _pick-disk:
    once: true
    results: [DISK]
    steps:
      # Removable disks only, so the laptop's own drive is never offered.
      - run: lsblk -dno NAME,SIZE,MODEL,RM | sed -n 's/ *1$//p'
        into:
          - DISKS: stdout | lines
      - get:
          - DISK:
              info: Which disk should the image be flashed to?
              options: .DISKS
          - SURE:
              info: "Wipe '{{.DISK}}'? Everything on it goes."
              options: ["no", "yes"]
      - run: '[ "{{.SURE}}" = yes ]'
      - set:
          - DISK: '{{ (.DISK | split " ")[0] }}'
```

</template>
<template #trivia>

```yaml
tasks:
  ask-trivia:
    info: Answer a trivia question from a category you pick.
    steps:
      # Query an API. The response is a value now.
      - run: curl -fsS https://the-trivia-api.com/v2/categories
        into:
          - CATEGORIES: stdout

      # The menu is built from what that step returned.
      - get:
          - CATEGORY:
              info: Which trivia category?
              options: .CATEGORIES | keys

      # Look up the chosen category's id by name.
      - run: curl -fsS "https://the-trivia-api.com/v2/questions?limit=1&categories={{ .CATEGORIES[.CATEGORY][0] }}"
        into:
          - TRIVIA: stdout[0]

      # Shuffle the answers, so the right one isn't always first.
      - run: [sh, -c, 'printf "%s\n" "$@" | sort -R', answers, .TRIVIA.correctAnswer, .TRIVIA.incorrectAnswers]
        into:
          - ANSWERS: stdout | lines

      # Dot straight into the JSON. No parsing step.
      - get:
          - GUESS:
              info: "{{ .TRIVIA.question.text }}"
              options: .ANSWERS

      # Show the result and wait for Enter.
      - say: Correct!
        if: "{{ eq .GUESS (.TRIVIA.correctAnswer | trim) }}"
      - say: "Not quite. It was {{ .TRIVIA.correctAnswer | trim }}."
        if: "{{ ne .GUESS (.TRIVIA.correctAnswer | trim) }}"
```

</template>
</TryDemo>
