# Plan: Unified Notification Settings and Events

**Created:** 2026-10-01

## Purpose

Use a common event model and settings vocabulary for webhook and browser notifications, while keeping settings independent per channel. Notify on final job outcomes and run completion, group nearby events, and make settings manageable through CLI/Web configuration. Rule-based diagnosis is a primary failure detail.

## Settings and defaults

Store settings in `notifications.toml`, resolved as a whole file in project, basedir, then global precedence (first existing file wins; do not merge scopes). `[webhook]` and `[browser]` share field names but have independent values. Event switches are `job_failure` (true), `job_success` (false), `run_failure` (true), and `run_success` (true). A job event represents the final job outcome, not each retry attempt. Blocked, cancelled, and cancelled-before-start jobs count as failures.

Include diagnosis name and suggestion by default; keep evidence, command, working directory, environment, and executor options out of defaults. `ROTARI_WEBHOOK_URL` may override the configured secret URL; other settings come from the file. Remove legacy `webhook.on` and format environment variables rather than silently migrating them. A run uses a snapshot of webhook settings captured at start; browser settings use the Web server's current successfully loaded snapshot.

## Shared event and delivery behavior

A channel-neutral notification package should own settings validation/defaults, field registry and ordering, event filtering, event/batch models, and display projection. Webhook HTTP encoders and browser delivery are adapters.

Group events by project and run. Start a fixed 10-second window at the first event and do not extend it; flush immediately at run completion. Keep success and failure in the same batch but label them. Enforce per-channel `max_jobs`, disclose omitted jobs, and limit browser text to 1000 characters. Delivery failure must not alter run/job results. The initial design does not require a persistent webhook delivery ledger or automatic retries.

## Browser and configuration management

Preserve browser permission, local opt-in, and first-poll baseline behavior. Reload settings atomically after successful save/reload; preserve the current snapshot on parse/validation failure. The Web UI should provide notification-specific structured editing, generate/save/reload, destination scope selection, and secret-safe handling. Static export has no live server and therefore cannot provide settings writes/reload or live notifications. Writing config remains behind existing control authorization; reading must not expose webhook URLs to unauthenticated remote users.

CLI config commands should support generating, showing, and validating notification settings using the repository's command/schema generation path. API responses, HTML, and logs must not reveal webhook secrets; mask existing values and accept a new URL only when changed.

## Delivery phases

1. Shared model, settings resolver, defaults, validation, template, and tests.
2. Final-job event hook and run completion events.
3. Webhook adapters, batching, fixed window, flush, and run-start snapshot.
4. Browser projection and notification manager with reload/revision behavior.
5. CLI and Web UI generate/show/validate/edit/save/reload flows.
6. Contracts, conformance, architecture, generated references, and user documentation.

## Current status

The shared `notifications.toml` settings direction and configurable fields have landed, as have Web settings/configuration surfaces and notification UX fixes. The plan's original checklist predates some of this implementation and is not an authoritative completion report. Before continuing, inspect current notification package, Web/API behavior, CLI support, contracts, and tests; then update the concrete remaining items here. Important invariants are per-channel settings, secret-safe URL handling, per-run webhook snapshot, atomic browser reload, final-result-only job notifications, and bounded aggregation.

## Validation

Test config precedence and validation; event inclusion and final-vs-retry behavior; grouping, limits, and flush; each webhook encoder; failed delivery; browser baseline/local settings/revision; atomic reload rollback; secret masking and authorization. Run focused package tests, relevant CLI/Web/conformance tests, then `scripts/check.sh`.
