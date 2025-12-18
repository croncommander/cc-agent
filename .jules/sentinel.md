## 2025-12-17 - Cron Injection via Newlines
**Risk:** The agent generates cron files by interpolating string fields (`CronExpression`, `JobID`, `Command`) directly into a template. If any of these fields contain a newline (`\n`), an attacker controlling the job definition (e.g., a compromised server) can inject arbitrary cron entries, leading to Remote Code Execution as root or other users.
**Learning:** Text-based formats like Cron, where newlines are delimiters, are vulnerable to injection just like SQL or HTML. Simple string formatting is insufficient when handling untrusted input in such formats.
**Action:** Always validate that inputs destined for line-based configuration files do not contain newline characters. Use sanitization or rejection for invalid inputs.
