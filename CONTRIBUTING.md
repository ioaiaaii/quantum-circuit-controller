# Contributing

Contributions are welcome. This page covers the mechanics; the engineering
context lives in [docs/engineering.md](./docs/engineering.md).

## Before you start

- Read the [implementation status matrix](./docs/README.md#implementation-status)
  so you build on a surface that exists.
- Good entry points: the
  [known limitations](./docs/operations.md#known-limitations) list and the
  [provider adapter guide](./docs/engineering.md#adding-a-provider-adapter).
- For anything larger than a fix, open an issue first and describe the
  change; the interfaces (`qcc.io/v1alpha1`, the executor gRPC contract,
  the `qcc_*` metrics specification) evolve deliberately.

## Setup, build, test

```bash
make tools-install      # pinned toolchain via mise
make test               # Go unit + envtest suites
make executor-test      # Python tests
make lint executor-lint # golangci-lint, ruff
```

The full loops, including the local two-terminal dev setup, are in
[docs/engineering.md](./docs/engineering.md#working-on-qcc).

## Pull requests

- Keep changes focused; one concern per PR.
- CI must pass: tests, lint, executor, proto checks.
- CRD changes: run `make manifests generate` and commit the generated
  files; keep `v1alpha1` additive.
- gRPC changes: run `make proto-generate` and commit both language stubs;
  run `make proto-breaking` before opening the PR.
- Match the codebase conventions: rationale comments (why, not what), the
  terminal-versus-transient error rule, pure decision functions. See
  [docs/engineering.md](./docs/engineering.md#principles).
- Commit messages follow Conventional Commits (`feat:`, `fix:`, `docs:`,
  `test:`, `chore:`).

## Licensing of contributions

QCC is licensed under the [Apache License 2.0](./LICENSE). By submitting a
contribution, you agree that it is provided under the same license
(inbound = outbound). There is no CLA.

## Questions

Open a GitHub issue. For suspected security problems, do not open a public
issue; see [SECURITY.md](./SECURITY.md).
