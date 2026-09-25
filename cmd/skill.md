# slk — Slack CLI (agent reference)

Agent-facing Slack from Bash. Read channels/threads, post/reply, search.

## Setup
Create a Slack app, add user-token scopes, install to the workspace, copy the
**User OAuth Token** (`xoxp-...`). Scopes:
`channels:read groups:read im:read mpim:read channels:history groups:history
im:history mpim:history chat:write search:read users:read files:read files:write`.

`slk --config <ws> config init --token xoxp-...` (or pipe the token on stdin).

## Global
- `--config <ws>` REQUIRED (env `SLK_CONFIG`); no default profile.
- `--json` / `--format json|csv|tsv` ; `--fields a,b,c` ; `--verbose` ; `--yes`.
- JSON uses readable UTF-8; all fields are preserved unless `--fields` selects
  a subset. Dotted object paths are supported (output keys retain the dots).
- Prefer plain `history`/`thread` for reading: dated transcripts with profile,
  channel, author, reply counts, and message links. JSON includes bulky blocks.
- Lean JSON: `slk --config work --json --fields ts,user,text,thread_ts thread C01234567 1700000000.000100`.

## Reference forms
`#general` or `general`, `@mako` or `mako`, raw IDs `C…`/`U…`, a Slack permalink,
or a raw `ts` (`1700000000.000100`). The directory cache resolves names; on a
miss it refetches once. `slk sync` refreshes it manually.

## Commands
- `slk me` — who am I.
- `slk channels [--types public,private,im,mpim] [--filter x] [--limit N]` (DMs list as `@partner`)
- `slk activity [--since YYYY-MM-DD] [--limit 200]` — channels sorted by latest
  indexed message, with date, type, preview, link, and sampled message count.
  Limit counts messages, not channels; this is a search sample, not a census.
  Default searches all dates; requires `search:read`. JSON includes sample metadata.
- `slk channel view <#chan|id>`
- `slk history <#chan|id> [--limit N] [--since 2024-01-01] [--until ...] [--oldest ts] [--latest ts]`
- `slk thread <#chan|id> <ts|permalink>` — thread replies (comments).
- `slk files download <file-id|file-permalink> [-o path-or-directory]` — download
  a Slack-hosted attachment with the profile token; requires `files:read`.
  Default filename is `<file-id>-<name>` in the current directory. Existing
  files are never overwritten; failed downloads leave no final file.
  Transcripts display file IDs; for message links, inspect the thread first.
- `slk files upload <path>... [-c #chan] [--thread ts] [--comment text | --comment-file -] [--title t] [--name n]`
  — upload local files; requires `files:write`. With `-c` all files are shared
  in one message (optionally in a thread, with a comment); without it they stay
  private. `--title`/`--name` only for a single file. Paths are validated before
  any upload. Output: `id`, `name`, `bytes` per file.
- `slk send <#chan|id> <text> [--thread ts] [--body-file -] [--id-only]`
- `slk reply <#chan|id> <ts> <text>` — sugar for a thread reply.
- `slk update <#chan|id> <ts> <text>` — your own message only.
- `slk delete <#chan|id> <ts> --yes` — your own message only.
- `slk search <query> [--in #chan] [--from @user] [--limit N] [--sort-dir asc|desc]` — native Slack
  operators (`in:`, `from:`, `after:`) also work inside <query>. `--limit` walks
  pages (up to 100/page, `--limit` 1–10000).
  Stderr always reports `total`, `returned`, `has_more`, `has_next_page`.
  `--json --with-meta` returns `{matches,meta}`; `--fields` selects fields within
  each match and leaves metadata intact. Plain `--json` remains an array.
  Example: `slk --config work --json --fields ts,user,text,permalink,thread_ts search 'in:general' --with-meta`.
  `has_more` includes matches omitted from a partially returned last page;
  `has_next_page` means another API page is available within Slack's 100-page cap.
  Metadata includes page/page_count/page_size/pages_fetched/omitted_from_last_page.
- `slk users [--filter x]` ; `slk user view <@user|id>`
- `slk api <method.name> -f key=value` — raw Web API escape hatch.
  Example: `slk --config work --fields messages.matches api search.messages -f 'query=in:general on:2026-09-10'`.

## Gotchas
- `search.messages` lags the index: a just-posted message may not be findable
  for a few seconds. Not a bug.
- `update`/`delete` only affect messages you authored (else `cant_*_message`).
- `sync` pulls the full directory — the heaviest call; it is cached on disk.
  Bots and deleted users are excluded from the cache.
- Output shows `ts=` on every line so you can reply/thread by it.
- Transcript/search dates include the local UTC offset. Rendered transcripts
  make one extra `auth.test` request for workspace message links; JSON/CSV/TSV do not.
- Channels include an explicit `type` column (public/private/im/mpim).
- On `missing_scope`, read `needed`/`provided`: add the required User Token Scopes
  at https://api.slack.com/apps → OAuth & Permissions, then Reinstall to Workspace
  and save the resulting User OAuth Token with `slk --config <ws> config init` via stdin.
  Update/unset `SLK_TOKEN` if set; it overrides saved config. Download HTTP 403 can
  also mean wrong workspace, inaccessible file, Slack Connect, or workspace policy.
- File downloads accept HTTPS private URLs from files.slack.com/files.slack-edge.com
  returned by files.info, strip OAuth credentials on cross-origin redirects, and
  reject HTML sign-in responses for non-HTML files. External provider files need
  that provider's authorized integration; adding scopes does not override file access.
