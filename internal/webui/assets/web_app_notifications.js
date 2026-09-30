function notificationsSupported() {
  return typeof Notification !== "undefined";
}
const notificationIconURL = "__ROTARI_NOTIFICATION_ICON__";
const notificationsDefaultOn = __ROTARI_NOTIFICATION_DEFAULT__;
const initialNotificationSettings = __ROTARI_NOTIFICATION_SETTINGS__;
const notificationsEnabledKey = "rotari-notifications-enabled";
const notificationSettingsByProject = new Map();
const defaultNotificationSettings = {
  job_failure: true,
  job_success: false,
  run_failure: true,
  run_success: true,
  max_jobs: 10,
};
Object.assign(defaultNotificationSettings, initialNotificationSettings);
async function notificationSettings(projectName) {
  if (notificationSettingsByProject.has(projectName))
    return notificationSettingsByProject.get(projectName);
  try {
    const params = new URLSearchParams({ project_name: projectName });
    const response = await fetch("/api/notification-settings?" + params, {
      cache: "no-store",
    });
    if (response.ok) {
      const settings = await response.json();
      notificationSettingsByProject.set(projectName, settings);
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
  button.textContent =
    Notification.permission === "granted" && notificationsEnabled()
      ? "Notification on"
      : "Notification off";
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
        statuses.set(
          project.project_name + "/" + run.run_id + "/" + job.id,
          { status: jobDisplayStatus(job, run), final: !!job.final },
        );
      }
    }
  }
  return statuses;
}
function truncateNotificationBody(body) {
  if (body.length <= 1000) return body;
  return body.slice(0, 997) + "...";
}
function notifyRunEvent(info, succeededJobNames, failedJobNames, runFinished) {
  if (
    !notificationsSupported() ||
    Notification.permission !== "granted" ||
    !notificationsEnabled()
  )
    return;
  const jobCountText =
    failedJobNames.length +
    " job" +
    (failedJobNames.length > 1 ? "s" : "") +
    " failed";
  let title;
  if (runFinished) {
    const outcome = info.status === "failed" ? "failed" : "succeeded";
    title =
      failedJobNames.length > 0
        ? "rotari: run " + outcome + " (" + jobCountText + ")"
        : "rotari: run " + outcome;
  } else if (failedJobNames.length > 0) {
    title = "rotari: " + jobCountText;
  } else if (succeededJobNames.length > 0) {
    title =
      "rotari: " +
      succeededJobNames.length +
      " job" +
      (succeededJobNames.length > 1 ? "s" : "") +
      " succeeded";
  } else {
    return;
  }
  const body = truncateNotificationBody(
    info.projectName +
    " / " +
    info.runID +
    (runFinished
      ? "\nstatus: " + (info.status === "failed" ? "failed" : "success")
      : "") +
    (failedJobNames.length
      ? "\nfailed: " + failedJobNames.join(", ")
      : "") +
    (succeededJobNames.length
      ? "\nsucceeded: " + succeededJobNames.join(", ")
      : ""),
  );
  const notification = new Notification(title, {
    body,
    icon: notificationIconURL,
  });
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
async function checkRunNotifications(previousState, nextState) {
  if (!previousState) return;
  const settingsByProject = new Map();
  await Promise.all(
    (nextState.projects || []).map(async (project) => {
      settingsByProject.set(
        project.project_name,
        await notificationSettings(project.project_name),
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
      const runSucceeded = info.status !== "failed";
      if (
        (runSucceeded && settings.run_success) ||
        (!runSucceeded && settings.run_failure)
      )
        events.set(runKey, {
          info,
          succeededJobNames: [],
          failedJobNames: [],
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
        const settings = settingsByProject.get(project.project_name);
        if (
          (status === "failed" && !settings.job_failure) ||
          (status === "success" && !settings.job_success)
        )
          continue;
        const event = events.get(runKey) || {
          info: nextRuns.get(runKey),
          succeededJobNames: [],
          failedJobNames: [],
          runFinished: false,
          settings,
        };
        const target =
          status === "failed"
            ? event.failedJobNames
            : event.succeededJobNames;
        if (target.length < settings.max_jobs) target.push(job.name || job.id);
        events.set(runKey, event);
      }
    }
  }
  events.forEach((event) =>
    notifyRunEvent(
      event.info,
      event.succeededJobNames,
      event.failedJobNames,
      event.runFinished,
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
        if (previousState) await checkRunNotifications(previousState, nextState);
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
  if (state !== previousState) await checkRunNotifications(previousState, state);
  await refreshOtherBasedirNotifications();
};
updateNotifyToggleLabel();
