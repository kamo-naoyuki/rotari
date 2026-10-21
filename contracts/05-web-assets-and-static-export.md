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
recorded a late start. The run page draws the timeline as one lane per job
(`lanes`, built by `BuildJobLanes` from the same inputs and finish rule):
an executed job spans its submission to its finish, a carried job and a job
not yet started are noted instead, and the axis starts at the initial point,
labelled with its clock time (and date when the run crosses a day), with the
rest marked as offsets from it. Its height stays within 260px whatever the job
count or parallelism: past 16 lanes' worth of jobs the lanes lose their names
and thin down to 2px, then share pixel rows, drawn failed over running over
success, and hovering a row lists its jobs. The shared
carried-result decision is in
[internal/runlineage](../internal/runlineage/runlineage.go); timeline
projection is in [internal/web](../internal/web/timeline.go) and display is in
[web_app_charts.js](../internal/webui/assets/web_app_charts.js) (`TestBuildJobLanes`,
`TestJobTimelineDrawsOneLanePerJob`, `TestJobTimelineCompressesManyJobs`), covered by
[`TestFilteredRerunCarriesCompletedResults`](../conformance/02-lifecycle/lifecycle_test.go) and
[`TestRunTimelineStartsAtActualRunStart`](../conformance/02-lifecycle/lifecycle_test.go).

## Activity

**WEB-10** `GET /api/activity?from=TIME&to=TIME` (RFC 3339) lists, across every
registered basedir, the runs whose span meets the range: from a run's start to
its finish, or to now while it runs, and only its start when it recorded no
finish (`Run.Span` and `Overlapping` in
[internal/runlist](../internal/runlist/runlist.go)). It lists the same runs,
lifecycles, and recorded times as `rotari runs` (`runlist.Collect`), with each
run's working directory, host, source revisions, and own notes
(`runlist.ReadDetails`). Every registered basedir is listed: one that no
longer exists is marked `missing`, and one whose runs cannot be read carries
its error while the others are still listed. A missing, malformed, reversed,
or longer than 45-day range is rejected with 400 rather than clamped. The
endpoint only reads, and only the runs that may meet the range: it skips,
without opening them, runs whose ID says they started at or after the range
and runs whose directory last changed before it, except a project's running
or interrupted run (`runlist.CollectBetween`). It also gives the latest run
start in any basedir (`latest_started_at`, from run IDs alone), so the page
opens on the latest day with runs in one more request. Implemented by `handleActivity` in
[activity.go](../internal/webui/activity.go); covered by
`TestActivityListsRunsAcrossBasedirs` and `TestActivityRejectsBadRanges` in
[activity_test.go](../internal/webui/activity_test.go), and
`TestActivityListsRunsAcrossRegisteredBasedirs` in
[conformance/05-web/activity_test.go](../conformance/05-web/activity_test.go).

The Activity page (`/activity/`, [web_app_activity.js](../internal/webui/assets/web_app_activity.js))
draws that response as a month calendar and one day's timeline. It sorts runs
into the browser's days by each run's `ends_at` (the server's span end), and
opens on today, or on the latest day with runs when today has none, saying so.
A day's lanes are one per (basedir, project), grouped by basedir and
collapsible, newest first, with lanes past twelve folded behind a "more"
button; bars are coloured by result and link to the run page. The eight
projects with the most running time in the shown month get the categorical
colours `--c1`–`--c8`, always beside their names; the rest share grey. A
missing or unreadable basedir is listed above the calendar. A static export
embeds every run of its basedir for the page and says the page shows that
basedir only. Covered by `TestActivityPage` in
[activity_page_test.go](../internal/webui/activity_page_test.go) and
`TestStaticExportIncludesActivity`.

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
├── web_shell.js
├── web_styles.css
├── web_shell.css
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
`TestStylesheetsTakeFontsFromTokens`). `web_shell.js` is inlined in `<head>`
of both page templates. It applies the viewer's System / Light / Dark choice,
kept per browser, before the page is drawn (`TestWebThemeChoice`), and runs the
page frame: the Settings menu, the navigation drawer, and the sidebar's width
(`TestWebShellMenuAndDrawer`). The fonts
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

A table renderer emits its table whole: columns, labels, and an `Actions`
cell built with `actionsCell` in `web_app_core.js`, whose buttons carry their
arguments in `data-` attributes. The cell is last in a row, except on the
project's queue table, where it comes first beside the row's editors. No later render step renames,
moves, or restyles a table's columns, and the run page's sections are drawn
whole by `renderRunGraphics` in `web_app_charts.js`. Of those sections the
failure causes section and the job timeline start open and the others closed;
one the viewer opens or closes stays so in that browser (`runSectionOpen`).
The failure causes section is drawn only for a run with failed or blocked
jobs and shows the run's `lineage_summary.failures` groups (CLI-4) as they
come: it does not regroup jobs. A cause's Select button checks exactly that
cause's jobs, and only the project's latest run, once finished, shows each
cause's retry preview, selected as the CLI's `retry:` hint selects it
(`TestWebRunPageShowsFailureCauses`). Layout lives in the
stylesheets; only data-driven sizes and colours are set inline.

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

The All projects and Job activity pages load the same page-frame stylesheet,
`web_shell.css` (sidebar, top bar, and status pills); static Web exports
include it beside each generated application page. Keep shared frame changes
in that asset rather than duplicating them in the page-specific stylesheets. `TestWebSidebarStylesAreSharedWithJobsPage`
and `TestGenerateStaticWebWritesProjectPages` in
[internal/webui/webui_test.go](../internal/webui/webui_test.go) cover the shared
asset and static copies.
The shared page frame uses the full viewport width without a maximum-width
cap, including in static exports; content padding and the mobile navigation
drawer remain in place (`TestWebShellUsesFullWidth` in
[internal/webui/shell_test.go](../internal/webui/shell_test.go)).

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

Both page kinds share one frame. A top bar holds breadcrumbs (the basedir,
then the project and run), the notification toggle, Refresh, and a Settings
menu with the theme choice and, on application pages, the config and
notification config actions; the page title and its facts follow. The sidebar
starts with Views (History search), then the basedir list. Below 760px wide the
sidebar is a drawer opened from the top bar, and no page scrolls sideways.

The basedir list is introduced by a `Registered basedirs` heading so the tree
is self-describing. Each basedir entry is collapsible and only the basedir
currently mounted starts expanded; collapsing or expanding another entry is a
user choice the sidebar keeps. An entry, the breadcrumb naming a page's
basedir, and the history search's basedir choice label the basedir by its parent directory in bold and its own name,
with the full path in its tooltip, because basedirs usually share a name such
as `.rotari-state`; `newWebBaseDir` in [assets.go](../internal/webui/assets.go)
computes the label once for both page kinds
(`TestNewWebBaseDirLabelsByParentAndName`). `All projects` and `Job activity` are nested
inside each basedir entry rather than offered once at the top level, since both
pages describe a single basedir. `All projects` has its own disclosure control;
its project list is nested beneath it, and `Job activity` follows the project
tree. The sidebar scrolls independently of the page body, preserves its
manually selected position during background refreshes, and remembers the
position per basedir. A separator at the sidebar/content boundary is shown on
hover and can be dragged to resize the sidebar; a label too long for the
width ends with an ellipsis. These rules apply to both the application page
and the server-rendered Job activity page.

Each basedir row has a notification monitor checkbox, drawn as a bell. The selected basedir IDs
are stored in browser local storage under a key scoped to the current Web
server session. Restarting `rotari web` therefore discards the extra selected
basedirs and returns monitoring to the current basedir. The application page
polls `/api/state` for its mounted basedir and `/api/active-runs` for each
other selected basedir; the server-rendered Job activity page polls the Job
activity endpoint. A newly selected basedir is
initialized from its current state, so existing completed jobs do not produce
retroactive notifications. Notifications remain controlled by the global
notification permission and on/off toggle.

**WEB-12** `rotari web` reads the registered basedir list at startup and
rereads the basedir registry on every page load and on `GET /api/basedirs`,
which the Refresh button calls. A basedir registered or unregistered after the
server started therefore appears or disappears without a restart. A mount
whose basedir ID is not listed rereads the registry once before answering 404,
so a page opened directly at a newly registered basedir is served. When the
reread fails, `/api/basedirs` answers 500 and the previous list stays. The
ways to update the application page are ordered; each reads everything the
one before it reads:

1. The 2-second poll reads `/api/state`, the selected project when its
   revision changed, and the run details it needs. It does not reread the
   basedir list.
2. Refresh also reloads the selected project whatever its revision, rereads
   the basedir list, and drops the other basedirs' cached project lists, so an
   expanded basedir lists its projects again.
3. Reloading the page also discards every browser-side cache and the
   sidebar's expansion state.
4. Restarting `rotari web` also rereads its options and configuration and
   starts a new notification session.

The Job activity page's Refresh reloads the page. Covered by
[`TestWebRereadsRegisteredBasedirsWithoutRestart`](../internal/webui/basedir_reload_test.go),
[`TestWebRefreshButtonRereadsBasedirs`](../internal/webui/basedir_reload_test.go),
and [`TestWebListsBasedirsRegisteredAfterStart`](../conformance/05-web/basedir_reload_test.go).

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

When the page cannot use notifications, because it is not a secure context
(plain HTTP from a non-loopback host) or the site's permission is denied, the
toggle on both the application and Job activity pages stays visible but
disabled, and its title gives the reason from `notificationUnavailableReason`
in `web_shell.js`. Covered by
`TestNotificationToggleExplainsWhyItIsUnavailable`.

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

The run page's jobs table (`runJobsHTML` in `web_app_core.js`) shows each
job's command under its name, as the Job activity page does. Both show an
attempt ID by its end (`.id-tail` in `web_shell.css`, written by
`idTailHTML` in `web_app_core.js` and `assets.go`), while its text, title,
and copy button keep the whole ID. Hosts, Executor options, and Working
directory have a copy button when not empty. Every column stays in the table; a column whose
values are the same for every job, and Job ID and Finished, start hidden, and the Columns
menu above the table turns any column but Job, Status, and Exit on or off,
remembered per browser. A row ends with all its actions as buttons, without a menu, in lines of
four: Log, Artifacts (on a job that has started, for the attempt Log shows;
disabled before), Launch where the
server allows launching (likewise) (it opens Artifacts with the commands for the
working directory listed, `launchOnWorkingDirectory` in
[web_app_launches.js](../internal/webui/assets/web_app_launches.js), or says
why the working directory cannot be launched on), Diagnosis on a failed job,
Notes (disabled when the job has none), Report, and Path. Covered by
`TestWebRunJobsTable` in
[run_jobs_test.go](../internal/webui/run_jobs_test.go) and
`TestWebLaunchWorkingDirectoryWithoutCandidatesInBrowser` in
[launch_browser_test.go](../internal/webui/launch_browser_test.go). The project's queue
table follows the same column rule, implemented once in `columnTable`: Job
name / ID and Command always shown, the source times hidden, and the rest
shown when they differ; a hidden column's editors stay in the row, so Save
still sends their values. Covered by `TestWebQueueTable` in
[queue_table_test.go](../internal/webui/queue_table_test.go).

**WEB-11** Each row of the All projects page ends with Path and Delete.
Delete asks for confirmation and then deletes the whole project through
`POST /api/delete-project` with `{"project_name": NAME}`, which calls the
same `Editor.DeleteProject` as `delete --project` (SAFE-12) and returns its
message and any warnings. The button is disabled for a project the page
shows as running; the server rejects a running or interrupted project and
an invalid name, and a read-only server or static export answers `403`.
`deleteProjectButton` in
[web_app_core.js](../internal/webui/assets/web_app_core.js) and
`deleteProject` in
[web_app_actions.js](../internal/webui/assets/web_app_actions.js); covered by
`TestWebAllProjectsDeleteProject` in
[delete_project_test.go](../internal/webui/delete_project_test.go),
`TestWebDeleteProjectEndpoint` in
[webui_test.go](../internal/webui/webui_test.go), and
`TestDeleteProjectRemovesOnlyThatProject` in
[conformance/04-coordination/private_state_test.go](../conformance/04-coordination/private_state_test.go).

Run job projections include each persisted attempt. The jobs table defaults to
the latest attempt, but stores a browser-local selection per job so rows can
independently display a prior attempt's result, timestamps, and log. The log
endpoint validates that an optional attempt ID belongs to its requested run and
job before reading that attempt directory.

The run page keeps Cancel and Suspend/Resume out of individual job rows.
The selection buttons and a count of checked jobs sit beside the Jobs heading,
and the toolbar under it (`runSelectionHTML` and `runActionsHTML` in
`web_app_core.js`) is rendered with the table.
The toolbar actions after Report and before the run's own actions (a button
per command for the directory holding the basedir where launching is
available, Cancel run while the run is running, and Delete run) operate on
the checked jobs:
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
options columns of the job and queue tables start clamped to three lines, and
the command under a run page's job name to two
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
`web_shell.css` beside every generated `index.html`; the fonts
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
(`--host`, `--port`, `--auth-token`, `--artifact-root`), its control endpoints
(`--allow-control`), or launching (`--allow-exec`, `--allowed-host`) are
rejected when explicitly supplied by flag, environment, or config, rather than
accepted and silently ignored. The shared web command
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

**WEB-5** A job row's Artifacts action shows the artifact candidates of the
attempt its Log button shows, laid out as `show -j JOB --artifacts`
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
listing by index, or its recorded absolute working directory as entry `wd`,
optionally with a clean relative child path inside a listed
directory, never a free path; (b) the path is under an
`rotari web --artifact-root DIR`, or, when no other user could have chosen
the listing's paths, under the job's recorded working directory or the
directory holding its basedir (never `/`); and (c) opening it through
`os.Root` does not leave that root, including through a symlink. No other
user could have chosen the paths when the basedir, its `projects`
directory, the project's directory, `queue.json`, `meta.json`, `runs`, and the
run's directory are each owned by the Web server's user or root and not
writable by group or others (permission bits only; an ACL is not seen):
whoever can write those can make a job record any working directory and
path. Otherwise `/api/artifacts` adds `roots_note`, which says why only
`--artifact-root` is served, and the Web UI shows it.
`/api/artifacts` adds `previewable` per entry (an empty list for a listing
without candidates; left out only by a static export without copied
contents), and `working_directory_previewable` for the working directory,
which the view lists first as `.`, also when there are no candidates (type
from `working_directory_type`, which the CLI's
listing JSON carries too) and which is not a candidate: it is served under
the same roots, so not for an untrusted record without `--artifact-root`;
a refused request is 403, a
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
covered by `TestWebPreviewsArtifactsUnderAllowedRoots` and
`TestWebServesArtifactsOnlyFromTrustedRecords` in
[conformance/05-web/artifacts_test.go](../conformance/05-web/artifacts_test.go).
The Table viewer (WEB-19) also serves, inline with the same headers, the
image, audio, and video files a served table's cells name, only under the
same roots; the ownership rule is `artifactRecordsTrust` in
[internal/webui/artifact_trust.go](../internal/webui/artifact_trust.go).
Opening an entry, including a directory child, scrolls to the rendered preview
below the listing; loading additional pages does not scroll it. This browser
behavior is checked by `TestWebArtifactPreviewInBrowser` in
[internal/webui/artifact_preview_test.go](../internal/webui/artifact_preview_test.go).

**WEB-7** `rotari web --static-dir DIR --static-artifact-contents` copies the
contents of the candidates WEB-6 would serve without `--artifact-root`
(which static export rejects), into `DIR/artifact-files/`: each file of at most
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

## Launching commands on artifacts

`rotari web --allow-exec` runs the commands a user registered in
`launchers.toml` against job artifacts. The user guide is
[Launching commands on artifacts](../docs/INSPECT.md#launching-commands-on-artifacts),
the file format [Launcher commands](../docs/CONFIGURATION.md#launcher-commands),
and the design [the artifact launcher plan](../development/2026-10-20-artifact-launcher/plan.md).
The manager is [internal/launcher](../internal/launcher/), the Web side
[internal/webui/launches.go](../internal/webui/launches.go),
[launch_guard.go](../internal/webui/launch_guard.go), and
[launch_proxy.go](../internal/webui/launch_proxy.go), and the CLI side
[cmd/rotari/web_launch.go](../cmd/rotari/web_launch.go).

**WEB-13** Launching is off unless `rotari web` gets `--allow-exec` on the
command line; no config file or environment variable enables it. It is
rejected with `--allow-control=false`, `--allowed-host` is rejected without
it, and both are rejected with `--static-dir` (WEB-3). With it, every route
requires the Web token, on loopback listeners too: without
`--auth-token` or `ROTARI_WEB_AUTH_TOKEN`, the server generates one and
prints it on stderr. Without it, `GET /api/launchers` reports launching
unavailable with the server's reason and enabling hint, and the launch API
answers 403 with the same text. Covered by `TestWebLaunchRequiresOptInAndToken`
in [conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go).

**WEB-14** While launching is enabled, a request whose `Host` is not
`localhost`, `127.0.0.1`, `[::1]` (any port), a specific `--host`, or an
`--allowed-host` name gets 421, and a request other than `GET` or `HEAD`, or a
WebSocket handshake, gets 403 unless its `Origin` equals its own scheme and host, or it has no `Origin`
and authenticates with `X-Rotari-Token` or `Authorization: Bearer`. Basic
authentication alone, which a browser resends by itself, does not exempt a
request without `Origin`. Covered by `TestWebLaunchGuardsRequests` in
[conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go).

**WEB-15** Only the built-in commands (TensorBoard, Jupyter Lab, MLflow, MLflow
(SQLite), Aim, and Streamlit) and the commands in the global config
directory's `launchers.toml` (`$XDG_CONFIG_HOME/rotari/launchers.toml`) can
run; an entry there with a built-in's name replaces it, and a copy of the file in a basedir, project,
or workspace is never read. `GET /api/launchers` marks a command
`unavailable`, with the reason, when the first program it runs is not on the
Web server's `PATH` (or an absolute path to it is not executable), and such a
command neither previews nor starts. `GET /api/launchers?kind=K&name=N` lists
only the commands that accept a target of kind K named N, including a
command's file name `patterns`, and a start refuses a target its patterns do
not match. `rotari web --allow-exec` refuses to
start when that file, a directory above it, or a symlink on the way is owned
by another user or writable by group or others, or when the file is invalid;
`rotari config --launchers` writes a template there with mode `0600` and
`config --list` lists it. `POST /api/launch` accepts only a registered
command's name, an artifact named as WEB-6 names one (entry index and
optional child under an allowed root) or, with a project and an existing
run but no job, attempt, or child, entry `basedir_parent` for the directory
holding the basedir under the same roots as that run's entries (the run
page's toolbar has a button per command that accepts it, disabled with the
reason when its program is missing, `basedirParentLaunchButtons` in
[web_app_launches.js](../internal/webui/assets/web_app_launches.js)), and
whether to start another
instance; any other field is rejected and nothing starts, as are an unknown
name, a command that does not accept the artifact's kind, and an artifact
that does not exist on the Web host. The command runs with `/bin/sh -c`,
empty stdin, `ROTARI_LAUNCHER_ARTIFACT` and `ROTARI_LAUNCHER_CWD` set, and
without `ROTARI_WEB_AUTH_TOKEN`. Covered by
`TestWebLaunchRunsOnlyRegisteredCommands` in
[conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go),
and `basedir_parent` by `TestLaunchOnBasedirParent` in
[launches_test.go](../internal/webui/launches_test.go), its toolbar buttons
by `TestWebRunJobsTable` in
[run_jobs_test.go](../internal/webui/run_jobs_test.go) and
`TestWebLaunchOnBasedirParentInBrowser` in
[launch_browser_test.go](../internal/webui/launch_browser_test.go).

**WEB-16** A start while the same command runs on the same target and
working directory returns that launch, marked reused, unless another instance
is requested. `GET /api/launches` lists launches with their state and exit
status, `GET /api/launch-log` pages their output, and `POST /api/launch-stop`
ends a launch's whole process group. Stopping or killing `rotari web`, even
with SIGKILL, ends every launch's process group. Covered by
`TestWebLaunchLifecycle` in
[conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go).

**WEB-17** A service launch (`mode = "service"`) is `starting` until the
process listening on `127.0.0.1:$ROTARI_LAUNCHER_PORT` belongs to the launch
and its readiness path under `$ROTARI_LAUNCHER_BASE_PATH` answers 2xx or 3xx;
a process outside the launch on that port fails it. Each service launch gets
its own `$ROTARI_LAUNCHER_TOKEN`, which never appears in an API response.
Only after readiness does `/_launch/ID/...` forward requests to it, with path
and query unchanged. Open links in both Artifacts and Launches keep this
server-wide path, including after refresh on a `/_basedir/ID/` page.
Requests are forwarded without the browser's `Authorization` and
`X-Rotari-Token` headers or Rotari launch-session cookie, and with the
command's `proxy_headers` (the readiness probe sends them too), in which
`$ROTARI_LAUNCHER_TOKEN` is the
launch's token, so a service can refuse anyone who reaches its port
without the proxy; before then and
after it ends the route answers 503, and 404 for an unknown ID.
An authenticated request under `/_launch/` establishes an HttpOnly,
SameSite=Strict cookie scoped to that prefix (Secure over TLS), valid for 12
hours and invalidated by a Web server restart. It authenticates only launch
routes, including requests carrying the application's own Authorization
header; all Host and Origin checks still apply. Other application cookies
are forwarded unchanged. Implemented by
[launch_auth.go](../internal/webui/launch_auth.go) and
[launch_proxy.go](../internal/webui/launch_proxy.go), with session lifetime
and guard checks in
[launch_auth_test.go](../internal/webui/launch_auth_test.go).
Covered by
`TestWebLaunchServesServices` in
[conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go).

**WEB-18** The live server shows the global launcher file whether or not
launching is enabled: `GET /api/launcher-config` returns its path, whether it
exists, its text, why `rotari web --allow-exec` would refuse it, and the
commands a restarted server would load (built-ins included, each with
`builtin` and `unavailable`). `POST /api/generate-launcher-config` writes the
same template as `rotari config --launchers` there, mode `0600`, answers 409
without changing an existing file, and 403 on a read-only server. The Web UI
writes no other launcher file content. The Settings menu's Launcher config
and Generate launcher config use them
([internal/webui/launcher_config.go](../internal/webui/launcher_config.go)).
Covered by `TestWebShowsAndGeneratesLauncherConfig` in
[conformance/05-web/launch_test.go](../conformance/05-web/launch_test.go).

**WEB-19** The live server's Table viewer reads an artifact named as WEB-6
names one, under the same roots and with the same refusals (403 outside
them or through an escaping symlink, 404 when missing), at
`/tableview/?<the content request's query>`. Launch with offers it, as a
link that opens in a new tab, for every directory and for files whose names
match `*.csv`, `*.tsv`, `*.tab`, `*.psv`, `*.txt`, `*.scp`, `*.lst`,
`*.list`, or `*.dat` (and `*.CSV`, `*.TSV`), on every live server: it runs no command, so it needs no
`--allow-exec` and is listed in `/api/launchers` as `viewers` whether or not
launching is available. `/tableview/api/rows` splits the file into records
on line breaks and cells on `sep`, which must be exactly one character other
than a double quote or a line break, honoring double quotes as CSV does and
skipping blank lines; `header=0` keeps the first record as a row (default
`1`, its column names). It returns `limit` rows (default 100, at most 1000)
from `offset`, numbered from 1, with the total, cells cut to 4 KiB, and an
error for a record over 16 MiB; the file is indexed once per version and
separator, so a page does not reread it from the start. `filters`, a JSON
list of `{column, op, value}` with columns counted from 0, keeps the rows
every condition matches: `=` and `!=` compare numbers when the cell and the
value both read as finite numbers and text otherwise, `<`, `<=`, `>`, `>=`
need a number and match only cells that read as one, `contains` and
`not_contains` compare text case-sensitively, and `empty` and `not_empty`
test for a blank cell (a short row's missing cells are blank); a column out
of range, an unknown operator, or an ordering against a non-number is
refused. `sort` (a column) and `desc=1` order the selected rows, numbers by
value before other cells by text, keeping the file's order among equal
cells, for up to 1,000,000 rows. Rows keep their numbers in the file.
`/tableview/api/describe` summarizes each column over the rows the filters
select, as pandas' `describe` does: non-blank cells, distinct values (up to
100,000, then `unique_over`), and over the cells that read as numbers their
count, mean, sample standard deviation, minimum, maximum, and sum.
A cell whose trimmed text ends in an image, audio, or video extension (as
WEB-6 lists them) names a file: absolute as it is, or relative to the base
`base` names, `basedir` (the default), `wd` (the job's recorded working
directory), or `file` (the table's own directory). Each such cell of a page
comes with the file's properties read from its headers without decoding it
(format; duration, sample rate, channels, and bit depth for WAV, FLAC,
MP3, Ogg Vorbis and Opus, M4A, MP4, MOV, and WebM; width and height for
images and video; size; and a bitrate, the format's own for PCM WAV and
constant-bitrate MP3 and otherwise size over duration), or with why it is
not shown: outside the roots, through an escaping symlink, or missing.
`/tableview/api/media?ref=&base=` serves that file under the same roots and
refusals, and nothing that is not an image, audio, or video file.
A directory is read as a table instead, through the same roots without
following directory symlinks and reading at most 100,000 entries, of the
files `pattern` matches by their slash-separated path inside it (`*` within
a name, `**` any number of directories, `{a,b}` alternatives; default
`**/*`), sorted by path, and listed again after 10 seconds. `view=files`
(the default) has one row per file with its path, format, duration,
sample rate, channels, bitrate, width, height, size, and modification time
in typed columns: a filter on a duration takes seconds or `m:ss`, on a size
a binary suffix such as `10M`, and on a time compares RFC 3339 text.
`view=stems` has one row per stem and one column per group, `group` naming
how a path splits: `top` (the first directory; the stem is the rest
without its extension), `parent` (the file's directory and its name without
the extension), `extension`, or `suffix` (the name's end after the last
`suffix_sep`); without `group`, `extension` when no matched file is in a
subdirectory and `top` otherwise. A group without the stem's file has a
blank cell; `incomplete=1` keeps the stems with one; only the first file of
a stem in a group is listed, and the page's `notes` say how many were left
out, or that the listing was cut short. A directory's media paths are
relative to it. A file's parameters are refused for a directory and a
directory's for a file, rather than ignored.
`/tableview/api/info` names the file or directory, a file's size, download,
and the separator its extension suggests, and the bases. The page opens a cell's media in tabs of the active pane of panes
split right or down, one pane always active; tabs move between panes by
drag and drop, a drop near an edge splitting that pane; audio shows a
waveform and a spectrogram the browser computes from the decoded samples,
and linked panes share one time range and playback time. The page keeps the separator, header, media
base, directory listing, filters, sort, Describe, page, and page size in its
URL fragment, and drops
the filters and sort when the separator or the listing changes. Implemented by
[internal/tableview](../internal/tableview/) and
[internal/webui/table_view.go](../internal/webui/table_view.go), with the page in
[table_view.js](../internal/webui/assets/table_view.js) and
[table_view_panes.js](../internal/webui/assets/table_view_panes.js); covered by
`TestWebTableViewerReadsArtifacts` in
[conformance/05-web/table_view_test.go](../conformance/05-web/table_view_test.go), with
the rules in [internal/tableview/filter_test.go](../internal/tableview/filter_test.go),
[internal/tableview/directory_test.go](../internal/tableview/directory_test.go),
and [internal/tableview/media_test.go](../internal/tableview/media_test.go) (whose
sample files ffmpeg made, with ffprobe's values),
the page checked by `TestTableViewPagesInBrowser` and
`TestTableViewFiltersInBrowser`, `TestTableViewShowsMediaInBrowser`,
`TestTableViewGroupsStemsInBrowser`, `TestTableViewOpensFilesInPanesInBrowser`,
`TestTableViewMovesTabsAndLinksPanesInBrowser`, and
`TestSpectrogramFindsATone`, and the Launch with link by
`TestLaunchWithOpensTheTableViewerInBrowser` in
[internal/webui/table_view_test.go](../internal/webui/table_view_test.go).

**WEB-20** `rotari tableview PATH` serves the Table viewer (WEB-19) for one
delimited file or directory outside any project: the directory, or the
file's, is the only root anything is read under and the only base its
relative media paths resolve from, through `os.Root`, with the same
refusals. It listens on `--host` (default `127.0.0.1`) and `--port`
(default 8788, or the next free port when `--port` is not given), prints
`rotari tableview listening at http://ADDR`, serves the page's fonts and
design tokens itself, and, with `--auth-token` or `ROTARI_WEB_AUTH_TOKEN`,
requires the token as `rotari web` does. A missing path or no path fails
before listening. Implemented by
[cmd/rotari/tableview.go](../cmd/rotari/tableview.go) and `TableViewHandler`
in [internal/webui/table_view.go](../internal/webui/table_view.go); covered by
`TestTableViewCommandServesOnePath` in
[conformance/05-web/tableview_command_test.go](../conformance/05-web/tableview_command_test.go).

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
