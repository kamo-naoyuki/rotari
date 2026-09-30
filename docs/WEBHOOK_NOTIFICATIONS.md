# Webhook integrations

Rotari sends JSON `POST` requests for job results and run completions. Slack,
Microsoft Teams, and Discord are supported directly with service-specific
payloads.

Webhooks are configured in `notifications.toml`, not in `config.toml`. Generate
one with `rotari config --notifications`, or from the Web UI's **Notifications**
button. Rotari uses the first file it finds in the project, the basedir, then
the global config directory; scopes are not merged. Keep the URL out of the
file by setting `ROTARI_WEBHOOK_URL` instead.

## Choosing events

Four settings select what is sent:

```toml
[webhook]
url = "https://example.invalid/hook"
format = "json"
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = ["project", "run_id", "run_name", "run_status", "job_id", "job_name", "job_status", "exit_code", "success_count", "failure_count", "duration", "diagnosis_name", "diagnosis_suggestion"]
max_jobs = 20
```

A job is reported once, after its retries end, so a job that succeeds on a
retry is reported only as a success. Blocked jobs and jobs cancelled before
they start are reported as failures. Events that happen close together are
combined: the first pending event opens a ten-second window, and a run's
completion sends the pending batch immediately. `max_jobs` bounds how many
jobs a single notification lists.

`fields` selects the values reported for each event, in display order. A field
that does not apply to an event is omitted; for example, `job_name` is omitted
from a run event. Multiple diagnoses produce multiple diagnosis fields. The
event type, payload schema version, and notification creation time are always
included. The same selection controls generic JSON and the Slack, Teams, and
Discord messages.

## Available fields

Webhook and browser notifications share the same field vocabulary. Only
`link` is browser-only and is rejected in `[webhook].fields`.

| Category | Fields |
| --- | --- |
| Identity | `project`, `run_id`, `run_name`, `job_id`, `job_name`, `stage`, `array_task_id`, `attempt_id` |
| Result | `run_status`, `job_status`, `exit_code`, `error`, `success_count`, `failure_count`, `total_count` |
| Time | `started_at`, `finished_at`, `duration` |
| Execution | `executor`, `hosts`, `working_directory`, `command` |
| Diagnosis | `diagnosis_status`, `diagnosis_name`, `diagnosis_evidence`, `diagnosis_suggestion`, `diagnosis_rules`, `diagnosis_outdated` |
| Navigation | `link` (browser only) |

`working_directory`, `command`, and `diagnosis_evidence` can contain sensitive
or lengthy values, so they are available but disabled by default. Environment
variables and executor options cannot be selected.

## Direct integrations

Choose one of `slack`, `teams`, or `discord` for `webhook.format`. Each format
posts directly to that service's incoming webhook; no adapter service is
needed. The `url` is the service's webhook URL. A failed delivery is reported
as a warning and does not change the run result.

### Slack

Create a Slack app for the workspace, enable **Incoming Webhooks**, and add a
webhook for the target channel. Keep the resulting URL private.

```toml
[webhook]
url = "https://hooks.slack.com/services/T.../B.../..."
format = "slack"
```

The message uses Slack Block Kit and displays the configured `fields`.

### Microsoft Teams

Create an Incoming Webhook for the target Teams channel. Configure rotari with
that webhook URL:

```toml
[webhook]
url = "https://example.webhook.office.com/..."
format = "teams"
```

The message uses a MessageCard and displays the configured `fields`.

### Discord

Create a webhook in the target Discord channel. Configure rotari with that
webhook URL:

```toml
[webhook]
url = "https://discord.com/api/webhooks/..."
format = "discord"
```

The message uses embeds and displays the configured `fields`.

## Test a notification

For any of the direct integrations, keep `run_failure = true` and run a small
command that is expected to fail:

```sh
rotari add --project-name demo -- false && rotari run --project-name demo
```

Then check the configured destination. Rotari keeps no delivery record and
does not retry, so a failed delivery is reported only as a warning.

## Other destinations

For Make Custom Webhooks, Zapier Catch Hooks, Pipedream HTTP triggers, or a
small internal HTTP service, leave `format` unset (the default `json`).
Configure rotari with the receiver's URL, then transform the generic JSON
payload into the destination service's required format. Each event contains
the configured fields that are available for that event:

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
