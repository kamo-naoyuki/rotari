// Types for the Web UI scripts, which share one global scope (see
// contracts/05-web-assets-and-static-export.md). Checked by
// `npm run typecheck`; nothing here is shipped.

// Values assets.go writes into the page script.
declare const __ROTARI_EXECUTORS__: string[];
declare const __ROTARI_BASEDIRS__: { id: string; path: string; current: boolean }[];
declare const __ROTARI_STATUS_TONES__: Record<string, string>;
declare const diagnosisGuidance: { noMatchNext: string; unavailableNext: string; outdatedNote: string };

// Defined by web_shell.js, which runs before the page script.
declare function notificationUnavailableReason(): string;

// Static-export data that web_static_bootstrap.js puts on the window.
interface Window {
  __ROTARI_STATIC_STATE__?: unknown;
  __ROTARI_STATIC_WORD_CLOUDS__?: Record<string, unknown>;
}

// The run, job, and project shapes the scripts read from /api/state,
// /api/project, and /api/run (internal/web models), as far as the checked
// scripts use them.
interface WebJobResult {
  exit_code: number;
  error?: string;
  hosts?: string[];
}
interface WebJob {
  id: string;
  name?: string;
  elapsed?: string;
  result?: WebJobResult | null;
}
interface WebTimelinePoint {
  at: string;
  pending?: number;
  running?: number;
  success?: number;
  failed?: number;
}
interface WebJobLane {
  job_id: string;
  start?: string;
  end?: string;
  carried?: boolean;
}
interface WebLoadAverage {
  one: number;
  five: number;
  fifteen: number;
}
interface WebLoadSample extends WebLoadAverage {
  at: string;
}
// WebFailureGroup is runlineage.FailureGroup.
interface WebFailureGroup {
  kind: string;
  cause: string;
  suggestion?: string;
  count: number;
  carried?: number;
  exit_codes: number[];
  example: { job_id: string; attempt_id?: string; evidence?: string };
  jobs: { id: string; name?: string; array_task_id?: number; carried?: boolean }[];
}
interface WebRun {
  run_id: string;
  run_name?: string;
  running?: boolean;
  cwd?: string;
  jobs?: WebJob[];
  lineage_summary?: { failures?: WebFailureGroup[] };
  timeline?: WebTimelinePoint[];
  lanes?: WebJobLane[];
  context?: {
    load_samples?: WebLoadSample[];
    started_load?: WebLoadAverage;
  };
}
interface WebProject {
  project_name: string;
  runs: WebRun[];
}
