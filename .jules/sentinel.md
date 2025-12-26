2025-12-26 - Insecure Socket Fallback to /tmp
Risk: When the secure directory (`/var/lib/croncommander`) was missing, the agent silently fell back to creating its IPC socket in `/tmp`. In multi-user environments, this allowed local attackers to pre-create the socket (DoS) or spoof the daemon to intercept execution reports containing sensitive job details (Information Disclosure).
Learning: "Convenient" fallbacks in security-critical paths often create hidden "fail-open" vulnerabilities. Configuration errors (like a missing directory) should result in a hard failure ("Fail Secure") rather than a silent security downgrade.
Action: Removed the implicit fallback to `/tmp`. The agent now strictly enforces the use of the secure directory and will fail to start if it is missing or inaccessible.
