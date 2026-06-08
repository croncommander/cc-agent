# cc-agent Instructions

## Tests and compatibility

- Every new feature must include automated Go tests in the same change.
- Every bug fix must include a regression test when the behavior is testable.
- Run the relevant Go test suite before completing a change.
- Preserve core behavior: registration and liveness, cron discovery, managed
  cron synchronization, local daemon/exec IPC, command execution, durable
  execution-report delivery, output limits, and user/system execution modes.
- Protocol changes must be versioned. Before the first public release, direct
  cutovers are allowed when the agent, listener, specifications, tests,
  installer, and deployment configuration change together. After release,
  require an explicit compatibility and migration plan.

## Security and SOC 2 readiness

- Security is release-blocking. The agent runs near customer workloads, so a
  flaw in command execution, privilege handling, update delivery, credentials,
  file permissions, cron generation, or report handling can compromise a host.
- Preserve least privilege, strict file and socket permissions, bounded input
  and output, safe shell quoting, authenticated transport, idempotent delivery,
  secret redaction, and secure defaults.
- Add security and abuse-case tests for changes involving parsing, local IPC,
  commands, credentials, filesystem writes, networking, or privilege modes.
- We are not currently undergoing a SOC 2 audit and must not claim compliance.
  Keep changes traceable, testable, auditable, and suitable for future SOC 2
  controls from day zero.

## UX and CLI

- Keep CLI output and configuration simple, practical, and task-focused.
- Use a restrained Bauhaus-inspired approach for any visual or interactive
  surface: clear hierarchy, direct language, minimal decoration, and
  accessibility before novelty.
