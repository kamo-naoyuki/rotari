# Web UI and static export

This document describes the Web UI, static export, and embedded asset
boundaries. Read it for Web/API/static-site changes in addition to
[00-overview.md](00-overview.md).

Representative implementation and tests:

- [internal/webui/assets.go](../internal/webui/assets.go) for embedded asset
  declarations.
- [internal/webui/webui.go](../internal/webui/webui.go) and
  [internal/webui/webui_test.go](../internal/webui/webui_test.go) for Web
  handlers and static export, and
  [cmd/rotari/web.go](../cmd/rotari/web.go) for the `web` command, which
  supplies the CLI metadata, environment definitions, and config template
  through `webui.Options`.
- [internal/webui/assets/web_app_core.js](../internal/webui/assets/web_app_core.js)
  and [internal/webui/assets/web_template.html](../internal/webui/assets/web_template.html)
  for the browser application and page shell.

## Job timeline

**WEB-1** A run timeline counts carried results at its initial point and
includes timestamped submission and completion events for jobs executed in
the run, even when those jobs have origin metadata. The initial point uses
the actual run start; an initial load sample corrects older summaries that
recorded a late start. The chart identifies the initial point and
disambiguates repeated local clock labels with dates. The shared
carried-result decision is in
[internal/runlineage](../internal/runlineage/runlineage.go); timeline
projection is in [internal/web](../internal/web/timeline.go) and display is in
[web_app_charts.js](../internal/webui/assets/web_app_charts.js), covered by
[`TestFilteredRerunCarriesCompletedResults`](../conformance/02-lifecycle/lifecycle_test.go) and
[`TestRunTimelineStartsAtActualRunStart`](../conformance/02-lifecycle/lifecycle_test.go).

## History search

**WEB-2** The live Web UI provides a dedicated history search across projects
in user-selected scopes. Each scope selects a registered basedir and may
narrow to one project and one run; multiple scopes are combined as a union.
The dropdown choices load hierarchically from the selected basedir and project.
Search scope is independent of basedir checkboxes used to monitor browser
notifications. Search conditions select a project, run, or job field and a
case-insensitive substring; additional conditions are joined in displayed
order with AND or OR, evaluated left-to-right. When conditions target multiple
levels, results use the most specific level and conditions for parent levels
match the corresponding ancestor. Results are ordered by activity time with a
stable tie-break and paged in batches of 50. Time ranges apply to the selected
result level: run start/finish for runs, job finish/submission (falling back to
the run time) for jobs, and activity in a run or job for projects. The static
export includes the search route but explains that history search requires the
live Web UI. Search and dropdown-option APIs validate basedir IDs against the
server's registered-basedir allowlist and only read persisted state.

Search projection and condition evaluation live in
[`internal/web/search.go`](../internal/web/search.go); the API scanner is in
[`internal/webui/history_search.go`](../internal/webui/history_search.go),
and the page is in
[`web_app_search.js`](../internal/webui/assets/web_app_search.js). Package and
API coverage is in
[`internal/web/search_test.go`](../internal/web/search_test.go) and
[`internal/webui/webui_test.go`](../internal/webui/webui_test.go); the
cross-project API behavior is covered by
[`TestHistorySearchAcrossProjects`](../conformance/05-web/history_search_test.go).

## Asset layout

Web assets live under `internal/webui/assets/`:

```text
internal/webui/assets/
├── web_template.html
├── web_styles.css
├── web_sidebar_styles.css
├── web_app_core.js
├── web_app_actions.js
├── web_app_logs.js
├── web_app_tables.js
├── web_app_charts.js
├── web_app_matrix.js
├── web_app_notifications.js
├── web_app_search.js
├── web_app_bootstrap.js
├── web_static_bootstrap.js
├── cli_docs_template.html
├── environment_template.html
├── jobs_template.html
├── web_info_styles.css
├── favicon-dark.svg
└── favicon-light.svg
```

`assets.go` embeds these files. The JavaScript files are concatenated in this
order and delivered as one script; they intentionally share the global scope:

1. `web_app_core.js`
2. `web_app_actions.js`
3. `web_app_logs.js`
4. `web_app_tables.js`
5. `web_app_charts.js`
6. `web_app_matrix.js`
7. `web_app_notifications.js`
8. `web_app_search.js`
9. `web_app_bootstrap.js`

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

`/jobs/` is server-rendered from the same `joblist.Collect` path
([internal/joblist](../internal/joblist/joblist.go)) as `rotari jobs`:
it includes running jobs and jobs completed within the preceding 24 hours by
default. The dynamic page accepts `?since=DURATION`, validated by the shared
`joblist.ParseSince` helper. Its static equivalent is `jobs/index.html` and remains
fixed at the default window; preserve that page whenever changing the Web export
layout.

The All projects and Job activity pages load the same sidebar stylesheet,
`web_sidebar_styles.css`; static Web exports include it beside each generated
application page. Keep shared sidebar layout changes in that asset rather than
duplicating them in the page-specific stylesheets. `TestWebSidebarStylesAreSharedWithJobsPage`
and `TestGenerateStaticWebWritesProjectPages` in
[internal/webui/webui_test.go](../internal/webui/webui_test.go) cover the shared
asset and static copies.

The live Web UI sidebar is hierarchical: registered basedirs (plus the
startup `--basedir`) contain projects, and projects contain runs. Other than
History search, users choose a project from the selected basedir's tree.
`/api/state` is a lightweight index: it lists project names,
run counts, the latest and immediately preceding run summaries, and running-lock
metadata without loading project queues or completed run details. Keeping the
preceding summary lets notification polling catch a fast run followed by a new
run before the next tick. It includes full details only for
currently running runs across the selected basedir, so completion and
job-failure notifications work without scanning completed runs' jobs. Opening
a project loads its queue and run summaries through `/api/project`; opening a
completed run loads its jobs, attempts, context, and timeline through `/api/run`.
`/api/active-runs` exposes the same index for notification monitoring of
other basedirs. Completed run details are cached in the browser for that
session and are not polled again. These API routes are bound to the basedir in
the URL mount. The cross-basedir `/api/history-search` route is the exception:
it accepts only basedir IDs from the registered allowlist. The mount identifier resolves
only to the startup basedir or a basedir in the read-only registry list;
unlisted IDs are not accepted. The static export remains a complete
single-basedir snapshot and has no basedir switching. Covered by
[`TestWebStateLoadsProjectAndRunDetailsOnDemand`](../internal/webui/webui_test.go),
[`TestWebSwitchesBetweenRegisteredBasedirs`](../internal/webui/webui_test.go),
and [`TestWebSidebarLazilyListsProjectsInOtherBasedirs`](../internal/webui/webui_test.go).

The basedir list is introduced by a `Registered basedirs` heading so the tree
is self-describing. Each basedir entry is collapsible and only the basedir
currently mounted starts expanded; collapsing or expanding another entry is a
user choice the sidebar keeps. An entry labels itself with the basedir's
absolute path, shortened at the end with the full path in its tooltip, because
basedir names alone are ambiguous. `All projects` and `Job activity` are nested
inside each basedir entry rather than offered once at the top level, since both
pages describe a single basedir. `All projects` has its own disclosure control;
its project list is nested beneath it, and `Job activity` follows the project
tree. The sidebar scrolls independently of the page body, preserves its
manually selected position during background refreshes, and remembers the
position per basedir. A separator at the sidebar/content boundary is shown on
hover and can be dragged to resize the sidebar; basedir paths keep their
end with an ellipsis through CSS as the width changes. These
rules apply to both the application page and the server-rendered Job activity
page.

Each basedir row has a notification monitor checkbox. The selected basedir IDs
are stored in browser local storage under a key scoped to the current Web
server session. Restarting `rotari web` therefore discards the extra selected
basedirs and returns monitoring to the current basedir. The application page
polls `/api/state` for its mounted basedir and `/api/active-runs` for each
other selected basedir; the server-rendered Job activity page polls the Job
activity endpoint. A newly selected basedir is
initialized from its current state, so existing completed jobs do not produce
retroactive notifications. Notifications remain controlled by the global
notification permission and on/off toggle.

## Web server and control-plane security

- `web` binds `--host`/`--port`, defaulting to `127.0.0.1:8787`.
- A busy default port scans upward for a free port. An explicit port,
  including `--port 0`, never falls back and fails immediately if unavailable.
- The actually bound address is reported after listener creation. Non-loopback
  hosts produce a warning when no Web UI token is configured.
- `--auth-token` or `ROTARI_WEB_AUTH_TOKEN` wraps every Web route and accepts
  `Authorization: Bearer TOKEN`, `X-Rotari-Token: TOKEN`, or Basic
  authentication with username `rotari` and the token as the password; this is
  authentication only and does not encrypt HTTP traffic. The notification
  settings form masks a saved webhook URL in its input and never returns that
  URL in the read API response, but masking is only a visual safeguard: a URL
  entered in the form is sent to the Web server over the current HTTP
  connection when saved. Use loopback or a trusted network, or terminate HTTPS
  at a trusted reverse proxy; do not treat the password-style field or auth
  token as transport encryption.
- A non-loopback listener also exposes registered basedir paths; the warning
  without an auth token includes that path information.
- `loadWebState` ([internal/webui/webui.go](../internal/webui/webui.go)) exposes persisted runtime metadata for each project: `running.lock` fields and,
  in the project's `server`, whether its supervisor's `server.pid` exists and the PID it records. The panel does not query process
  liveness or infer that `state.lock` is held from the file's existence.
- A supervisor has no control surface of its own: only the `run` command that
  started it can send it requests, over inherited pipes, so reaching it does
  not depend on `ROTARI_PRIVATE_STATE` or on socket permissions.
- Web mutating routes are gated by `allowControl`, enabled by default and
  configurable with `--allow-control` or `ROTARI_WEB_ALLOW_CONTROL`.
  `--allow-control=false` rejects them with `403` before reading request bodies.
  Read-only `GET` routes and the read-only `POST /api/history-search` remain
  available.
- Web state exposes only whether an environment variable is set. Raw values
  never cross the HTTP boundary; `Value` is populated only by the local
  `rotari env` CLI command.

## Config viewing and generation

The Web config view, generation, and save contracts are described with the
rest of config handling in
[01-resolution-and-config.md](01-resolution-and-config.md#configuration-files).

## Desktop notifications

`internal/webui/assets/web_app_notifications.js` shows a browser `Notification`
when a run finishes or a job fails. It is entirely client-side: no server
route, socket, or webhook exists for this. `runServer` (the job-execution
daemon) and `rotari web` are separate processes that never talk to each other
directly; `rotari web` only re-reads persisted state from disk per request
(see "Web UI model and reports" below), so there is no event to push from the
runner side even if one were added. Instead this feature wraps the existing
`refresh()` polling loop (`web_app_core.js`) and diffs the previous and next
`/api/state` index on every tick; that index includes lightweight summaries
for each project's latest run and full details only for currently running
runs. When other basedirs are selected for notifications, their `/api/active-runs`
projections are polled as well:

- A run is newly finished when it stops being `running` between two polls, or
  when it is seen for the first time already finished (covers runs shorter
  than the 2-second poll interval).
- A job is newly finished when `jobDisplayStatus(job, run)` (`web_app_tables.js`)
  becomes `"failed"` or `"success"` in an active-run detail, the projection marks
  the job's result final, and the previous poll did not already show that final
  result. Retried attempts are not reported because a job's result becomes final
  only after its retries end.
- Job results and a run's own completion detected in the same poll tick are
  merged into one `Notification` per run; events from different ticks stay
  separate.
- The very first poll after page load never notifies (there is no previous
  snapshot to diff against), so existing history never triggers a notification
  burst on open.

Which events are shown comes from `notifications.toml`, whose `[browser]`
section uses the same keys and field vocabulary as `[webhook]` but holds its
own values. `rotari web` reads the settings for its own basedir at startup and
serves per-project settings from `/api/notification-settings`, so a saved or
reloaded file applies to open pages on their next poll without restarting the
process. `browser.job_failure`, `browser.job_success`, `browser.run_failure`,
and `browser.run_success` select the events, `browser.max_jobs` bounds the
jobs listed per notification, and the body is truncated at 1000 characters.
The static export has no server to read, save, or reload those files, so it
offers neither the notification editor nor live notifications.

The permission itself (`Notification.permission`) cannot be revoked from
JavaScript once granted, so the toolbar's on/off toggle is a separate
`localStorage` flag (`rotari-notifications-enabled`) checked before showing
each notification; it does not touch the browser's actual permission grant.
`--notifications`/`ROTARI_WEB_NOTIFICATIONS` (`webui.Options.Notifications`,
injected into the bundle as `__ROTARI_NOTIFICATION_DEFAULT__`) only
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

The run page keeps Cancel and Suspend/Resume out of individual job rows.
The toolbar actions between Report and Delete run operate on the checked jobs:
Cancel includes selected pending, running, and suspended jobs; Suspend targets
selected running jobs; Resume targets selected suspended jobs. Cancel and
Suspend stay disabled unless at least one checked job is running, while Resume
is enabled when a checked job is suspended. The Web API accepts one `job_id`
for existing callers or a `job_ids` list for these bulk actions. The browser
selection and action state are implemented in
[`web_app_actions.js`](../internal/webui/assets/web_app_actions.js) and
[`web_app_logs.js`](../internal/webui/assets/web_app_logs.js); the API validates
IDs in [`webui.go`](../internal/webui/webui.go). Covered by
[`TestRunBulkControlsOperateOnSelectedJobs`](../internal/webui/webui_test.go)
and [`TestWebJobControlRejectsStaleRunID`](../internal/webui/webui_test.go).

A run page adds one collapsible section per matrix group, collapsed by default,
between the run graphics and the job table controls
([web_app_matrix.js](../internal/webui/assets/web_app_matrix.js)).
`addMatrixPanels` runs at the end of each render, like the other run
sections, and its header shows the group's success and failure counts.
`LoadJobs` attaches each member's group ID, base name, dimensions, and values
from the run's command snapshot; it leaves out the base environment, which may
hold secrets. Rows and columns default to the first two dimensions and can be
switched with the section's selectors (choosing the other axis's dimension
swaps them); remaining dimensions split the group into one grid per
combination. Cells are keyed by values in dimension order, so any axis choice
finds the same jobs. Expansion and axis choices are kept per group while the
page is open. A cell takes the worst state of its jobs (array tasks share a
cell and show `succeeded/total`), classified from the same resolved `result`
as the jobs table. Clicking a cell opens a box, attached to `document.body` so
the periodic re-render does not remove it, that lists each of the cell's jobs
with a job-name copy button and a copy of its table row's action buttons plus
`Show in table`. A copy presses the button at the same position in the current
row, so every action, including ones added by later render steps, behaves
exactly as in the table. `restoreMatrixActions` moves the box to the
re-rendered cell, rebuilds it when the cell's jobs changed, or closes it when
the cell is gone or collapsed. Jobs whose matrix provenance was cleared by a
partial copy or change appear only in the table. Covered by `TestWebRunViewDrawsMatrixGrid` in
[cmd/rotari/web_test.go](../cmd/rotari/web_test.go).

Long values in the Command, Working directory, Dependencies, and Executor
options columns of the job and queue tables start clamped to three lines
(`clampLongTableCells` in
[web_app_tables.js](../internal/webui/assets/web_app_tables.js)). Clicking the
text, which highlights on hover, or pressing Enter or Space on it expands or
collapses it; a click that ends a text selection does not, so values can still
be selected. A cell is
considered once its text exceeds 60 characters, and it is left unclamped when
the browser measures that the text already fits. Buttons such as copy icons
stay outside the clamped text, cells with editors are left alone, and expanded
cells stay open across re-renders. Covered by `TestWebRunViewClampsLongCells`.

Report generation redacts known hostnames and paths, then applies heuristic
redaction to common absolute paths and FQDNs in log text. This is best-effort
privacy protection, not complete secret detection; users must review reports
before sharing them externally. `rotari web` (not the static export) exposes
a `Redact: On` / `Redact: Off` button in the report modal; turning it off
requests `/api/report` with `redact=false` and re-fetches the unredacted
report. Redaction stays on by default and the static export always serves
its precomputed, redacted report regardless of the button, so the button is
hidden there.

## Runtime and static mode

The ordinary Web UI and GitHub Pages static demo use the same JavaScript.
`pageParts()` abstracts route parsing:

- normal Web mode reads `location.pathname`;
- static mode uses the injected `routeParts()` helper.

`staticPath()` prefixes links with the repository base path, and
`rewriteStaticLinks()` repairs dynamically created absolute links. Keep links in
the HTML template relative where possible.

Static export injects a bootstrap before the app script. The bootstrap provides
persisted state, logs, reports, configuration files, config-generation targets,
and defines `routeParts()`. For a selected-job report, it returns only the
selected jobs' pre-generated reports, rather than the whole run report. The
static app uses the same UI functions and request flow as the ordinary Web UI
whenever possible, including configuration viewing and editing. Requests that
would mutate state or files return the same read-only `403` message as a server
started with `--allow-control=false`. The bootstrap must run before the app
script: otherwise the first state request can hit GitHub Pages' 404 document
and briefly render that HTML as application text.

Static pages receive a copy of `web_styles.css` beside every generated
`index.html`. If a new asset or static API endpoint is added, update both the
normal Web handler and `webui.GenerateStatic`/its bootstrap.

## Editing rules

- Edit HTML, CSS, and JavaScript in `internal/webui/assets/`, not in `webui.go`.
- Keep JavaScript additions in the responsibility file that owns the behavior.
- Prefer stable semantic classes and data attributes for UI behavior and tests;
  do not identify controls by their visible labels when a class or attribute can
  express the meaning.
- Tests should inspect final generated HTML for required hooks and obsolete
  vocabulary. Avoid assertions that depend on formatter whitespace or quote
  style.
- Run pre-commit on changed files, `go test ./...`, and `go build ./...` after
  Web asset changes.

## Generated Web pages

`cliDocsHTML` and `environmentHTML` are still generated by Go because they
iterate over Go metadata and escape dynamic values. The metadata comes from
the CLI through `webui.Options` (`Commands`, `Environments`). They are separate from the
main interactive Web assets and should not be folded into the JavaScript app.
