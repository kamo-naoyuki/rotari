// Types for the Web UI scripts, which share one global scope (see
// contracts/05-web-assets-and-static-export.md). Checked by
// `npm run typecheck`; nothing here is shipped.

// Values assets.go writes into the page script.
declare const __ROTARI_EXECUTORS__: string[];
declare const __ROTARI_BASEDIRS__: { id: string; path: string; current: boolean }[];
declare const __ROTARI_STATUS_TONES__: Record<string, string>;
declare const diagnosisGuidance: { noMatchNext: string; unavailableNext: string; outdatedNote: string };

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
  result?: WebJobResult | null;
}
interface WebTimelinePoint {
  at: string;
  pending?: number;
  running?: number;
  success?: number;
  failed?: number;
}
interface WebLoadAverage {
  one: number;
  five: number;
  fifteen: number;
}
interface WebLoadSample extends WebLoadAverage {
  at: string;
}
interface WebRun {
  run_id: string;
  run_name?: string;
  running?: boolean;
  jobs?: WebJob[];
  timeline?: WebTimelinePoint[];
  context?: {
    load_samples?: WebLoadSample[];
    started_load?: WebLoadAverage;
  };
}
interface WebProject {
  project_name: string;
  runs: WebRun[];
}
