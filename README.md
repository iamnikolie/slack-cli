# slack-cli (`slk`)

[![CI](https://github.com/iamnikolie/slack-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/iamnikolie/slack-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamnikolie/slack-cli.svg)](https://pkg.go.dev/github.com/iamnikolie/slack-cli)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

An agent-facing Slack CLI. Replaces a Slack MCP server inside Claude Code and
agent workflows: the agent runs `slk <command>` from Bash and reads token-lean
output (rendered transcripts/tables by default, `--json`/`--format` for
machines).

Sibling of [`fibery-cli`](https://github.com/iamnikolie/fibery-cli),
[`gitlab-cli`](https://github.com/iamnikolie/gitlab-cli) and
[`dziga`](https://github.com/iamnikolie/dziga): same doctrine — plain text on
stdout, nothing costs context until it is called.

> Unofficial, community-built tool. Not affiliated with, endorsed by, or supported by Slack Technologies.

Binary name is `slk` — no PATH conflict with the official `slack` CLI.

## Install

**Homebrew:**

```bash
brew trust iamnikolie/tap   # Homebrew 6 refuses untrusted third-party taps
brew tap iamnikolie/tap
brew install iamnikolie/tap/slack-cli
```

**Prebuilt binary** — download the archive for your platform from
[Releases](https://github.com/iamnikolie/slack-cli/releases), then:

```bash
tar xzf slack-cli_*_darwin_arm64.tar.gz
sudo mv slk /usr/local/bin/
```

**With Go** (1.23+):

```bash
go install github.com/iamnikolie/slack-cli@latest
```

**From source** — `make install` symlinks the binary, so a later `make build`
updates the installed CLI without reinstalling:

```bash
git clone https://github.com/iamnikolie/slack-cli.git
cd slack-cli
make install        # symlink → ~/.local/bin/slk
```

Check what you got with `slk version`.

## Setup

Create a Slack app and use a **user token** (`xoxp-…`); `search.messages`
requires one, and a user token sees every channel you are in without per-channel
invites.

1. Go to https://api.slack.com/apps → **Create New App** → **From scratch**.
2. Name it (e.g. `slk`), pick your workspace.
3. **OAuth & Permissions** → **Scopes** → **User Token Scopes**, add:
   `channels:read groups:read im:read mpim:read channels:history groups:history
   im:history mpim:history chat:write search:read users:read files:read files:write`.
   Optional, per feature: `reactions:write` (`react`), `im:write` (`dm` to
   someone you have no DM with yet), `pins:read` and `bookmarks:read`
   (`channel info`). Commands report a missing optional scope by name.
4. **Install to Workspace** → **Allow**.
5. Copy the **User OAuth Token** (`xoxp-…`).

Then save it (no default profile — every command needs `--config`):

```bash
slk --config work config init --token xoxp-...
# or: echo xoxp-... | slk --config work config init
```

Config is written to `~/.slk/work/config.yaml` (mode 0600). The token may also
come from the `SLK_TOKEN` env var. Each profile is isolated under `~/.slk/<name>/`.

That file holds a user token in plain text at mode 0600 — the same posture as
`~/.aws/credentials` or a `.netrc`. A `xoxp-` token acts as you: it can read
every channel you are in and post as you. Grant only the scopes listed above,
keep the file out of repositories and dotfile backups, and use `SLK_TOKEN` from
a secret manager if you need better. Revoke at api.slack.com/apps → your app →
**OAuth & Permissions** → *Revoke All OAuth Tokens*.

> Enterprise/managed workspaces may require an admin to approve app installs.
> If **Install** is blocked, ask a workspace admin.

## Profiles

`--config <name>` is **required** (or set `SLK_CONFIG`); there is no default
profile. Exceptions: `slk skill`, `slk version`, `slk --help`.

```bash
slk --config work me
SLK_CONFIG=work slk channels --filter eng
```

## Reference forms

Commands accept whatever you have:

- `#general` or `general` — channel by name (resolved from the directory cache).
- `@mako` or `mako` — user by name / real name.
- `C0123…` / `U0123…` — raw Slack IDs.
- A Slack permalink (`…/archives/C…/p17000000001234`) — carries channel + ts.
- A raw `ts` (`1700000000.000100`).

Names resolve from a disk cache (`~/.slk/<ws>/directory.json`). On a miss the
cache is refetched once. `slk sync` refreshes it manually.

## Command reference

Run `slk skill` for the full agent-facing reference, or `slk <command> --help`.

### Plumbing
- `slk config init` — save the user token (`--token` / `SLK_TOKEN` / stdin).
- `slk config show` — active profile, base URL, token state (masked).
- `slk me` — authenticated identity (`auth.test`).
- `slk sync` — refresh the channel + user directory cache.
- `slk api <method.name>` — raw Web API (`-f key=value`, repeatable).
- `slk skill` / `slk version`.

### Channels
- `slk channels` (alias `channel list`) — `--types public,private,im,mpim`,
  `--filter`, `--limit`. DMs (im) list under the partner's handle.
- `slk channel view <#chan|id>` — metadata.
- `slk channel info <#chan|id>` — topic, purpose, members, creator, bookmarks
  and pinned messages (oldest first); a section whose scope is missing is reported, not fatal.
- Listings include `type` (`public`, `private`, `im`, `mpim`).
- `slk activity --limit 200` — overview grouped by channel, newest first:
  latest indexed message date, type, preview, permalink, and sampled count.
  `--since YYYY-MM-DD` narrows the search; default includes all dates.
  The limit counts searched messages, not channels. Counts describe the sample,
  not total channel traffic; search visibility/indexing may omit messages.
  Requires `search:read`. JSON includes query, sample size, total matches,
  truncation status, and channel rows.

### Catching up
- `slk unread [#chan...] --since 7d` — messages past your channel read marker,
  plus threads you follow that have unread replies (only those replies). With no
  channels, active ones are discovered through search. Read-only: nothing is
  marked read.
- `slk mentions --since 7d` — messages that mention you.
- `slk digest [#chan...] --since 1d` — one transcript across channels with
  threads inline (`--replies=false` to collapse them).
- `slk tail <#chan...>` — only what is new since the previous `tail` of each
  channel; cursors live in `~/.slk/<profile>/cursors.json` (`--peek` leaves
  them). A channel's first tail starts at `--since` (default 1d). The cursor
  follows top-level messages, so new replies in older threads show up in
  `unread`, not `tail`.

### Reading
- `slk history <#chan|@user|id>` — `--limit` (latest N), `--since`/`--until`,
  `--oldest`/`--latest` (ts). Printed oldest first; `--desc` flips. Threads
  collapse to `↳ N replies (ts=…)`.
  - `--replies` expands every thread inline under its parent (one
    `conversations.replies` call per thread; the latest `--replies-limit N`,
    default 50, after `↳ +K earlier replies: slk thread …`). JSON nests them under `replies`;
    CSV/TSV/table emit one row per message, text expanded and `--max-chars`
    capped as in the transcript.
  - `--thread <ts|permalink>` reads one thread instead of the channel (same as
    `slk thread`); a thread permalink as the only argument implies it.
- `slk thread <#chan|id> <ts|permalink>` — all replies in a thread
  (`--limit` keeps the latest N, default 200; `--since`/`--until` window long ones).
- `slk get <permalink | #chan ts> [--context N]` — one message, marked `»`, with
  N messages on each side; a top-level message shows its replies, a reply shows
  its thread around it.
- A reply's permalink (`…?thread_ts=<parent>`) resolves to its parent wherever
  a thread is expected (`thread`, `history --thread`, `reply`, `send --thread`,
  `files upload --thread`).
- Times: `2h`, `3d`, `1w`, `today`, `yesterday`, `YYYY-MM-DD[ HH:MM]` (local
  time), RFC3339, epoch seconds, or a ts.
- Plain transcripts include profile/channel headings, full dates with local UTC
  offsets, `ts=` in each header, decoded markup (`@name`, `#channel`,
  `label (url)`, unescaped `&<>`), an `(edited)` mark, files, attachments and
  link unfurls (`▸`), reactions, and message links. Bot messages with empty text
  fall back to their Block Kit text, or to their first attachment when they
  have only attachments. A link Slack displays as its own shortened URL prints
  once, as the full URL. Linked files (Google Drive) show as `external`.
  `thread --since/--until` with no replies in the window says so.
  `--no-links` drops link lines (and swaps `permalink` for `ts` in
  `search`/`mentions` tables),
  `--max-chars N` caps each message. Link generation uses one `auth.test`
  lookup per run; failure emits a warning and leaves messages readable.

### Posting
- Text is sent as standard Markdown (`markdown_text`: `**bold**`,
  `[label](url)`, lists, code; 12,000 characters max). `--mrkdwn` sends Slack's
  own mrkdwn in `text` instead.
- `--dry-run` (any write command) prints the resolved channel, thread, format
  and text without sending.
- `slk send <#chan|@user|id> <text>` — `--thread <ts|permalink>`, `--at`,
  `--body-file -`, `--id-only`.
- `slk reply <#chan|id> <ts|permalink> <text>` — sugar for a thread reply.
- `slk dm <@user> <text>` — the DM is opened when none exists (`im:write`).
- `--at` on send/reply/dm schedules the message: `+2h`, `in 30m`, `16:00` (next
  occurrence), `tomorrow 09:00`, `YYYY-MM-DD HH:MM`. `slk scheduled [#chan]`
  lists pending ones; `slk scheduled delete <#chan> <id> --yes` cancels.
- `slk update <#chan|id> <ts> <text>` — edit your own message.
- `slk delete <#chan|id> <ts>` — requires `--yes`; your own message only.
- `slk react <permalink | #chan ts> <:emoji:>` (`--remove`, or `slk unreact`) —
  needs `reactions:write`; a reply's permalink reacts on the reply.

### Attachments

```bash
slk --config work files download F01234567
slk --config work files download 'https://acme.slack.com/files/U01234567/F01234567/screenshot.png' -o ./screenshot.png
slk --config work --json files download F01234567 -o ./downloads/
slk --config work files upload ./report.pdf -c '#general' --comment 'Weekly report'
slk --config work files upload a.png b.png -c '#design' --thread 1700000000.000100
echo 'details' | slk --config work files upload ./log.txt -c @alice --comment-file -
```

`files upload` requires `files:write`. It uses Slack's external upload flow
(`files.getUploadURLExternal` → POST bytes → `files.completeUploadExternal`).
The pre-signed upload URL never receives the OAuth token and must be a Slack file
host. With `--channel` all files are shared in one message (optionally in
`--thread`, with `--comment`/`--comment-file`); without it they stay private to
you. All paths are checked (exist, regular, non-empty) before any upload starts.

Requires `files:read`. The command calls `files.info`, then downloads the original
using `url_private_download` (or `url_private`) with the profile's bearer token.
Transcripts show attachment IDs and names. A message permalink may contain multiple
files: read its thread first and select a file ID.

Default destination is `<file-id>-<name>` in the current directory. `-o` accepts a
file path or an existing directory. Downloads use mode 0600, publish only after
completion, and never overwrite existing paths. JSON reports path, bytes, MIME
type, file ID, and permalink. Slack-hosted files are supported; external provider
files and Slack documents without private download URLs are reported explicitly.
OAuth credentials are restricted to Slack file hosts and removed on cross-origin
redirects. HTML sign-in responses are not saved as screenshots.

If Slack reports `missing_scope`, the error includes `needed` and `provided` scopes
when available. Open [Slack Apps](https://api.slack.com/apps), select the app, add
the required **User Token Scopes** under **OAuth & Permissions**, then **Reinstall
to Workspace** and approve. Save the resulting User OAuth Token via stdin using
`slk --config work config init`. Update/unset `SLK_TOKEN` if set, as it overrides
the saved token. Admin approval may be needed. A download HTTP 403 alone does not
prove a missing scope: also check file/channel visibility, workspace/profile,
Slack Connect, and workspace policy.

See Slack's [file authentication requirements](https://docs.slack.dev/reference/objects/file-object/#access-control--sharing)
and [files.info scopes](https://docs.slack.dev/reference/methods/files.info/).

### Search
- `slk search [query]` — `--in #chan`, `--from @user|me`, `--to @user|me`,
  `--since`/`--until`/`--on` (days; `--since` includes its own day), `--has
  link|pin|reaction|:emoji:` (repeatable), `--thread-only`, `--limit` (walks
  result pages, 100/page). Filters work without a query. Native Slack operators
  (`in:`, `from:`, `after:`) also work inside the query. Rendered text is
  decoded like transcripts.
- `--sort-dir asc|desc` controls timestamp order (default `desc`). Rendered
  results include dates and channel names.
- `--limit` walks pages, accepts 1–10000, and keeps a fixed page size (up to 100).
  Stderr always reports `total`, `returned`, `has_more`, and `has_next_page`.
- `--json --with-meta` returns `{"matches":[...],"meta":{...}}`. `--fields` projects
  each match, retaining completeness metadata. Plain `--json` remains an array.
  Metadata also includes `page`, `page_count`, `page_size`, `pages_fetched`, and
  `omitted_from_last_page`. `has_more` includes messages omitted when the limit
  cuts a page short; `has_next_page` means another API page is available within
  Slack's 100-page cap. Search totals reflect indexed matches and Slack search
  filters, not a complete channel history. Narrow the query to inspect results
  beyond the API cap.

### Readable JSON

JSON output preserves all response fields and numeric precision, while decoding
Slack's escaped Unicode and URLs into readable UTF-8. `--fields` also works in
JSON mode; dotted object paths select nested values and remain dotted output keys.
Without a field selection, Slack's `blocks`, attachments, and other metadata are
still included. Use plain transcripts or select fields for concise reading:

```bash
slk --config work thread C01234567 1700000000.000100
slk --config work history '#general' --since 3d --replies --no-links
slk --config work unread --since 3d
slk --config work get 'https://acme.slack.com/archives/C01234567/p1700000000000100' --context 3
slk --config work --json --fields ts,user,text,thread_ts thread C01234567 1700000000.000100
slk --config work search 'in:general on:2026-09-10' --sort-dir asc --limit 100
slk --config work --json --fields ts,user,text,permalink,thread_ts search 'in:general' --with-meta
slk --config work --fields messages.matches api search.messages -f 'query=in:general on:2026-09-10'
```

### Users
- `slk users` — `--filter`; lists the cache, so `--fields` takes only `id`,
  `name`, `real_name`. `slk user <@user|id>` (or `user view`) — title,
  status, presence, local time and time zone.

## Persistent flags

| Flag | Description |
|---|---|
| `--config <name>` | **Required.** Use `~/.slk/<name>/` profile (env `SLK_CONFIG`); no default |
| `--format table\|json\|csv\|tsv` | Output format (default rendered) |
| `--json` | Alias for `--format json` (all fields, readable UTF-8) |
| `--fields a,b,...` | Fields for all formats, including JSON; supports dotted object paths |
| `--verbose` | Dump API request/response to stderr |
| `--yes` | Confirm destructive operations (`delete`) |

## Notes

- Retries on HTTP 429 honoring the `Retry-After` header (exponential backoff).
- Slack returns HTTP 200 on logical errors; `slk` inspects `{"ok":false}` and
  exits non-zero with a mapped, hint-carrying message — failures are never
  silent successes.
- The directory cache (channels + users) is read by name-resolution and `users`/
  `channels` listing. `slk sync` is the heaviest call; it is cached on disk.
  Bots and deleted users are excluded; DM channels are named after the partner.
  A channel a transcript shows that the cache lacks is looked up and added.
- `search.messages` can lag the search index — a just-posted message may not be
  findable for a few seconds. Not a bug.
- `update`/`delete` only affect messages you authored.
- Long text (message bodies) can come from a file or stdin via `--body-file <path>`
  (use `-` for stdin), avoiding shell-quoting.
- `slk skill` is the single source of truth for command UX.

## Development

```bash
make test          # go test ./...
make vet           # go vet ./...
make fmt           # gofmt -w .
make build         # build ./slk, version stamped from git describe
make install       # symlink to ~/.local/bin
```

CI runs gofmt, `go vet` and `go test -race` on Linux and macOS for every push
and pull request. Tests never touch the network — they exercise pure helpers
(reference parsing, transcript rendering, search-query building, field
projection).

## Contributing

Issues and pull requests are welcome — see [CONTRIBUTING.md](CONTRIBUTING.md).

## License

[MIT](LICENSE) © Mykola Klitovchenko
