# Security Policy

## Supported versions

Security fixes are applied to the latest release on the `main` branch.

## Reporting a vulnerability

Please **do not** open a public issue for security problems.

Use GitHub's private vulnerability reporting on the repository's **Security** tab. If that is unavailable, contact the maintainer through the GitHub profile [@shuaigehuang](https://github.com/shuaigehuang) and ask for a private channel.

Include:

- Affected component and version.
- Reproduction steps or a minimal proof of concept.
- Impact assessment.
- Any suggested fix.

Never include real desktop tokens, phone tokens, Magpie keys, provider API keys, server addresses, or full conversation logs.

## Security model

- The relay never stores plaintext phone tokens. It stores SHA-256 hashes in `sessions.json`.
- The desktop token is stored locally with Windows DPAPI.
- Phone tokens are stored with Android secure storage.
- The relay forwards only whitelisted RPC methods and cannot initiate commands.
- Codex conversation content is not persisted by the relay.
- Magpie secrets are never written to logs. Full gateway keys are returned only once when created or rotated.

## Plaintext HTTP

`--http-public` runs without TLS. In this mode the desktop token, phone token, conversation content, commands, and Magpie management results can be intercepted or modified by a network attacker. Use it only for temporary local testing. Production deployments should use `--domain` with a valid certificate and HTTPS.

## Operator responsibilities

- Keep the server OS, Go, Node.js, Flutter, Codex, and Magpie up to date.
- Restrict SSH access and use key authentication.
- Protect `/etc/codexlink-relay.env` and `/var/lib/codexlink-relay`.
- Review authorized phones regularly and revoke unused devices.
- Do not expose the relay directly to the internet with the desktop token embedded in scripts or logs.
