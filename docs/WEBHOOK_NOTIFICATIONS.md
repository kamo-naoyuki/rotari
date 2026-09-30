# Webhook integrations

Rotari sends one JSON `POST` request when a run is finalized. Slack, Microsoft
Teams, and Discord are supported directly with service-specific payloads.
Other destinations can receive the generic JSON payload and transform it as
needed.

## Generic JSON payload

The default `json` format sends fields like these:

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

## Direct integrations

Choose one of `slack`, `teams`, or `discord` for `webhook.format`. Each format
posts directly to that service's incoming webhook; no adapter service is
needed. The `url` is the service's webhook URL. Set `on` to `always`,
`success`, or `failure`; comma-separated values are also accepted. A failed
delivery is reported as a warning and does not change the run result.

### Slack

Create a Slack app for the workspace, enable **Incoming Webhooks**, and add a
webhook for the target channel. Keep the resulting URL private.

```toml
[webhook]
url = "https://hooks.slack.com/services/T.../B.../..."
on = "failure"
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
on = "failure"
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
on = "failure"
format = "discord"
```

The message uses an embed and includes the project, run, result counts, and
failed-job details when present.

## Test a notification

For any of the direct integrations, configure `on = "failure"` and run a small
command that is expected to fail:

```sh
rotari add --project-name demo -- false && rotari run --project-name demo
```

Then check the configured destination. If the run was already notified,
rotari will not send it again because it records `webhook.sent` in the run
directory.

## Other destinations

For Make Custom Webhooks, Zapier Catch Hooks, Pipedream HTTP triggers, or a
small internal HTTP service, leave `format` unset (the default `json`).
Configure rotari with the receiver's URL, then transform the generic JSON
payload into the destination service's required format.
