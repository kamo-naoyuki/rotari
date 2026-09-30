# Web browser notifications

The Web UI can show a browser desktop notification when a run finishes or a
job fails. This is entirely local and never talks to an external service. The
page polls lightweight project/run metadata and the details of active runs so
it can monitor failures without repeatedly loading completed run histories.
This is the local-only alternative to
[Webhook integrations](WEBHOOK_NOTIFICATIONS.md) for anyone who cannot send run
data outside their network.

## Enabling notifications

Open a run or project page in `rotari web` and click `Enable notifications` in
the toolbar. The browser asks for notification permission once; after
granting it, the button becomes a `Notifications on`/`Notifications off`
toggle you can click to pause or resume notifications without revoking the
browser permission (which the page cannot do itself once granted).

## What triggers a notification

- A run finishing, successfully or not.
- A job reaching its final result, even before its run finishes.

A job is reported once, after its retries end, so a job that succeeds on a
retry is not reported as a failure. Job results and a run's own completion
detected in the same polling tick
(the current basedir's `/api/state` is checked every 2 seconds, and selected
other basedirs use `/api/active-runs`) are merged into one notification per
run; events detected at different times stay separate. Project queues and run
summaries are loaded when a project is opened; run details are fetched when
opened and active-run details are refreshed while those runs are executing. Completed run details are not repeatedly fetched.
The very first poll after loading the page never triggers a notification, so
existing run history never causes a notification burst when you open the page.

Clicking a notification focuses the tab and opens the corresponding run page.

## Choosing events and detail

The `[browser]` section of `notifications.toml` selects what is shown. It uses
the same settings as `[webhook]` but keeps its own values:

```toml
[browser]
job_failure = true
job_success = false
run_failure = true
run_success = true
max_jobs = 10
```

Generate the file with `rotari config --notifications`, or edit it from the
Web UI's **Notifications** button, which saves the file and reloads the
settings without restarting `rotari web`. Open pages pick the new settings up
on their next poll. `max_jobs` bounds how many jobs one notification lists,
and the notification body is truncated at 1000 characters.

The static export has no server to read or save these files, so it offers
neither the editor nor live notifications.

## Where the setting is stored

The on/off toggle is stored in the browser's `localStorage`, scoped per
origin (protocol + host + port). That means:

- It survives closing the tab or restarting the `rotari web` process; it does
  not survive switching to a different host or port, or clearing the browser's
  site data.
- Two different `rotari web` addresses (e.g. different ports) each have their
  own independent setting.

## Changing the default

Use `--notifications=false` or `ROTARI_WEB_NOTIFICATIONS=false` on `rotari web`
to make the toggle start out off. This only changes the starting value for a
browser origin that has never used the toggle before; once a browser has
clicked it, that explicit choice always takes precedence over the server
default.
