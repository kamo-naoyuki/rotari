function notificationsSupported() {
  return typeof Notification !== "undefined";
}
const notificationIconURL = "__ROTARI_NOTIFICATION_ICON__";
const notificationsDefaultOn = __ROTARI_NOTIFICATION_DEFAULT__;
const initialNotificationSettings = __ROTARI_NOTIFICATION_SETTINGS__;
const notificationsEnabledKey = "rotari-notifications-enabled";
const notificationSettingsByProject = new Map();
function notificationSettingsKey(projectName, basedirID) {
  return (basedirID || mountedBasedirID || "") + "\0" + projectName;
}
const defaultNotificationSettings = {
  job_failure: true,
  job_success: false,
  run_failure: true,
  run_success: true,
  fields: [
    "project",
    "run_id",
    "run_name",
    "run_status",
    "job_name",
    "attempt_id",
    "job_status",
    "exit_code",
    "diagnosis_name",
    "diagnosis_suggestion",
    "link",
  ],
  max_jobs: 10,
};
Object.assign(defaultNotificationSettings, initialNotificationSettings);
async function notificationSettings(projectName, basedirID) {
  const key = notificationSettingsKey(projectName, basedirID);
  if (notificationSettingsByProject.has(key))
    return notificationSettingsByProject.get(key);
  try {
    const params = new URLSearchParams({ project_name: projectName });
    const url =
      (basedirID
        ? basedirURL(basedirID, "/api/notification-settings")
        : "/api/notification-settings") +
      "?" +
      params;
    const response = await fetch(url, {
      cache: "no-store",
    });
    if (response.ok) {
      const settings = await response.json();
      notificationSettingsByProject.set(key, settings);
      return settings;
    }
  } catch (error) {
    // Use defaults after temporary configuration failures.
  }
  return defaultNotificationSettings;
}
function notificationsEnabled() {
  const stored = localStorage.getItem(notificationsEnabledKey);
  if (stored === null) return notificationsDefaultOn;
  return stored !== "false";
}
function setNotificationsEnabled(enabled) {
  localStorage.setItem(notificationsEnabledKey, enabled ? "true" : "false");
}
function updateNotifyToggleLabel() {
  const button = document.getElementById("notify-toggle");
  if (!button) return;
  if (!notificationsSupported()) {
    button.hidden = true;
    return;
  }
  button.disabled = Notification.permission === "denied";
  const enabled =
    Notification.permission === "granted" && notificationsEnabled();
  button.textContent = enabled ? "Notification on" : "Notification off";
  button.classList.toggle("notifications-on", enabled);
  button.setAttribute("aria-pressed", String(enabled));
}
async function toggleRunNotifications() {
  if (!notificationsSupported()) return;
  if (Notification.permission === "default") {
    await Notification.requestPermission();
  } else if (Notification.permission === "granted") {
    setNotificationsEnabled(!notificationsEnabled());
  }
  updateNotifyToggleLabel();
}
// runSucceeded mirrors webhook notifications: a run succeeded when its exit
// code is 0; a failed or cancelled run, or one without an exit code, did not.
function runSucceeded(run) {
  return !!run && run.exit_code === 0;
}
function collectRunStatuses(appState) {
  const statuses = new Map();
  if (!appState || !appState.projects) return statuses;
  for (const project of appState.projects) {
    for (const run of project.runs || []) {
      statuses.set(project.project_name + "/" + run.run_id, {
        running: !!run.running,
        status: run.status,
        projectName: project.project_name,
        runID: run.run_id,
        run,
      });
    }
  }
  return statuses;
}
function collectJobStatuses(appState) {
  const statuses = new Map();
  if (!appState || !appState.projects) return statuses;
  for (const project of appState.projects) {
    for (const run of project.runs || []) {
      for (const job of run.jobs || []) {
        statuses.set(project.project_name + "/" + run.run_id + "/" + job.id, {
          status: jobDisplayStatus(job, run),
          final: !!job.final,
        });
      }
    }
  }
  return statuses;
}
function truncateNotificationBody(body) {
  if (body.length <= 1000) return body;
  return body.slice(0, 997) + "...";
}
function notificationDuration(startedAt, finishedAt) {
  const milliseconds = Date.parse(finishedAt) - Date.parse(startedAt);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return "";
  if (milliseconds < 1000) return milliseconds + "ms";
  const seconds = milliseconds / 1000;
  return (Number.isInteger(seconds) ? seconds : seconds.toFixed(3)) + "s";
}
function notificationFieldValues(name, info, job, jobStatus) {
  const run = info.run || {};
  const result = job && job.result;
  const diagnoses = (result && result.diagnoses) || [];
  const runJobs = run.jobs || [];
  const value = (item) =>
    item === undefined || item === null || item === "" ? [] : [item];
  switch (name) {
    case "project":
      return value(info.projectName);
    case "run_id":
      return value(info.runID);
    case "run_name":
      return value(run.run_name);
    case "run_status":
      return job ? [] : value(runSucceeded(info.run) ? "success" : "failed");
    case "job_id":
      return job ? value(job.id) : [];
    case "job_name":
      return job ? value(job.name) : [];
    case "stage":
      return job ? value(job.stage) : [];
    case "array_task_id":
      return job ? value(job.array_task_id) : [];
    case "attempt_id":
      return job ? value(job.attempt_id || (result && result.attempt_id)) : [];
    case "job_status":
      return job ? value(jobStatus) : [];
    case "exit_code":
      return value(job ? result && result.exit_code : run.exit_code);
    case "error":
      return job ? value(result && result.error) : [];
    case "success_count":
      return job
        ? []
        : value(
            runJobs.filter(
              (item) =>
                item.final && jobDisplayStatus(item, run).startsWith("success"),
            ).length,
          );
    case "failure_count":
      return job
        ? []
        : value(
            runJobs.filter(
              (item) =>
                item.final &&
                !jobDisplayStatus(item, run).startsWith("success"),
            ).length,
          );
    case "total_count":
      return job ? [] : value(runJobs.filter((item) => item.final).length);
    case "started_at":
      return value(job ? job.submitted_at : run.started_at);
    case "finished_at":
      return value(job ? job.finished_at : run.finished_at);
    case "duration":
      return value(
        notificationDuration(
          job ? job.submitted_at : run.started_at,
          job ? job.finished_at : run.finished_at,
        ),
      );
    case "executor":
      return job ? value(job.executor) : [];
    case "hosts":
      return job
        ? value(result && result.hosts && result.hosts.join(", "))
        : [];
    case "working_directory":
      return job ? value(job.working_directory) : [];
    case "command":
      return job
        ? value(((result && result.command) || job.command || []).join(" "))
        : [];
    case "diagnosis_status":
      return job ? value(result && result.diagnosis_status) : [];
    case "diagnosis_name":
      return job
        ? diagnoses.map((diagnosis) => diagnosis.name).filter(Boolean)
        : [];
    case "diagnosis_evidence":
      return job
        ? diagnoses.map((diagnosis) => diagnosis.evidence).filter(Boolean)
        : [];
    case "diagnosis_suggestion":
      return job
        ? diagnoses.map((diagnosis) => diagnosis.suggestion).filter(Boolean)
        : [];
    case "diagnosis_rules":
      return job ? value(result && result.diagnosis_rules) : [];
    case "diagnosis_outdated":
      return job && result && result.diagnosis_status
        ? [!!job.diagnosis_outdated]
        : [];
    default:
      return [];
  }
}
function notificationEventLines(settings, info, job, jobStatus) {
  const lines = [];
  for (const name of settings.fields || []) {
    if (name === "link") continue;
    for (const value of notificationFieldValues(name, info, job, jobStatus))
      lines.push(name.replaceAll("_", " ") + ": " + value);
  }
  return lines;
}
function notifyRunEvent(
  info,
  succeededJobs,
  failedJobs,
  runFinished,
  settings,
) {
  if (
    !notificationsSupported() ||
    Notification.permission !== "granted" ||
    !notificationsEnabled()
  )
    return;
  const jobCountText =
    failedJobs.length + " job" + (failedJobs.length > 1 ? "s" : "") + " failed";
  let title;
  if (runFinished) {
    const outcome = runSucceeded(info.run) ? "succeeded" : "failed";
    title =
      failedJobs.length > 0
        ? "rotari: run " + outcome + " (" + jobCountText + ")"
        : "rotari: run " + outcome;
  } else if (failedJobs.length > 0) {
    title = "rotari: " + jobCountText;
  } else if (succeededJobs.length > 0) {
    title =
      "rotari: " +
      succeededJobs.length +
      " job" +
      (succeededJobs.length > 1 ? "s" : "") +
      " succeeded";
  } else {
    return;
  }
  const sections = [];
  for (const job of [...failedJobs, ...succeededJobs]) {
    const status = jobDisplayStatus(job, info.run);
    const lines = notificationEventLines(settings, info, job, status);
    if (lines.length) sections.push(lines.join("\n"));
  }
  if (runFinished) {
    const lines = notificationEventLines(settings, info, null, "");
    if (lines.length) sections.push(lines.join("\n"));
  }
  const body = truncateNotificationBody(sections.join("\n\n"));
  const notification = new Notification(title, {
    body,
    icon: notificationIconURL,
  });
  if ((settings.fields || []).includes("link"))
    notification.onclick = () => {
      window.focus();
      location.href =
        appURL("/project/") +
        encodeURIComponent(info.projectName) +
        "/run/" +
        encodeURIComponent(info.runID);
    };
}
// Job failures and a run's own completion that occur in the same poll are
// merged into a single notification per run.
async function checkRunNotifications(previousState, nextState, basedirID) {
  if (!previousState) return;
  const settingsByProject = new Map();
  const resolvedBasedirID =
    basedirID ||
    mountedBasedirID ||
    registeredBasedirs.find((entry) => entry.current)?.id ||
    "";
  await Promise.all(
    (nextState.projects || []).map(async (project) => {
      settingsByProject.set(
        notificationSettingsKey(project.project_name, resolvedBasedirID),
        await notificationSettings(project.project_name, resolvedBasedirID),
      );
    }),
  );
  const previousRuns = collectRunStatuses(previousState);
  const nextRuns = collectRunStatuses(nextState);
  const previousJobs = collectJobStatuses(previousState);
  const events = new Map();
  nextRuns.forEach((info, runKey) => {
    const before = previousRuns.get(runKey);
    const newlyFinished =
      !info.running && ((before && before.running) || !before);
    if (newlyFinished) {
      const settings = settingsByProject.get(info.projectName);
      const succeeded = runSucceeded(info.run);
      if (
        (succeeded && settings.run_success) ||
        (!succeeded && settings.run_failure)
      )
        events.set(runKey, {
          info,
          succeededJobs: [],
          failedJobs: [],
          runFinished: true,
          settings,
        });
    }
  });
  for (const project of nextState.projects || []) {
    for (const run of project.runs || []) {
      const runKey = project.project_name + "/" + run.run_id;
      for (const job of run.jobs || []) {
        const status = jobDisplayStatus(job, run);
        if ((status !== "failed" && status !== "success") || !job.final)
          continue;
        const jobKey = runKey + "/" + job.id;
        const before = previousJobs.get(jobKey);
        if (before?.final && before.status === status) continue;
        const settings = settingsByProject.get(
          notificationSettingsKey(project.project_name, resolvedBasedirID),
        );
        if (
          (status === "failed" && !settings.job_failure) ||
          (status === "success" && !settings.job_success)
        )
          continue;
        const event = events.get(runKey) || {
          info: nextRuns.get(runKey),
          succeededJobs: [],
          failedJobs: [],
          runFinished: false,
          settings,
        };
        const target =
          status === "failed" ? event.failedJobs : event.succeededJobs;
        if (target.length < settings.max_jobs) target.push(job);
        events.set(runKey, event);
      }
    }
  }
  events.forEach((event) =>
    notifyRunEvent(
      event.info,
      event.succeededJobs,
      event.failedJobs,
      event.runFinished,
      event.settings,
    ),
  );
}
const otherBasedirNotificationStates = new Map();
async function refreshOtherBasedirNotifications() {
  if (typeof registeredBasedirs === "undefined") return;
  const selected = selectedNotificationBasedirIDs();
  const activeID =
    mountedBasedirID || registeredBasedirs.find((entry) => entry.current)?.id;
  const targets = registeredBasedirs.filter(
    (entry) => selected?.has(entry.id) && entry.id !== activeID,
  );
  await Promise.all(
    targets.map(async (entry) => {
      try {
        const response = await fetch(basedirURL(entry.id, "/api/active-runs"), {
          cache: "no-store",
        });
        if (!response.ok) return;
        const nextState = await response.json();
        const previousState = otherBasedirNotificationStates.get(entry.id);
        if (previousState)
          await checkRunNotifications(previousState, nextState, entry.id);
        otherBasedirNotificationStates.set(entry.id, nextState);
      } catch (error) {
        // Keep polling after temporary network failures.
      }
    }),
  );
  for (const id of otherBasedirNotificationStates.keys()) {
    if (!selected?.has(id)) otherBasedirNotificationStates.delete(id);
  }
}
const originalRefresh = refresh;
refresh = async function (forceProject = true) {
  const previousState = state;
  await originalRefresh(forceProject);
  if (state !== previousState)
    await checkRunNotifications(
      previousState,
      state,
      mountedBasedirID ||
        registeredBasedirs.find((entry) => entry.current)?.id ||
        "",
    );
  await refreshOtherBasedirNotifications();
};
updateNotifyToggleLabel();
