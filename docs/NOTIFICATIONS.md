# Notifications

Rotari supports local browser desktop notifications and outbound Webhook
notifications. Both channels are configured in `notifications.toml` and use
the same event and field vocabulary, while keeping independent settings.

Generate the file with `rotari config --notifications`, or edit it from the
Web UI's **Notifications** button. Rotari uses the first file it finds in the
project, the basedir, then the global config directory; scopes are not merged.

On a run page, **Notification config (read only)** displays the notification
config copied when that run started, rather than the current project settings.
You can copy its contents, but cannot edit, save, or reload it. The button is
disabled if the run has no notification config snapshot. This also works in
static exports. Run pages do not offer **Generate config**.

## Browser notifications

The Web UI can show a desktop notification when a run finishes or a job reaches
its final result. This is entirely local and never sends data to an external
service.

### Enabling browser notifications

Open a run or project page in `rotari web` and click `Enable notifications` in
the toolbar. After browser permission is granted, the button becomes a
`Notifications on`/`Notifications off` toggle. The toggle is stored in the
browser's `localStorage`, scoped by protocol, host, and port.

Use `--notifications=false` or `ROTARI_WEB_NOTIFICATIONS=false` on `rotari web`
to make the initial toggle state off. Once a browser has changed the toggle,
that explicit choice takes precedence over the server default.

### Browser events and details

The `[browser]` section controls local notifications:

```toml
[browser]
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = ["project", "run_id", "run_name", "run_status", "job_name", "attempt_id", "job_status", "exit_code", "diagnosis_name", "diagnosis_suggestion", "link"]
max_jobs = 10
```

A job is reported only after its retries end. Job results and a run completion
detected in the same polling tick are merged into one notification per run.
The first poll establishes a baseline and never reports existing history.
`max_jobs` limits how many jobs one notification lists, and the body is
truncated at 1000 characters.

The defaults include `run_id`, `job_name`, and `attempt_id` so a notification
identifies its run, job, and exact execution without repeating the job ID.
Those fields remain selectable. In browser notifications, the default `link`
opens the exact run page; removing it disables click navigation.

The current basedir's `/api/state` is checked every two seconds. Selected other
basedirs use `/api/active-runs`. Completed run details are not repeatedly
loaded. Static exports have no server to load notification settings, so they
provide neither the editor nor live notifications. `--notifications` still
selects the initial local toggle shown in a static export. Live-server options
(`--host`, `--port`, `--auth-token`, and `--allow-control`) cannot be combined
with `--static-dir`; use them only when starting the HTTP server.

## Webhook notifications

Rotari can send JSON `POST` requests for job results and run completions.
Generic JSON, Slack, Microsoft Teams, and Discord formats are supported.
Keep the URL out of the file by setting `ROTARI_WEBHOOK_URL`, which overrides
`webhook.url`.

### Webhook events and details

The `[webhook]` section controls outbound notifications:

```toml
[webhook]
url = "https://example.invalid/hook"
format = "json"
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = ["project", "run_id", "run_name", "run_status", "job_name", "attempt_id", "job_status", "exit_code", "success_count", "failure_count", "duration", "diagnosis_name", "diagnosis_suggestion"]
max_jobs = 20
```

A job is reported once after its retries end. Blocked jobs and jobs cancelled
before they start are failures. Events within ten seconds of the first pending
event are combined, and run completion flushes the pending batch immediately.
`max_jobs` limits the listed jobs; aggregate success and failure counts still
cover the whole run.

Delivery errors are warnings and do not change the run result. Rotari keeps no
delivery record and does not retry.

### Slack

Create a Slack app, enable **Incoming Webhooks**, and use its URL:

```toml
[webhook]
url = "https://hooks.slack.com/services/T.../B.../..."
format = "slack"
```

The message uses Slack Block Kit and displays the configured fields.

### Microsoft Teams

Create an Incoming Webhook for the target channel:

```toml
[webhook]
url = "https://example.webhook.office.com/..."
format = "teams"
```

The message uses a MessageCard and displays the configured fields.

### Discord

Create a webhook in the target channel:

```toml
[webhook]
url = "https://discord.com/api/webhooks/..."
format = "discord"
```

The message uses embeds and displays the configured fields.

### Testing a Webhook

Keep `run_failure = true` and run a command expected to fail:

```sh
rotari add --project-name demo false && rotari run --project-name demo
```

### Generic JSON

For Make, Zapier, Pipedream, or an internal HTTP service, use the default
`json` format. Each event contains the configured fields available to it:

```json
{
  "schema_version": 1,
  "created_at": "2026-09-30T17:41:02Z",
  "events": [
    {
      "event": "job.finished",
      "fields": [
        {"name": "project", "value": "demo"},
        {"name": "run_id", "value": "20260930-174102-9c8bb0d5"},
        {"name": "job_name", "value": "train"},
        {"name": "job_status", "value": "failed"},
        {"name": "exit_code", "value": 1},
        {"name": "diagnosis_name", "value": "Permission denied"},
        {"name": "diagnosis_suggestion", "value": "Check file ownership and permissions."}
      ]
    },
    {
      "event": "run.finished",
      "fields": [
        {"name": "project", "value": "demo"},
        {"name": "run_id", "value": "20260930-174102-9c8bb0d5"},
        {"name": "run_status", "value": "failed"},
        {"name": "exit_code", "value": 1},
        {"name": "success_count", "value": 0},
        {"name": "failure_count", "value": 1}
      ]
    }
  ]
}
```

## Events and fields

Both channels independently select job failures, job successes, run failures,
and run successes. `fields` selects values in display order. Fields that do
not apply to an event are omitted, and multiple diagnoses produce repeated
diagnosis fields. The event type, payload schema version, and creation time are
always present in generic JSON.

Browser and Webhook notifications share the following vocabulary. Only `link`
is browser-only and is rejected in `[webhook].fields`.

| Category | Fields |
| --- | --- |
| Identity | `project`, `run_id`, `run_name`, `job_id`, `job_name`, `stage`, `array_task_id`, `attempt_id` |
| Result | `run_status`, `job_status`, `exit_code`, `error`, `success_count`, `failure_count`, `total_count` |
| Time | `started_at`, `finished_at`, `duration` |
| Execution | `executor`, `hosts`, `working_directory`, `command` |
| Diagnosis | `diagnosis_status`, `diagnosis_name`, `diagnosis_evidence`, `diagnosis_suggestion`, `diagnosis_rules`, `diagnosis_outdated` |
| Navigation | `link` (browser only) |

`working_directory`, `command`, and `diagnosis_evidence` can contain sensitive
or lengthy values, so they are disabled by default. Environment variables and
executor options cannot be selected.
