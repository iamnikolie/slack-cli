# Security Policy

## Supported versions

The latest release is the supported one. Fixes land on `main` and go out in the
next tag.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Use GitHub's private vulnerability reporting instead:
[Security → Report a vulnerability](https://github.com/iamnikolie/slack-cli/security/advisories/new).
That opens a private advisory visible only to the maintainers.

Include what you did, what happened, and the impact you think it has. Expect a
first response within a week — this is a spare-time project, not a product with
an on-call rotation.

## Scope notes

Some things are known and by design rather than vulnerabilities:

- **The token is a user token, stored in plain text** in
  `~/.slk/<profile>/config.yaml` at mode 0600 — the same posture as
  `~/.aws/credentials` or `.netrc`. A `xoxp-` token acts as you: it reads every
  channel you belong to and posts under your name. Anyone who can read your home
  directory can do the same. Use `SLK_TOKEN` from a secret manager if you need
  better than that.
- **`--verbose` prints request and response bodies to stderr.** The token is not
  logged, but message content is. Redact before pasting output into an issue.
- **Message and channel content is rendered as it arrives.** The CLI does not
  sanitize workspace content for the terminal, and workspace content is written
  by other people.
- **The directory cache on disk** (`slk sync`) holds channel and user names for
  your workspace. It is not encrypted.

Revoke a leaked token at api.slack.com/apps → your app → **OAuth & Permissions**
→ *Revoke All OAuth Tokens*, which invalidates it immediately.
