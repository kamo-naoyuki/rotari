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
max_jobs = 20
```

A job is reported once, after its retries end, so a job that succeeds on a
retry is reported only as a success. Blocked jobs and jobs cancelled before
they start are reported as failures. Events that happen close together are
combined: the first pending event opens a ten-second window, and a run's
completion sends the pending batch immediately. `max_jobs` bounds how many
jobs a single notification lists.

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

The message uses Slack Block Kit and includes the project, run, result counts,
and failed-job details when present.

### Microsoft Teams

Create an Incoming Webhook for the target Teams channel. Configure rotari with
that webhook URL:

```toml
[webhook]
url = "https://example.webhook.office.com/..."
format = "teams"
```

The message uses a MessageCard and includes the project, run, result counts,
and failed-job details when present.

### Discord

Create a webhook in the target Discord channel. Configure rotari with that
webhook URL:

```toml
[webhook]
url = "https://discord.com/api/webhooks/..."
format = "discord"
```

The message uses an embed and includes the project, run, result counts, and
failed-job details when present.

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
payload into the destination service's required format. The payload contains
the run result and, for failed runs, failed-job details:

```json
{
  "event": "run.finished",
  "project": "demo",
  "run": "nightly (run-1)",
  "status": "failed",
  "exit_code": 1,
  "success": 3,
  "failed": 1,
  "failed_jobs": ["train"],
  "show_command": "rotari show --run-id 'run-1' --failed-logs --no-pager"
}
```

`failed_jobs` and `show_command` are omitted for a successful run.
