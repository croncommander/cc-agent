## Why CronCommander Doesn’t Block "Dangerous" Commands
CronCommander is designed to execute commands on servers—reliably, remotely, and automatically. 
That power naturally raises an important question: If CronCommander can run arbitrary commands, how do you prevent remote code execution (RCE)?
The short answer is: we don’t try to guess which commands are "safe". We design the system so that even if a command is risky, its impact is strictly limited.

It’s tempting to prevent RCE by blocking characters like: ; | && || $( ) ` > <
Unfortunately, this approach fails in practice:
- These constructs are fundamental to real automation
- Attackers can trivially bypass filters
- Legitimate workflows break
- The system gains false confidence without real protection

Cron itself does not attempt to sanitize commands. Historically, it assumes: "If you can schedule jobs, you are trusted."
Metacharacter blocking is a classic "looks secure, breaks the product" change.

## 2025-12-18 - Cron Shell Injection via Unquoted Command Strings
**Risk:** The previous fix for injection used `shellQuote` but did not apply it to the `Command` payload itself, and the `Command` was not wrapped in an explicit shell invocation. This allowed metacharacters in the `Command` string to be interpreted by the cron shell, potentially allowing attackers to execute arbitrary commands outside the intended execution scope.
**Learning:** When passing a "command string" through an intermediate shell (like cron) to an execution wrapper, the safest pattern is to quote the command string and pass it as an argument to `sh -c`. This ensures the cron shell sees it as a single argument, and the inner shell executes it as intended, isolating the execution environments.
**Action:** Wrap cron commands in `/bin/sh -c '<quoted_command>'` and ensure all other arguments (like JobID) are strictly shell-quoted.

## 2025-12-26 - Insecure Socket Fallback to /tmp
Risk: When the secure directory (`/var/lib/croncommander`) was missing, the agent silently fell back to creating its IPC socket in `/tmp`. In multi-user environments, this allowed local attackers to pre-create the socket (DoS) or spoof the daemon to intercept execution reports containing sensitive job details (Information Disclosure).
Learning: "Convenient" fallbacks in security-critical paths often create hidden "fail-open" vulnerabilities. Configuration errors (like a missing directory) should result in a hard failure ("Fail Secure") rather than a silent security downgrade.
Action: Removed the implicit fallback to `/tmp`. The agent now strictly enforces the use of the secure directory and will fail to start if it is missing or inaccessible.

## 2025-12-29 - Socket Creation Race Condition
Risk: The `net.Listen` function creates the socket file using the process's default umask (often `0022` or `0002`). This creates a small window where the socket file might be accessible to unauthorized users before `os.Chmod` is called to restrict permissions. In environments with shared groups or permissive umasks, this could allow local attackers to connect to the socket.
Learning: `net.Listen` does not accept permissions as an argument. Relying on a subsequent `os.Chmod` introduces a Time-of-Check Time-of-Use (TOCTOU) vulnerability. The only atomic way to control file creation permissions in standard Go `net` package is to modify the process `umask` prior to the call.
Action: Wrap `net.Listen` with `syscall.Umask(0117)` to strictly enforce `0660` permissions at the moment of creation, then restore the original umask.
## 2025-12-27 - Unbounded Socket Read DoS (OOM)
Risk: The daemon's internal socket listener (`handleSocketConnection`) read unlimited data from incoming connections before unmarshalling JSON. A local authenticated attacker (e.g., a compromised `ccrunner` user) could cause a Denial of Service (DoS) by sending a massive payload (e.g., gigabytes of data), forcing the daemon to allocate excessive memory and potentially triggering an Out-Of-Memory (OOM) crash.
Learning: Never trust the size of incoming data, even from "trusted" local users. `json.Decoder` reads from the stream until it finds a valid object or error, but it buffers data. Without an `io.LimitReader`, a decoder can be coerced into reading indefinitely.
Action: Implemented a strict 1MB read limit (`io.LimitReader`) on the socket connection before passing it to the JSON decoder. This is sufficient for legitimate execution reports (stdout/stderr are capped at 256KB each) but prevents memory exhaustion attacks.

## 2026-01-23 - Insecure Socket Directory in /tmp
Risk: The agent's non-root fallback placed its IPC socket directly in `/tmp` (`/tmp/cc-agent-<user>.sock`). Since `/tmp` is shared, this allowed local attackers to pre-create the socket (DoS) or manipulate it. Permissions on the file itself were insufficient as the parent directory was shared and unverified.
Learning: Placing security-sensitive files (sockets, pidfiles) directly in shared directories like `/tmp` is unsafe. Always use a private subdirectory with `0700` permissions to prevent pre-creation attacks and race conditions.
Action: Enforced a dedicated, private subdirectory (`/tmp/cc-agent-<uid>/`) for the socket with strict `0700` permissions and ownership checks on both creation (daemon) and connection (exec).
