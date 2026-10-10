# Plan: Visual Refresh of the Web UI

**Created:** 2026-10-11
**Status:** Proposed; decisions made, Phase 0 not started
**Related:** [Recent activity view](../2026-10-11-recent-activity-view/plan.md), whose mockup the user liked and asked to carry over to the rest of the Web UI

## Purpose

The user found the Activity mockup much better looking than the current Web
UI and wants the existing pages brought up to that level. The goal is a UI
that reads at a glance: what is running, what failed, where. Better looks
are the goal, not new features; every page keeps its information and
controls.

## What the UI looks like today

Checked at `01652576` with a static export of a small state (two projects,
three runs, one failing array), screenshotted with headless Chrome at
1400×1000.

- **Header.** Every page is titled "rotari Web" (the jobs page "rotari Job
  activity"), not after what it shows. The subtitle is the raw absolute state
  path, which wraps over two lines. Up to five equal-weight buttons (View
  config, Generate config, Notification config, Generate notification config,
  Refresh) wrap into two- and three-line buttons and outweigh the content.
- **Facts as boxes.** The run header shows Project, Run ID, Status, Lifecycle,
  Client, Exit, Jobs, Failure causes, and Origins as identical bordered
  chips. "Status: failed" is plain text, so the most important fact has the
  least emphasis.
- **Boxes inside boxes.** A dashed container holds a panel that holds a
  dashed "Queue is empty" box; borders nest three deep.
- **Tables.** Column headers wrap ("RUN-NAME", "PROJECT ↕" on two lines). Run
  and attempt IDs and timestamps break mid-value. Each command and attempt
  cell carries its own copy button, doubling row height on the jobs page.
  Status on the jobs page is coloured text only; "success (carried)" looks
  like plain success.
- **Hidden visuals.** On the run page, Run statistics, Job timeline, Load
  average, and Output word cloud are all collapsed, so the page's charts are
  not visible until opened.
- **Page edges.** The background and sidebar stop short of the window, leaving
  a dark band at the bottom of a page shorter than the window.
- **Sidebar.** Basedir paths are truncated from the end
  (`/tmp/claude-22156/-…`), hiding the part that tells basedirs apart.
- **Type.** No font is set beyond `ui-sans-serif, system-ui`, so Linux
  browsers fall back to DejaVu Sans.

### Why it is hard to change

The look is applied after rendering. Bootstrap code runs a chain of DOM
patches (`labelJobActionHeaders`, `moveActionColumnsLeft`,
`mergeLogButtonIntoActions`, `styleActionColumns`, `clarifyLogControls`, ...)
that find headers by their text, rename them, move columns, and set inline
styles: `web_app_charts.js` alone assigns `.style.*` 351 times, and
`mergeActionColumns` is defined twice, the second as an empty override. The
three stylesheets have about 220 lines with literal colours beside a handful
of tokens.
A visual change today means editing renderers, patches, and CSS that each
assume the others' output, which is the "one rule in several places" pattern
that AGENTS.md warns about. It already causes a visible bug: the projects
table's Started column is relabelled "Actions" and moved first (recorded in
[ISSUES.md](../ISSUES.md)).

## Scope

- Every page of the live server and the static export: projects, project,
  run, job/attempt modals, jobs, history search, config and notification
  dialogs, and the sidebar.
- The look: tokens, type, spacing, page header, status display, tables,
  section layout, charts' colours.
- The structure that makes the look maintainable: renderers emit final markup
  with classes; the post-render patches and inline styles go away.

## Non-goals

- New information or controls, except where a decision below says so.
- A front-end framework, a build step, or any new dependency. The assets stay
  plain JS and CSS embedded in the binary.
- External fonts or CDNs: the live server and the static export must work
  offline (for example on a compute node).
- Changing API responses.

## Design direction

Carry over what made the Activity mockup read well:

- **Tokens first.** One `:root` set of colour, type, spacing, and radius
  tokens; components use only tokens. Status colours (succeeded, failed,
  cancelled, interrupted, running, carried) are tokens reserved for status.
- **One status component.** Status everywhere is a pill with icon and label
  (✓ ✕ ■ ! ▶), coloured by the status tokens, with "carried" as its own
  visible variant. Used on every page and in the Activity page.
- **Hierarchy in the header.** The page title is what the page shows: the
  project name, or the run name with its status pill. A breadcrumb
  (basedir › project › run) replaces the raw path; the full path stays one
  click away (copy). Secondary facts become one quiet label/value line.
- **Fewer frames.** Borders and fills only where they separate an object;
  no dashed wrappers around panels.
- **Tables that do not wrap their keys.** IDs, times, and headers stay on one
  line, with long IDs shortened in the middle and complete on hover and copy;
  copy buttons appear on row hover and keyboard focus; `tabular-nums` for
  numbers and times; sticky headers on long tables.
- **Charts visible at rest.** The run page shows the job timeline open by
  default; other sections remember their open state per browser.
- **Settings in one menu.** The config and notification buttons move into a
  single "Settings" menu; Refresh stays visible.

## Approach

Each phase is shown first as a sample-data mockup (an artifact, as for the
Activity page) and implemented only after the user accepts it.

### Phase 0: screenshot baseline

A development script builds a fixed fixture state (plain job, failing array,
carried results, an active run, notes, artifacts), exports it with
`rotari web --static-dir`, and screenshots each page with headless Chrome at
desktop and phone widths. Run before and after each phase to compare. It is
a review aid, not a test: CI does not depend on a browser.

### Phase 1: tokens and status component

Collect the colours, sizes, and fonts into tokens and replace literals; add
the status pill and use it wherever a status is rendered; add the light theme
and the theme choice; embed the fonts. Apart from these, a visual no-op.

### Phase 2: renderers own their markup

Make the table and header renderers emit the final column order, labels, and
classes, and delete the post-render patches and inline styles they replace.
This fixes the mislabelled Started column. The rewritten files get JSDoc
types and join the `tsc --checkJs` check. Every page must look the same as
after Phase 1 except for that fix; the Phase 0 screenshots check it.

### Phase 3: page shell

Page titles and breadcrumbs, the Settings menu, sidebar paths truncated in
the middle, full-height background and sidebar.

### Phase 4: page bodies

Run header hierarchy, tables (no-wrap keys, hover copy, sticky headers),
fewer frames, charts open at rest, the jobs page with status pills and a
compact command/attempt layout.

The Activity page is built on the Phase 1 tokens and status component, so
Phase 1 should land before the activity plan's Phase 2.

## Decisions

- **Light theme (2026-10-11).** Add a light theme. The UI follows the OS
  setting (`prefers-color-scheme`); a Light / Dark / System choice in the
  Settings menu overrides it and is remembered per browser. Both themes are
  designed from the same tokens, not by inverting colours, and status and
  chart colours are checked for contrast on both backgrounds. The static
  export behaves the same. Part of Phase 1.
- **Embedded fonts (2026-10-11).** Ship one sans and one monospace face in
  the binary as WOFF2, served by the live server and copied into the static
  export, so every machine renders the same. Constraints: an open licence
  (SIL OFL or similar) whose text is shipped beside the files; a Latin subset
  with only the weights used (about 100–200 KB in all); the system font stack
  stays as fallback, which also covers CJK characters in job names and paths.
  The faces are chosen with the Phase 1 mockup. Adding them is a new asset,
  not a new code dependency.
- **Language (2026-10-11).** The assets stay plain JavaScript, embedded as
  they are, so `go build` alone still produces a complete binary and no
  generated files are committed. Types are added as JSDoc annotations and
  checked by `tsc --checkJs --noEmit` (a `typescript` devDependency beside
  Prettier and jsdom, run in CI), starting with the files Phase 2 rewrites
  and widening from there. A full TypeScript build is reconsidered only if
  the front end grows well beyond today's ~10,000 lines.
- **Run page sections (2026-10-11).** The job timeline is open by default;
  the other sections remember their open state per browser.

## Tests and contracts

- `internal/webui` tests that assert rendered text (for example the "Actions"
  header, button labels) are updated only where a phase intentionally changes
  that text; each such change is listed in the work log.
- `conformance/` WEB rows must pass unchanged; the Web UI's behaviour does not
  change.
- `scripts/check.sh` before each commit; the Phase 0 screenshots reviewed for
  each phase.
- Docs: descriptions of the Web UI (`docs/INSPECT.md`, `docs/NOTIFICATIONS.md`,
  `docs/GETTING_STARTED.md`) updated where they name changed labels or layout.
