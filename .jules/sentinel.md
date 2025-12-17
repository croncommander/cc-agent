## 2024-04-12 - Cron Injection via Newlines
**Risk:** The agent generates cron files by interpolating string fields (`CronExpression`, `JobID`, `Command`) directly into a template. If any of these fields contain a newline (`\n`), an attacker controlling the job definition (e.g., a compromised server) can inject arbitrary cron entries, leading to Remote Code Execution as root or other users.
**Learning:** Text-based formats like Cron, where newlines are delimiters, are vulnerable to injection just like SQL or HTML. Simple string formatting is insufficient when handling untrusted input in such formats.
**Action:** Always validate that inputs destined for line-based configuration files do not contain newline characters. Use sanitization or rejection for invalid inputs.

## 2024-05-23 - Shell Injection via Unquoted Arguments in Cron Files
**Risk:** The `JobID` field was interpolated directly into the cron command line (`... --job-id %s ...`). Since cron executes commands via a shell (`/bin/sh` or specified `SHELL`), an attacker could inject shell metacharacters (e.g., `; rm -rf /`) into the `JobID` to execute arbitrary commands.
**Learning:** Even if a string is intended to be a simple identifier, if it passes through a shell (as all cron commands do), it must be quoted or strictly validated. Standard Go `exec.Command` safety does not apply when the entry point is a shell script or cron line.
**Action:** Use a `shellQuote` helper (wrapping in single quotes and escaping existing single quotes) for all data arguments interpolated into shell command strings.
