# Upward reads between scope layers

A higher layer always overrides a lower one (`env < vars: < env files < CLI args < const:`), but a layer's templates read only the final values of layers above it, so layers are resolved top-down, in the reverse of precedence order. This lets `vars:` defaults be built from `.env`, CLI args and `const:` without letting them override any of those, and it cannot cycle because every reference points upward. The OS env is readable by every layer except `const:`, which stays a closed world, and a `vars:` entry whose name a higher layer already set is skipped without being evaluated.

## Considered Options

- **Resolve in precedence order (the previous behaviour).** Simple to read, but `vars:` could not see `.env` values.
- **Move `vars:` above env files.** `vars:` could see `.env`, but it would then override it, so `.env` could no longer override a default.
- **Upward reads (chosen).** `vars:` sees everything above it and still loses to it.

## Consequences

An `env:` path can no longer read a `vars:` default. `.env.{{.STAGE}}` driven by `vars: [STAGE: dev]` is a load-time error; select the file with a CLI arg, the OS env or `const:` instead.
