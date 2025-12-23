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
