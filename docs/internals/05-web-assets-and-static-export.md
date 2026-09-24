# Web UI and static export

This document describes the Web UI, static export, and embedded asset
boundaries. Read it for Web/API/static-site changes in addition to the main
internal notes.

Representative implementation and tests:

- [cmd/rotari/web_assets.go](../../cmd/rotari/web_assets.go) for embedded asset
  declarations.
- [cmd/rotari/web.go](../../cmd/rotari/web.go) and
  [cmd/rotari/web_test.go](../../cmd/rotari/web_test.go) for Web handlers and
  static export.
- [cmd/rotari/assets/web_app_core.js](../../cmd/rotari/assets/web_app_core.js)
  and [cmd/rotari/assets/web_template.html](../../cmd/rotari/assets/web_template.html)
  for the browser application and page shell.

## Asset layout

Web assets live under `cmd/rotari/assets/`:

```text
cmd/rotari/assets/
├── web_template.html
├── web_styles.css
├── web_app_core.js
├── web_app_actions.js
├── web_app_logs.js
├── web_app_tables.js
├── web_app_charts.js
├── web_app_notifications.js
├── web_app_bootstrap.js
├── web_static_bootstrap.js
├── cli_docs_template.html
├── environment_template.html
├── web_info_styles.css
├── favicon-dark.svg
└── favicon-light.svg
```

`web.go` embeds these files. The JavaScript files are concatenated in this
order and delivered as one script; they intentionally share the global scope:

1. `web_app_core.js`
2. `web_app_actions.js`
3. `web_app_logs.js`
4. `web_app_tables.js`
5. `web_app_charts.js`
6. `web_app_notifications.js`
7. `web_app_bootstrap.js`

Do not reorder these files without running the full Web test suite. The
separation is for source readability and ownership, not JavaScript module
isolation. `web_app_notifications.js` must load before `web_app_bootstrap.js`:
it wraps the global `refresh()` function, and bootstrap both calls `refresh()`
immediately and passes it to `setInterval`, so the wrap must already be in
place by then.

The static export bootstrap is kept separately in `web_static_bootstrap.js`
because it provides the static fetch and routing adapters used only by
generated pages. It is formatted as ordinary JavaScript, then receives the
generated state, logs, and reports during export.

## Web server and control-plane security

- `web` binds `--host`/`--port`, defaulting to `127.0.0.1:8787`.
- A busy default port scans upward for a free port. An explicit port,
  including `--port 0`, never falls back and fails immediately if unavailable.
- The actually bound address is reported after listener creation. Non-loopback
  hosts produce a warning when no Web UI token is configured.
- `--auth-token` or `ROTARI_WEB_AUTH_TOKEN` wraps every Web route and accepts
  `Authorization: Bearer TOKEN`, `X-Rotari-Token: TOKEN`, or Basic
  authentication with username `rotari` and the token as the password; this is
  authentication only and does not encrypt HTTP traffic.
- `loadWebState` exposes persisted runtime metadata: `running.lock` fields and
  the presence of `server.sock`/`server.pid`. The panel does not query process
  liveness or infer that `state.lock` is held from the file's existence.
- The Unix-socket control surface is separate from `ROTARI_PRIVATE_STATE`:
  reaching it means controlling the server, not merely reading state.
- `runServer` always sets the socket to `0600`. On Linux,
  `verifyPeerCredential` rejects connections whose UID differs from the server
  process; on other platforms the socket mode is the enforcement.
- Web mutating routes are gated by `allowControl`, enabled by default and
  configurable with `--allow-control` or `ROTARI_WEB_ALLOW_CONTROL`.
  `--allow-control=false` rejects them with `403` before reading request bodies.
  Read-only `GET` routes remain available.
- Web state exposes only whether an environment variable is set. Raw values
  never cross the HTTP boundary; `Value` is populated only by the local
  `rotari env` CLI command.

## Desktop notifications

`cmd/rotari/assets/web_app_notifications.js` shows a browser `Notification`
when a run finishes or a job fails. It is entirely client-side: no server
route, socket, or webhook exists for this. `runServer` (the job-execution
daemon) and `rotari web` are separate processes that never talk to each other
directly; `rotari web` only re-reads persisted state from disk per request
(see "Web UI model and reports" below), so there is no event to push from the
runner side even if one were added. Instead this feature wraps the existing
`refresh()` polling loop (`web_app_core.js`) and diffs the previous and next
`/api/state` snapshots on every tick:

- A run is newly finished when it stops being `running` between two polls, or
  when it is seen for the first time already finished (covers runs shorter
  than the 2-second poll interval).
- A job is newly failed when `jobDisplayStatus(job, run)` (`web_app_tables.js`)
  becomes `"failed"` and was not already `"failed"` on the previous poll.
- Job failures and a run's own completion detected in the same poll tick are
  merged into one `Notification` per run; events from different ticks stay
  separate.
- The very first poll after page load never notifies (there is no previous
  snapshot to diff against), so existing history never triggers a notification
  burst on open.

The permission itself (`Notification.permission`) cannot be revoked from
JavaScript once granted, so the toolbar's on/off toggle is a separate
`localStorage` flag (`rotari-notifications-enabled`) checked before showing
each notification; it does not touch the browser's actual permission grant.
`--notifications`/`ROTARI_WEB_NOTIFICATIONS` (`webNotificationsDefault` in
`web.go`, injected into the bundle as `__ROTARI_NOTIFICATION_DEFAULT__`) only
seeds the toggle's starting value for an origin that has never set the
`localStorage` flag; an explicit prior toggle click always wins.

## Web UI model and reports

The Web UI is a projection of the same persisted model, not a separate
database. `show --report`, the Web UI's `/api/report`, and static Web
generation all use the same Go formatter for AI reports. Opening an AI service
copies the report and opens a new tab; rotari does not transmit or submit the
report.

Run job projections include each persisted attempt. The jobs table defaults to
the latest attempt, but stores a browser-local selection per job so rows can
independently display a prior attempt's result, timestamps, and log. The log
endpoint validates that an optional attempt ID belongs to its requested run and
job before reading that attempt directory.

Report generation redacts known hostnames and paths, then applies heuristic
redaction to common absolute paths and FQDNs in log text. This is best-effort
privacy protection, not complete secret detection; users must review reports
before sharing them externally.

## Runtime and static mode

The ordinary Web UI and GitHub Pages static demo use the same JavaScript.
`pageParts()` abstracts route parsing:

- normal Web mode reads `location.pathname`;
- static mode uses the injected `routeParts()` helper.

`staticPath()` prefixes links with the repository base path, and
`rewriteStaticLinks()` repairs dynamically created absolute links. Keep links in
the HTML template relative where possible.

Static export injects a bootstrap before the app script. The bootstrap provides
persisted state, logs, and reports through `fetch` and defines `routeParts()`.
For a selected-job report, it returns only the selected jobs' pre-generated
reports, rather than the whole run report. It must run before the app script:
otherwise the first state request can hit GitHub Pages' 404 document and
briefly render that HTML as application text.

Static pages receive a copy of `web_styles.css` beside every generated
`index.html`. If a new asset or static API endpoint is added, update both the
normal Web handler and `generateStaticWeb`/its bootstrap.

## Editing rules

- Edit HTML, CSS, and JavaScript in `cmd/rotari/assets/`, not in `web.go`.
- Keep JavaScript additions in the responsibility file that owns the behavior.
- Prefer stable semantic classes and data attributes for UI behavior and tests;
  do not identify controls by their visible labels when a class or attribute can
  express the meaning.
- Tests should inspect final generated HTML for required hooks and obsolete
  vocabulary. Avoid assertions that depend on formatter whitespace or quote
  style.
- Run `npm run format:check`, `go test ./...`, and `go build ./...` after Web
  asset changes.

## Generated Web pages

`cliDocsHTML` and `environmentHTML` are still generated by Go because they
iterate over Go metadata and escape dynamic values. They are separate from the
main interactive Web assets and should not be folded into the JavaScript app.
