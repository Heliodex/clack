# /slackauthticket

Researched 2026-11-14, after Karim asked for `/slackauthticket <ticket>`
sign-in. The ask was framed as Slack desktop's sign-in, typed into the
Ctrl+K jump box. It isn't that. `/slackauthticket` is how the Slack CLI
signs in: `slack login` mints a ticket and asks you to send the command
from a message composer in Slack, and Slack's server answers with its own
modals. No Slack client has code for it, and Slack's jump box doesn't run
commands at all.

## Evidence

Static reading only. No request carried a credential to Slack, and no
real ticket was used.

- Slack desktop 4.53.287 for Linux, the newest release on 2026-11-14,
  downloaded without a cookie from downloads.slack-edge.com:
  `slack-desktop-4.53.287-amd64.deb` sha256 `c2057f221e41275b7eadc5b658edc00289466b6fb55c912c5894831f6900f969`,
  `app.asar` `3b44a00bbae78d12b7fde810a4d831ff92ed05d1af0569210e5f9f6cc201ca5c`,
  `dist/main.bundle.cjs` `22b23b6ae295ca986a098ae63edda035206f5430f63d485d4b1fa2778489ecbc`,
  `dist/preload.bundle.js` `8edef56cc3e9eb146bd15a3857a21616b394bb5bb9bcfe7086cf28ec27cc38e2`.
  A case-insensitive search of every file in app.asar for `authticket`,
  `auth_ticket` and `slackauth` finds nothing. Neither do the 4.53.276
  macOS and Windows main bundles from earlier work.
- Slack's web client, build `5b18c9c90a59`, captured by slack-datamine-plus
  on 2026-11-14. All 3,856 chunks in its `build/chunks.txt` were streamed
  from the public CDN without a cookie and searched for `slackauthticket`,
  `authticket`, `auth_ticket`, `exchangeAuthTicket` and
  `generateAuthTicket`. Nothing matched. The same pipeline finds
  `/slackdevtools` in client-boot and slash-commands-local, so the search
  itself works. The datamine's `ui-strings.txt`, `dev-commands.txt` and
  `slash-commands.txt` don't mention it either.
- The Slack CLI's source, github.com/slackapi/slack-cli at main
  `0b9f72394c4c`: `internal/pkg/auth/login.go`, last changed in
  `922813ee99b2`, and `internal/api/auth.go`, last changed in
  `890a67ea3f23`. Its guide `docs/guides/authorizing-the-slack-cli.md`
  is also at docs.slack.dev/tools/slack-cli/guides/authorizing-the-slack-cli.

## The flow

1. `slack login` POSTs `apps.hosted.generateAuthTicket` to slack.com with
   `slack_cli_version`, the CLI's major.minor version such as `v3.9`.
   `slack auth token`, which asks for a service token that doesn't
   rotate, adds `no_rotation=true`. The call carries no token. The answer
   is `{ok, ticket}`, and login.go calls the ticket a "base64 encoded
   UUID", which is what Karim's example decoded to.
2. The CLI prints `/slackauthticket <ticket>` and says to run it "in any
   Slack channel or DM". Its comments warn that indented or bold pasted
   text keeps Slack from seeing a command.
3. The user sends that line from a message composer in the workspace.
   Slack's server opens a modal asking to grant the Slack CLI some
   permissions. After Confirm, a second modal shows a short challenge
   code. The CLI's agent skill gives `JDt1IK7X` as an example.
4. The user types the code into the CLI, which POSTs
   `apps.hosted.exchangeAuthTicket` with `ticket`, `challenge` and
   `slack_cli_version`, this time the full version. The answer is
   `{ok, is_ready, token, refresh_token, exp, team_id, team_domain,
   team_name, user_id, enterprise_id, is_enterprise_install}`, and `token`
   is an `xoxp-` user token. An answer without one fails with "No token
   returned from the following Slack API method". With `--no-prompt
   --ticket … --challenge …`, `is_ready: false` also means not signed in.
   The skill doc says tickets are single-use and time-limited.
5. The CLI saves the token to `~/.slack/credentials.json` and renews it
   with `tooling.tokens.rotate`, which takes `refresh_token` and answers
   `{token, refresh_token, team_id, user_id, iat, exp}`. Service tokens
   never expire and are printed instead of saved.

So the thing that signs in is the CLI. The Slack client carries one
command to the server and shows the server's modals.

## What a Slack client does with it

- The composer. `executeCommand`, boot-render module `5dbN` in build
  5b18c9c90a59, finds no local handler for the command and sends it
  through `chatCommandApi`: `chat.command` with `command`, `disp`,
  `text`, `blocks`, `channel` and `client_token`, plus `thread_ts`,
  `team_id` and `app_id` when they apply. The ticket travels in `text`.
  Its log lines name only the command, `Command "/slackauthticket" is
  PENDING` and then RESOLVED or FAILED with the error code. The modals
  arrive on the socket as `view_opened` for the command's
  `client_token`. The thread check `isSlashCommandEnabled` only runs for
  a command in the workspace's command list, so an unlisted one reaches
  the server from a thread too.
- Draft sync. `syncDraftCreate`, boot-shared `oJnH`, makes no exception
  for commands. Pausing before Enter saves the line to the user's drafts
  on Slack's server, and sending deletes it.
- The jump box. No omniswitcher module runs a slash command. The one link
  goes the other way: `/search` opens the switcher, the
  `openFromSlashCommand` thunk in boot-shared `pfjr`. A
  `/slackauthticket <ticket>` typed into the jump box is a search query.
- Slack desktop's main process has nothing for it. Desktop sign-in is the
  `slack://…/magic-login/…` link the browser hands over, which
  docs/DESKTOP-SIGNIN.md covers and Clack already takes.

## What Clack does

The same. Clack's composer sends a `/word` it doesn't run itself to
`chat.command` with a `client_token`: `serverCommand` in
internal/core/command.go is a port of `executeCommand`. Blockkit shows
the modals that come back as `view_opened`, and the realtime socket that
carries them is always on. Modal text is a richview, which selects with
the mouse and copies with Ctrl/⌘+C, so the challenge code can be copied
out. Ctrl+K searches, as Slack's does.

So `slack login` already works with Clack the way the CLI's guide says:
paste the `/slackauthticket …` line into any conversation's composer,
press Enter, Confirm in Slack's modal, and copy the code into the
terminal. Ctrl+K is the wrong box in Slack too.

This branch adds two guards:

- `slack.Redact`, which every log line, error string and crash record
  passes through, masks `/slackauthticket <ticket>`, also with the
  full-width ／ that Slack's composer accepts, and `ticket=` and
  `challenge=` values. No Clack log line carried them before. The rule is
  a backstop for future ones. Test: `TestRedactAuthTicket`.
- `TestSlackAuthTicketCommand` in internal/ui/composer. In a channel and
  in a thread, the line goes out as one `chat.command` with the ticket
  only in `text`. Nothing is posted, no other request carries the
  ticket, a modal for the command's token opens, and no log line holds
  the ticket.

## Why Clack doesn't sign itself in with a ticket

The brief asked for a ticket exchange that writes Clack's creds.json. For
that, Clack would have to call `apps.hosted.generateAuthTicket` and
`apps.hosted.exchangeAuthTicket` itself and pose as the Slack CLI, the
same kind of disguise SECURITY.md C-18 rules out. The user would also be
approving the CLI's permissions, not Clack's.

It wouldn't run Clack either. The exchange returns an `xoxp-` user token
for the CLI's app. Clack runs on a session, an `xoxc-` token with the `d`
cookie, for `client.*`, edgeapi and the realtime socket. That mismatch
hasn't been tried live, and the scopes Slack gives the CLI's token aren't
in the CLI's source.

## Not verified live

A real ticket is single-use, and none was available.

- The modals' blocks. Clack draws whatever Block Kit Slack sends, so a
  block type Clack lacks would show up here first.
- Whether `/slackauthticket` is in `commands.list`. If it is listed
  without `is_runnable_from_thread`, Clack refuses it in a thread before
  calling the API, as Slack does.
- What an expired or used ticket looks like. It could be a `chat.command`
  error, which Clack shows as Slackbot's "*/slackauthticket* failed with
  the error …", or a message inside Slack's modal.
- Copying the challenge code out of the real modal.
