# slk — Slack CLI (agent reference)

Agent-facing Slack from Bash. Read channels/threads, catch up, post/reply, search.

## Setup
Create a Slack app, add user-token scopes, install to the workspace, copy the
**User OAuth Token** (`xoxp-...`). Scopes:
`channels:read groups:read im:read mpim:read channels:history groups:history
im:history mpim:history chat:write search:read users:read files:read files:write`.
Optional, per feature: `reactions:write` (react), `im:write` (DM someone you
have no DM with yet), `pins:read` + `bookmarks:read` (channel info).

`slk --config <ws> config init --token xoxp-...` (or pipe the token on stdin).

## Global
- `--config <ws>` REQUIRED (env `SLK_CONFIG`); no default profile.
- `--json` / `--format json|csv|tsv` ; `--fields a,b,c` (fixes table/CSV columns
  and their order) ; `--verbose` ; `--yes`.
- `--dry-run` on any write command: prints the resolved plan, sends nothing.
  Use it before posting as the user when channel/thread resolution matters.
- Transcripts: `--no-links` drops permalink lines (ts= stays), `--max-chars N`
  caps each message. Both cut tokens a lot on busy channels.
- JSON uses readable UTF-8; all fields are preserved unless `--fields` selects
  a subset. Dotted object paths are supported (output keys retain the dots).
- Prefer plain transcripts for reading. Line format:
  `[YYYY-MM-DD HH:MM:SS +zz:zz] ts=<ts> @author (edited): text`, then indented
  `file …`, `▸ attachment/unfurl`, `reactions: :+1:×3`, `↳ N replies`, link.
  Markup is decoded (`@name`, `#channel`, `label (url)`, `&` not `&amp;`).
  JSON keeps raw Slack fields and bulky blocks.
- Lean JSON: `slk --config work --json --fields ts,user,text,thread_ts thread C01234567 1700000000.000100`.

## Reference forms
`#general` or `general`, `@mako` or `mako`, raw IDs `C…`/`U…`, a Slack permalink,
or a raw `ts` (`1700000000.000100`). Where a channel is expected, `@user` (or a
`U…` ID) means the DM with that user. Times (`--since/--until/--at`): `2h`, `3d`,
`1w`, `today`, `yesterday`, `YYYY-MM-DD[ HH:MM]` (local), RFC3339, epoch, ts;
`--at` also takes `+2h`, `in 30m`, `16:00`, `tomorrow 09:00`. The directory cache resolves names; on a
miss it refetches once, and a transcript's channel missing from it is looked up
(`conversations.info`) and cached. `slk sync` refreshes it manually.

## Commands
- `slk me` — who am I.
- `slk channels [--types public,private,im,mpim] [--filter x] [--limit N]` (DMs list as `@partner`)
- `slk activity [--since YYYY-MM-DD] [--limit 200]` — channels sorted by latest
  indexed message, with date, type, preview, link, and sampled message count.
  Limit counts messages, not channels; this is a search sample, not a census.
  Default searches all dates; requires `search:read`. JSON includes sample metadata.
- `slk channel view <#chan|id>` ; `slk channel info <#chan|id>` — topic,
  purpose, members, bookmarks, pinned messages (missing scopes are reported).

### Catching up (start here)
- `slk unread [#chan...] [--since 7d]` — messages past your read marker plus
  followed threads with unread replies (only the new replies). No channels →
  active ones discovered via search. Your own messages are skipped. Read-only.
- `slk mentions [--since 7d] [--limit 50]` — messages that @-mention you.
- `slk digest [#chan...] [--since 1d] [--limit 100] [--replies=false]` — one
  transcript across channels, threads inline. New replies under parents older
  than --since are not shown (use `unread`).
- `slk tail <#chan...> [--since 1d] [--peek]` — only what is new since the last
  tail of each channel (cursors in ~/.slk/<ws>/cursors.json). For loops. The
  cursor follows top-level messages: new replies in older threads need `unread`.

### Reading
- `slk history <#chan|@user|id|permalink> [--limit N] [--since 3d] [--until ...] [--desc]`
  — latest N messages in the window, printed oldest first (`--desc` flips).
  - `--replies [--replies-limit 50]` — expand every thread inline under its
    parent (`    ↳ ` lines), the latest N replies of each; one API call per
    thread. JSON nests `replies`.
  - `--thread <ts|permalink>` — read one thread instead (= `slk thread`);
    `slk history <thread-permalink>` implies it.
- `slk thread <#chan|id> <ts|permalink> [--since ...] [--until ...] [--limit 200]` — a
  whole thread: the parent plus its latest N replies.
- `slk get <permalink | #chan ts> [--context N]` — one message (marked `»`) with N
  messages around it; a top-level message shows its replies, a reply shows its
  thread. Best first step when all you have is a link.
- Never call `conversations.replies`/`history` via `slk api` — use these.
- A reply permalink (`?thread_ts=`) resolves to its parent for thread targets.
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
### Writing (as the user — consider `--dry-run` first)
- Text is standard Markdown (`**bold**`, `[label](url)`, lists, code) sent as
  `markdown_text` (12k chars max). `--mrkdwn` sends Slack's own syntax instead.
- `slk send <#chan|@user|id> <text> [--thread ts|permalink] [--at time] [--body-file -] [--id-only]`
- `slk reply <#chan|id> <ts|permalink> <text> [--at time]` — thread reply.
- `slk dm <@user> <text> [--at time]` — opens the DM if needed (`im:write`).
- `--at` schedules (chat.scheduleMessage); `slk scheduled [#chan]` lists,
  `slk scheduled delete <#chan> <id> --yes` cancels. Slack returns no text for
  scheduled Markdown messages.
- `slk update <#chan|id> <ts> <text>` — your own message only.
- `slk delete <#chan|id> <ts> --yes` — your own message only.
- `slk react <permalink | #chan ts> <:emoji:> [--remove]` ; `slk unreact …` —
  needs `reactions:write`; targets the exact message (a reply, not its parent).
- `slk search [query] [--in #chan] [--from @user|me] [--to @user|me] [--since 3d]
  [--until …] [--on day] [--has link|pin|reaction|:emoji:] [--thread-only] [--limit N]
  [--sort-dir asc|desc]` — filters need no query; native operators (`in:`,
  `from:`, `after:`) also work inside it. `--since` includes its own day. `--limit` walks
  pages (up to 100/page, `--limit` 1–10000).
  Stderr always reports `total`, `returned`, `has_more`, `has_next_page`.
  `--json --with-meta` returns `{matches,meta}`; `--fields` selects fields within
  each match and leaves metadata intact. Plain `--json` remains an array.
  Example: `slk --config work --json --fields ts,user,text,permalink,thread_ts search 'in:general' --with-meta`.
  `has_more` includes matches omitted from a partially returned last page;
  `has_next_page` means another API page is available within Slack's 100-page cap.
  Metadata includes page/page_count/page_size/pages_fetched/omitted_from_last_page.
- `slk users [--filter x]` (cache: id, name, real_name only) ; `slk user <@user|id>`
  (= `user view`) — title, status, presence, local time and tz: check before
  pinging someone.
- `slk api <method.name> -f key=value` — raw Web API escape hatch.
  Example: `slk --config work --fields messages.matches api search.messages -f 'query=in:general on:2026-09-10'`.

## Gotchas
- `search.messages` lags the index: a just-posted message may not be findable
  for a few seconds. Not a bug.
- `update`/`delete` only affect messages you authored (else `cant_*_message`).
- `sync` pulls the full directory — the heaviest call; it is cached on disk.
  Bots and deleted users are excluded from the cache.
- Output shows `ts=` in every message header so you can reply/react/get by it.
- `history` without `--since` reads the latest messages; old channels can be
  months stale — check the dates.
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
