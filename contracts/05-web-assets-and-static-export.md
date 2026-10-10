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
notifications. The result level (project, run, or job) is selected once for
the whole search. Each condition selects an attribute from that level or one
of its ancestors: project results expose project attributes, run results
expose run and project attributes, and job results expose job, run, and
project attributes. Run attributes include the run's host and working
directory; a job with no job-specific working directory uses its run's
working directory for job searches. Ignore-case matching is on by default and
can be disabled. Job status uses the shared persisted execution-status
projection, so interrupted-run attempts can be searched by recorded states
such as `running (recorded)`, `waiting (recorded)`, `pending`,
`suspended (recorded)`, or
`unknown`, without requiring a live supervisor.
Fuzzy matching is an independent, opt-in setting for free-text attributes; it
allows a small edit distance and ignores terms shorter than four characters.
Status, executor, and diagnosis conditions use dropdowns and exact value
matching. Diagnosis choices come from the built-in diagnosis rules plus the
Python exception fallback, not from scanning saved results. Other attributes
including job and run host and working directory accept search text and match
substrings by default. Additional
conditions are joined in displayed order with AND or OR, evaluated left-to-right.
Results always remain at the selected level. Results are
ordered by activity time with a
stable tie-break and paged in batches of 50. Time ranges apply to the selected
result level: run start/finish for runs, job finish/submission (falling back to
the run time) for jobs, and activity in a run or job for projects. The static
export includes the search route but explains that history search requires the
live Web UI. Search and dropdown-option APIs validate basedir IDs against the
server's registered-basedir allowlist and only read persisted state. Selecting
a job result opens its run detail, selects the table page containing that job,
and scrolls the matching row into view.

Search projection and condition evaluation live in
[`internal/web/search.go`](../internal/web/search.go); the API scanner is in
[`internal/webui/history_search.go`](../internal/webui/history_search.go),
and the page is in
[`web_app_search.js`](../internal/webui/assets/web_app_search.js). Package and
API coverage is in
[`internal/web/search_test.go`](../internal/web/search_test.go) and
[`internal/webui/webui_test.go`](../internal/webui/webui_test.go); the
cross-project API behavior and recorded unfinished job status are covered by
[`TestHistorySearchAcrossProjects`](../conformance/05-web/history_search_test.go)
and [`TestHistorySearchFindsRecordedUnfinishedJobStatus`](../conformance/05-web/history_search_test.go).

## Asset layout

Web assets live under `internal/webui/assets/`:

```text
internal/webui/assets/
├── web_template.html
├── web_tokens.css
├── web_theme.js
├── web_styles.css
├── web_sidebar_styles.css
├── web_app_core.js
├── web_app_actions.js
├── web_app_logs.js
├── web_app_artifacts.js
├── web_app_tables.js
├── web_app_charts.js
├── web_app_matrix.js
├── web_app_notifications.js
├── web_app_search.js
├── web_app_bootstrap.js
├── web_static_bootstrap.js
├── jobs_template.html
├── web_info_styles.css
├── favicon-dark.svg
├── favicon-light.svg
└── fonts/
```

`assets.go` embeds these files. The JavaScript files are concatenated in this
order and delivered as one script; they intentionally share the global scope:

1. `web_app_core.js`
2. `web_app_actions.js`
3. `web_app_logs.js`
4. `web_app_artifacts.js`
5. `web_app_tables.js`
6. `web_app_charts.js`
7. `web_app_matrix.js`
8. `web_app_notifications.js`
9. `web_app_search.js`
10. `web_app_bootstrap.js`

Do not reorder these files without running the full Web test suite. The
separation is for source readability and ownership, not JavaScript module
isolation. `web_app_notifications.js` must load before `web_app_bootstrap.js`:
it wraps the global `refresh()` function, and bootstrap both calls `refresh()`
immediately and passes it to `setInterval`, so the wrap must already be in
place by then.

Colours and typefaces come from `web_tokens.css`: light values on `:root`,
dark values when the OS prefers dark and the viewer has not chosen light, or
when the viewer chose dark. Stylesheets and scripts set no colour literal and
name fonts only through `--font-sans` and `--font-mono`
(`TestStylesAndScriptsTakeColoursFromTokens`,
`TestStylesheetsTakeFontsFromTokens`). `web_theme.js` is inlined in `<head>`
of both page templates and applies the viewer's System / Light / Dark choice,
kept per browser, before the page is drawn (`TestWebThemeChoice`). The fonts
are IBM Plex Sans and Mono, IBM's Latin-1 WOFF2 files under the SIL OFL
([fonts/README.md](../internal/webui/assets/fonts/README.md));
[fonts.go](../internal/webui/fonts.go) serves them and their licence under
`/fonts/` with `/web_fonts.css` and writes them once at a static export's
root (`TestLiveServerServesEmbeddedFonts`, `TestStaticExportWritesFontsOnce`).

A status is shown as a pill whose tone (ok, bad, run, warn, off) names its
colour and icon. `statusTones` in
[internal/webui/status.go](../internal/webui/status.go) is the only
status-to-tone rule: Go renders the Job activity page with it and injects it
into the application script, which normalises labels the same way
(`TestStatusToneMatchesInGoAndJS`).

A table renderer emits its table whole: columns, labels, and a first
`Actions` cell built with `actionsCell` in `web_app_core.js`, whose buttons
carry their arguments in `data-` attributes. Tables rendered this way have the
`final` class; later render steps do not rename, move, or restyle their
columns.

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
  when it is seen for the first time already finished and its run ID sorts
  after every run the previous poll had in its project (covers runs shorter
  than the 2-second poll interval). An older run seen for the first time only
  appeared because the page loaded it, as opening a project or run does, and
  does not notify.
- A job is newly finished when `jobDisplayStatus(job, run)` (`web_app_tables.js`)
  becomes `"failed"` or `"success"`, the projection marks the job's result
  final, and the previous poll did not already show that final result. A job
  the previous poll lacked counts only in a run that poll showed running or
  that is new as above; opening a finished run, which loads its jobs, does not
  notify. Retried attempts are not reported because a job's result becomes
  final only after its retries end.
- Settings are looked up per basedir and project, for the page's basedir and
  each other polled basedir alike.
- Covered by `TestBrowserNotificationsReportOnlyWhatHappenedSinceThePreviousPoll`
  and `TestBrowserRunNotificationsUseTheExitCode`.
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
JavaScript once granted, so the global on/off toggle is a separate
`localStorage` flag (`rotari-notifications-enabled`) checked before showing
each notification; it does not touch the browser's actual permission grant.
`--notifications`/`ROTARI_WEB_NOTIFICATIONS` (`webui.Options.Notifications`,
injected into the app bundle and the Job activity page) only seeds the
toggle's starting value for an origin that has never set the `localStorage`
flag; an explicit prior toggle click always wins. The static Job activity page
shows this local toggle too, but does not poll for live job events.

## Web UI model and reports

The Web UI is a projection of the same persisted model, not a separate
database. `show --report`, the Web UI's `/api/report`, and static Web
generation all use the same Go formatter, `internal/report`, for run and job
reports. A run report is the run's record for people and agents: its sources,
its run notes, a table of its jobs, and each job's notes and evidence. The
report modal renders the Markdown, escaping all text and keeping only http(s)
links, and shows the source on request; Copy gives the Markdown. rotari does
not transmit or submit the report. Covered by
`TestRunReportRecordsSourcesNotesAndJobTable`,
`TestRenderMarkdownRendersReportsAndEscapesText`, and
`TestStaticWebReportRedactionToggle`.

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
before sharing them externally. Both `rotari web` and the static export expose
a `Redact: On` / `Redact: Off` button in the report modal. Turning it off
requests the unredacted report; the static export includes both report variants
in its generated files, so treat the export as containing sensitive data even
when the UI initially displays redacted reports. Redaction is on by default.

## Runtime and static mode

The ordinary Web UI and GitHub Pages static demo use the same JavaScript.
`pageParts()` abstracts route parsing:

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

Static pages receive `web_fonts.css`, `web_tokens.css`, `web_styles.css`, and
`web_sidebar_styles.css` beside every generated `index.html`; the fonts
themselves are written once, under `fonts/` at the export root. If a new asset
or static API endpoint is added, update both the normal Web handler and
`webui.GenerateStatic`/its bootstrap.

**WEB-9** A page of the static export shows what the live server's page
shows: the same tables, columns, row actions, and panels, with controls
answered by the read-only `403`. A page opened from disk has a path ending in
`index.html`; `routeParts()` drops that file name, so the page is routed like
its live path. Implemented by `routeParts` in
[web_static_bootstrap.js](../internal/webui/assets/web_static_bootstrap.js)
and the table renderers in
[web_app_core.js](../internal/webui/assets/web_app_core.js); covered for the
projects overview and a project page by
`TestStaticExportPagesShowWhatLivePagesShow` in
[conformance/05-web/static_pages_test.go](../conformance/05-web/static_pages_test.go).

## Web CLI modes

**WEB-3** `rotari web --static-dir DIR` writes a static export and exits; it
does not start the live HTTP server. Options that configure that server
(`--host`, `--port`, `--auth-token`) or its control endpoints
(`--allow-control`) are rejected when explicitly supplied by flag, environment,
or config, rather than accepted and silently ignored. The shared web command
boundary is in [cmd/rotari/web.go](../cmd/rotari/web.go); static-mode pair and
standalone checks are in
[conformance/03-interfaces/pairweb/web_static_pairs_test.go](../conformance/03-interfaces/pairweb/web_static_pairs_test.go).

**WEB-4** The static export's initial browser notification toggle, on both the
application and Job activity pages, follows `--notifications`. The Job
activity page does not poll for live events. Static export does not load saved
notification settings or provide live event polling: it has no server. The
markers are rendered by [internal/webui/assets.go](../internal/webui/assets.go)
and checked by `TestCLIFlagPairWebNotificationsEffect` in
[conformance/03-interfaces/pairweb/web_static_pairs_test.go](../conformance/03-interfaces/pairweb/web_static_pairs_test.go).

**WEB-5** A job row's Artifacts button shows the artifact candidates of the
attempt its Output button shows, laid out as `show -j JOB --artifacts`
prints them (CLI-16). `GET /api/artifacts?project_name=P&run_id=R&job_id=J`
(with an optional `attempt_id` of that job) returns the same
`jobstatus.ListArtifacts` JSON that `show -j J --json` carries, and rejects
an unknown job or another job's attempt. The static export embeds the listing
of every job's latest attempt and of each of its attempts, so the button
works there too; what each path holds is observed when the export is made.
The handler is in [internal/webui/artifacts.go](../internal/webui/artifacts.go)
and the view in `showArtifacts` in
[web_app_artifacts.js](../internal/webui/assets/web_app_artifacts.js); covered by
`TestWebShowsArtifactCandidates` in
[conformance/05-web/artifacts_test.go](../conformance/05-web/artifacts_test.go).

**WEB-6** The live server previews and downloads an artifact candidate's
content only when (a) the request names an entry of the attempt's recorded
listing by index, optionally with a clean relative child path inside a listed
directory, never a free path; (b) the path is under the job's recorded
working directory or an `rotari web --artifact-root DIR`; and (c) opening it
through `os.Root` does not leave that root, including through a symlink.
`/api/artifacts` adds `previewable` per entry; a refused request is 403, a
missing file 404. Images (png, jpg, jpeg, gif, svg), audio (wav, mp3, flac,
ogg, oga, opus, m4a), and video (mp4, webm, mov) are served inline, with
range requests so media can seek, and every other file, or any file with
`download=1`, as an attachment, all with `Cache-Control: no-store`,
`X-Content-Type-Options: nosniff`, and a `sandbox` Content-Security-Policy,
so an SVG's scripts never run. `/api/artifact-array` describes a `.npy`
file, or each of up to 200 arrays in a `.npz` file, from their headers:
dtype, shape, order, and the first 50 values in memory order for boolean,
integer, and floating dtypes; object arrays are pickles and their values are
never read. Files are
streamed without a size limit; text is served 64 KiB at a time on line
boundaries, from the start or, for `.log` and `.txt` in the UI, from the end,
and a file with a NUL byte is not text. A directory lists its immediate
children, directories first and then by name, 200 per page, reading at most
100,000 names. `--artifact-root` must name a directory and is rejected with
`--static-dir`; static exports contain listings but no file contents unless
WEB-7 applies. The
handlers are in [internal/webui/artifact_files.go](../internal/webui/artifact_files.go)
and the view in [web_app_artifacts.js](../internal/webui/assets/web_app_artifacts.js);
covered by `TestWebPreviewsArtifactsUnderAllowedRoots` in
[conformance/05-web/artifacts_test.go](../conformance/05-web/artifacts_test.go).
Opening an entry, including a directory child, scrolls to the rendered preview
below the listing; loading additional pages does not scroll it. This browser
behavior is checked by `TestWebArtifactPreviewInBrowser` in
[internal/webui/artifact_preview_test.go](../internal/webui/artifact_preview_test.go).

**WEB-7** `rotari web --static-dir DIR --static-artifact-contents` copies the
contents of the candidates WEB-6 would serve, with the job's working
directory as the only root, into `DIR/artifact-files/`: each file of at most
10 MiB, up to 100 MiB in all, one copy per file however many attempts list it.
It embeds the first answer of the other previews (a text file of at most
1 MiB as one page, an `.npy`/`.npz` description, a directory's first page),
marks those entries previewable, and prints how many files and bytes it
copied when it copied any. Opening a directory's children or later pages in
the export says they are not included. The flag requires `--static-dir`.
Without it, a static export has no file contents. Implemented in
[internal/webui/static_artifacts.go](../internal/webui/static_artifacts.go)
and `staticArtifactFileURL` in
[web_static_bootstrap.js](../internal/webui/assets/web_static_bootstrap.js);
covered by `TestStaticExportCopiesArtifactContents` in
[conformance/05-web/artifacts_test.go](../conformance/05-web/artifacts_test.go).

**WEB-8** A run report, from `show RUN_ID --report`, `/api/report`, or the
static export, reads each job's log, for its job table's last log line and
its log excerpt, from the attempt that produced the job's result: a job
carried into a retry is read from the run it was carried from, through
`jobstatus.ResultAttemptDir`, the lookup `FilterJob` uses. Implemented by
`readReportLog` in [internal/report/report.go](../internal/report/report.go);
covered by `TestRunReportReadsCarriedJobLogs` in
[conformance/05-web/report_test.go](../conformance/05-web/report_test.go) and
`TestRunReportReadsCarriedJobLogsFromTheirOrigin` in
[internal/report/record_test.go](../internal/report/record_test.go).

## Editing rules

- Edit HTML, CSS, and JavaScript in `internal/webui/assets/`, not in `webui.go`.
- Keep JavaScript additions in the responsibility file that owns the behavior.
- Prefer stable semantic classes and data attributes for UI behavior and tests;
  do not identify controls by their visible labels when a class or attribute can
  express the meaning.
- Tests should inspect final generated HTML for required hooks and obsolete
  vocabulary. Avoid assertions that depend on formatter whitespace or quote
  style.
- Scripts marked `// @ts-check` are checked against their JSDoc types by
  `npm run typecheck` ([internal/webui/tsconfig.json](../internal/webui/tsconfig.json),
  shared types in `assets/web_globals.d.ts`); CI and `scripts/check.sh` run it.
  Mark a script when rewriting it, and keep it passing.
- Run pre-commit on changed files, `go test ./...`, and `go build ./...` after
  Web asset changes.

## Generated Web pages

`cliDocsHTML` and `environmentHTML` are still generated by Go because they
iterate over Go metadata and escape dynamic values. The metadata comes from
the CLI through `webui.Options` (`Commands`, `Environments`). They are separate from the
main interactive Web assets and should not be folded into the JavaScript app.
