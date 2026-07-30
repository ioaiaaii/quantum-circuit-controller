# Security Policy

## Supported versions

QCC is a research proof of concept. Only the latest release on the
`v1.0.x` line receives fixes.

## Threat model, stated plainly

Read the [security posture](./docs/operations.md#security-posture) before
deploying. In short: QCC v1.0.x assumes a single-tenant, trusted cluster.
Circuit sources are executed as code inside the executor pod, the
controller-executor gRPC channel is plaintext and unauthenticated inside
the cluster, and one IBM credential serves the whole cluster. Reports that
restate these documented assumptions are not vulnerabilities; reports that
show an escape beyond them are.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository
("Report a vulnerability" under the Security tab). Do not open a public
issue for suspected vulnerabilities.

Include what you observed, a reproduction path, and the impact you
believe it has. You can expect an acknowledgement within a week; fixes
are best-effort, in the open, and credited unless you prefer otherwise.
