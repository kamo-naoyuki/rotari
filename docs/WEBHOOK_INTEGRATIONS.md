# Webhook integrations

Rotari sends one JSON `POST` request when a run is finalized. The request is
service-independent, so the receiving workflow can forward it to Slack,
Discord, email, or another notification service.

## Payload

A completed run sends fields like these:

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

## Slack

Slack is supported directly through Incoming Webhooks. No adapter service is
needed.

### 1. Create the Slack destination

In Slack, create an app for the workspace and enable **Incoming Webhooks**.
Add a webhook for the channel where notifications should appear. Keep the
resulting Slack URL private.

### 2. Configure rotari

```sh
export ROTARI_WEBHOOK_URL='https://hooks.slack.com/services/T.../B.../...'
export ROTARI_WEBHOOK_ON='failure'
export ROTARI_WEBHOOK_FORMAT='slack'
```

Or put the settings in the project TOML config:

```toml
[webhook]
url = "https://hooks.slack.com/services/T.../B.../..."
on = "failure"
format = "slack"
```

`on` can be `always`, `success`, or `failure`; comma-separated values are also
accepted. A failed delivery is reported as a warning and does not change the
run result.

### 3. Test the notification

Run a small command that is expected to fail:

```sh
rotari add --run --project-name demo 'false'
```

Then check the configured Slack channel. If the run was already notified,
rotari will not send it again because it records `webhook.sent` in the run
directory.

## Other services

Microsoft Teams is also supported directly with `format: teams`. Create an
Incoming Webhook for the target channel, then configure:

```toml
[webhook]
url = "https://example.webhook.office.com/..."
on = "failure"
format = "teams"
```

The Teams message uses a MessageCard with the project, run, result counts, and
failed job details.

Discord is supported directly with `format: discord`. Create a webhook in the
target channel, then configure:

```toml
[webhook]
url = "https://discord.com/api/webhooks/..."
on = "failure"
format = "discord"
```

The Discord message uses an embed with the project, run, result counts, and
failed job details.

For Make Custom Webhooks, Zapier Catch Hooks, Pipedream HTTP triggers, or a
small internal HTTP service, leave `format` unset. Configure rotari with the
receiver's URL, then transform the generic JSON payload into the destination
service's required format.

Other services can use the generic JSON format through an adapter such as n8n,
Make, or Zapier.
