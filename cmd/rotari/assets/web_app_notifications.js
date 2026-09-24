function notificationsSupported() {
  return typeof Notification !== "undefined";
}
const notificationIconURL = "__ROTARI_NOTIFICATION_ICON__";
const notificationsDefaultOn = __ROTARI_NOTIFICATION_DEFAULT__;
const notificationsEnabledKey = "rotari-notifications-enabled";
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
  if (Notification.permission !== "granted") {
    button.textContent = "Enable notifications";
    button.disabled = Notification.permission === "denied";
    return;
  }
  button.disabled = false;
  button.textContent = notificationsEnabled()
    ? "Notifications on"
    : "Notifications off";
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
          jobDisplayStatus(job, run),
        );
      }
    }
  }
  return statuses;
}
function notifyRunEvent(info, failedJobNames, runFinished) {
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
  } else {
    return;
  }
  const body =
    info.projectName +
    " / " +
    info.runID +
    (runFinished
      ? "\nstatus: " + (info.status === "failed" ? "failed" : "success")
      : "") +
    (failedJobNames.length ? "\n" + failedJobNames.join(", ") : "");
  const notification = new Notification(title, { body, icon: notificationIconURL });
  notification.onclick = () => {
    window.focus();
    location.href =
      "/project/" +
      encodeURIComponent(info.projectName) +
      "/run/" +
      encodeURIComponent(info.runID);
  };
}
// Job failures and a run's own completion that occur in the same poll are
// merged into a single notification per run.
function checkRunNotifications(previousState, nextState) {
  if (!previousState) return;
  const previousRuns = collectRunStatuses(previousState);
  const nextRuns = collectRunStatuses(nextState);
  const previousJobs = collectJobStatuses(previousState);
  const events = new Map();
  nextRuns.forEach((info, runKey) => {
    const before = previousRuns.get(runKey);
    const newlyFinished = !info.running && ((before && before.running) || !before);
    if (newlyFinished) {
      events.set(runKey, { info, failedJobNames: [], runFinished: true });
    }
  });
  for (const project of nextState.projects || []) {
    for (const run of project.runs || []) {
      const runKey = project.project_name + "/" + run.run_id;
      for (const job of run.jobs || []) {
        if (jobDisplayStatus(job, run) !== "failed") continue;
        const jobKey = runKey + "/" + job.id;
        if (previousJobs.get(jobKey) === "failed") continue;
        const event =
          events.get(runKey) ||
          { info: nextRuns.get(runKey), failedJobNames: [], runFinished: false };
        event.failedJobNames.push(job.name || job.id);
        events.set(runKey, event);
      }
    }
  }
  events.forEach((event) =>
    notifyRunEvent(event.info, event.failedJobNames, event.runFinished),
  );
}
const originalRefresh = refresh;
refresh = async function () {
  const previousState = state;
  await originalRefresh();
  if (state !== previousState) checkRunNotifications(previousState, state);
};
updateNotifyToggleLabel();
