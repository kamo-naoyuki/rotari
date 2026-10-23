const executorNames = __ROTARI_EXECUTORS__;
let registeredBasedirs = __ROTARI_BASEDIRS__;
const mountedBasedirID = (() => {
  const parts = location.pathname.split("/").filter(Boolean);
  return parts[0] === "_basedir" ? parts[1] : "";
})();
const sidebarScrollKey =
  "rotari-sidebar-scroll:" +
  (registeredBasedirs.find((item) => item.current)?.id || mountedBasedirID);
// Status tones (ok, bad, run, warn, off) name a status's colour and icon.
// The table is statusTones in internal/webui/status.go, the Web UI's one
// status-to-colour rule; statusBase normalises a label the same way Go does.
// The Web UI writes dates in English whatever the browser's language, as
// all its other text is English; every client-side date goes through these.
const uiLocale = "en-US";
/** A local date and time as the server writes them, "2026-10-11 05:52:22". */
function formatDateTime(value) {
  const date = new Date(value);
  if (Number.isNaN(+date)) return "";
  const two = (number) => String(number).padStart(2, "0");
  return (
    date.getFullYear() +
    "-" +
    two(date.getMonth() + 1) +
    "-" +
    two(date.getDate()) +
    " " +
    formatClock(date)
  );
}
/** A local time of day, "05:52:22". */
function formatClock(value) {
  const date = new Date(value);
  const two = (number) => String(number).padStart(2, "0");
  return (
    two(date.getHours()) +
    ":" +
    two(date.getMinutes()) +
    ":" +
    two(date.getSeconds())
  );
}
/** A local date with English names, such as "Sun, Oct 11" or "October 2026". */
function formatDate(value, options) {
  return new Date(value).toLocaleDateString(uiLocale, options);
}
const statusTones = __ROTARI_STATUS_TONES__;
function statusBase(status) {
  let base = String(status).trim().toLowerCase();
  if (base.endsWith("...")) base = base.slice(0, -3).trim();
  const open = base.indexOf(" (");
  if (open >= 0 && base.endsWith(")")) base = base.slice(0, open);
  return base;
}
function statusTone(status) {
  return statusTones[statusBase(status)] || "off";
}
function statusColor(status) {
  return "var(--s-" + statusTone(status) + ")";
}
// A status pill: the label in its tone's colour, with its icon; a carried
// result keeps its tone and gets a dashed outline.
function statusPillClass(status) {
  return (
    "status-pill tone-" +
    statusTone(status) +
    (String(status).toLowerCase().includes("(carried)") ? " carried" : "")
  );
}
function statusPill(status, text = status) {
  return (
    '<span class="' + statusPillClass(status) + '">' + esc(text) + "</span>"
  );
}
// Row actions. A table renderer emits its actions cell itself, last in the
// row (first only in the queue table, beside its editors); its buttons carry
// their arguments in data- attributes and call one of these handlers.
function actionsCell(buttons) {
  return (
    '<td class="actions"><div class="action-buttons">' +
    buttons.filter(Boolean).join(" ") +
    "</div></td>"
  );
}
function showJobReport(button) {
  const project = state.projects.find(
    (item) => item.project_name === button.dataset.project,
  );
  const run =
    project && project.runs.find((item) => item.run_id === button.dataset.run);
  const job =
    run && (run.jobs || []).find((item) => item.id === button.dataset.job);
  if (job) showAIReport(project, run, job);
}
function pathButton(path) {
  return (
    '<button class="show-path" data-path="' +
    esc(path) +
    '" onclick="showPath(this.dataset.path)">Path</button>'
  );
}
function deleteRunButton(projectName, runID) {
  return (
    '<button class="delete-run-row" data-project="' +
    esc(projectName) +
    '" data-run="' +
    esc(runID) +
    '" onclick="deleteRun(this.dataset.project, this.dataset.run)">Delete</button>'
  );
}
function deleteProjectButton(projectName, running) {
  return (
    '<button class="delete-project-row" data-project="' +
    esc(projectName) +
    '"' +
    (running
      ? ' disabled title="Running projects cannot be deleted"'
      : ' title="Delete the project: its queue, runs, and project configuration"') +
    ' onclick="deleteProject(this.dataset.project)">Delete</button>'
  );
}
function basedirURL(id, path) {
  const entry = registeredBasedirs.find((item) => item.id === id);
  const prefix = entry?.current ? "" : "/_basedir/" + id;
  return prefix + (path.startsWith("/") ? path : "/" + path);
}
function appURL(path) {
  if (!path.startsWith("/")) path = "/" + path;
  return mountedBasedirID && !path.startsWith("/_basedir/")
    ? "/_basedir/" + mountedBasedirID + path
    : path;
}
const nativeFetch = window.fetch.bind(window);
window.fetch = function (input, options) {
  if (typeof input === "string" && input.startsWith("/")) input = appURL(input);
  return nativeFetch(input, options);
};
let state;
let projectRuntimeDetailsOpen = false;
let stateJSON = "";
const projectStateCache = new Map();
const runDetailCache = new Map();
// A run section's open state chosen on this page (runSectionOpen).
/** @type {Record<string, boolean>} */
const expandedRunGraphics = {};
let selectedOutput = "";
let selectedLog = null;
let selectedReportContext = null;
let followTimer = null;
let sidebarScrollActive = false;
let sidebarRenderDeferred = false;
let sidebarScrollTimer = 0;
const selectedAttemptByJob = {};
let sortState = {
  queue: { key: "name", direction: 1 },
  run: { key: "started", direction: -1 },
  job: { key: "name", direction: 1 },
  queueJobs: { key: "name", direction: 1 },
};
function pageParts() {
  const parts =
    typeof routeParts === "function"
      ? routeParts()
      : location.pathname.split("/").filter(Boolean);
  if (parts[0] === "_basedir") parts.splice(0, 2);
  return parts;
}
async function fetchWebJSON(path) {
  try {
    const response = await fetch(path);
    return response.ok ? await response.json() : null;
  } catch (error) {
    // Secondary detail routes may be unavailable in static exports or during a transient disconnect.
    return null;
  }
}
function pruneProjectRunCache(projectName, project) {
  const knownRuns = new Set((project.runs || []).map((run) => run.run_id));
  const prefix = projectName + "/";
  for (const [key, detail] of runDetailCache) {
    if (key.startsWith(prefix) && !knownRuns.has(key.slice(prefix.length)))
      runDetailCache.delete(key);
  }
}
function projectNeedsRefresh(index, projectName, force) {
  if (!projectName || typeof rewriteStaticLinks === "function") return false;
  const indexed = (index.projects || []).find(
    (project) => project.project_name === projectName,
  );
  const cached = projectStateCache.get(projectName);
  const changed = !cached || cached.revision !== indexed?.revision;
  return force || changed;
}
async function refreshProjectOverview(index, projectName, force) {
  if (!projectNeedsRefresh(index, projectName, force)) return;
  const project = await fetchWebJSON(
    "/api/project?project_name=" + encodeURIComponent(projectName),
  );
  if (!project) return;
  project._loaded = true;
  projectStateCache.set(projectName, project);
  pruneProjectRunCache(projectName, project);
}
async function refreshActiveRunDetails(index) {
  const activeRunKeys = new Set();
  for (const project of index.projects || []) {
    for (const run of project.runs || []) {
      const key = project.project_name + "/" + run.run_id;
      if (run.running) activeRunKeys.add(key);
      if (run.running || Array.isArray(run.jobs)) runDetailCache.set(key, run);
    }
  }
  const finishedRuns = [...runDetailCache].filter(
    ([key, run]) => run.running && !activeRunKeys.has(key),
  );
  await Promise.all(
    finishedRuns.map(async ([key]) => {
      const [projectName, runID] = key.split("/");
      const detail = await fetchWebJSON(
        "/api/run?project_name=" +
          encodeURIComponent(projectName) +
          "&run_id=" +
          encodeURIComponent(runID),
      );
      if (detail) runDetailCache.set(key, detail);
    }),
  );
  return activeRunKeys;
}
async function refreshSelectedRun(index, projectName, runID, activeRunKeys) {
  if (!runID) return;
  const key = projectName + "/" + runID;
  const indexed = (index.projects || []).find(
    (project) => project.project_name === projectName,
  );
  const isRunning = indexed?.running_run_id === runID;
  const cached = runDetailCache.get(key);
  if (
    !cached ||
    (isRunning && !activeRunKeys.has(key)) ||
    (cached.running && !isRunning) ||
    ["interrupted", "incomplete"].includes(cached.lifecycle || cached.status)
  ) {
    const detail = await fetchWebJSON(
      "/api/run?project_name=" +
        encodeURIComponent(projectName) +
        "&run_id=" +
        encodeURIComponent(runID),
    );
    if (detail) runDetailCache.set(key, detail);
  }
}
function mergeWebIndex(index) {
  return {
    ...index,
    projects: (index.projects || []).map((indexed) => {
      const loaded = projectStateCache.get(indexed.project_name);
      const project = loaded
        ? { ...indexed, ...loaded, _loaded: true }
        : { ...indexed, _loaded: typeof rewriteStaticLinks === "function" };
      project.running_run_id = indexed.running_run_id;
      project.runner_pid = indexed.runner_pid;
      project.runner_host = indexed.runner_host;
      project.runner_started_at = indexed.runner_started_at;
      const runs = new Map(
        (project.runs || []).map((run) => [run.run_id, run]),
      );
      for (const run of indexed.runs || [])
        runs.set(run.run_id, { ...runs.get(run.run_id), ...run });
      for (const [key, detail] of runDetailCache) {
        const prefix = indexed.project_name + "/";
        if (key.startsWith(prefix)) {
          const runID = key.slice(prefix.length);
          runs.set(runID, { ...runs.get(runID), ...detail });
        }
      }
      project.runs = [...runs.values()].sort((a, b) =>
        b.run_id.localeCompare(a.run_id),
      );
      return project;
    }),
  };
}
function renderRefreshedState(nextState, projectName, runID) {
  document.getElementById("disconnect-banner").classList.remove("show");
  const nextStateJSON = JSON.stringify(nextState);
  if (nextStateJSON === stateJSON) return;
  stateJSON = nextStateJSON;
  state = nextState;
  const selectedRunKey = runID ? projectName + "/" + runID : "";
  for (const [key, detail] of runDetailCache) {
    if (!detail.running && key !== selectedRunKey) runDetailCache.delete(key);
  }
  if (sidebarScrollActive) {
    sidebarRenderDeferred = true;
    return;
  }
  render();
  if (typeof rewriteStaticLinks === "function") rewriteStaticLinks();
}
async function refresh(forceProject = true) {
  if (
    document.activeElement?.closest(
      ".web-queue-commands input,.web-queue-commands select",
    )
  )
    return;
  try {
    const response = await fetch("/api/state");
    if (!response.ok) {
      document.getElementById("app").textContent = await response.text();
      return;
    }
    const index = await response.json();
    const parts = pageParts();
    const projectName =
      parts[0] === "project" ? decodeURIComponent(parts[1]) : "";
    const runID = parts[2] === "run" ? decodeURIComponent(parts[3]) : "";
    const activeRunKeys = await refreshActiveRunDetails(index);
    await refreshProjectOverview(index, projectName, forceProject);
    await refreshSelectedRun(index, projectName, runID, activeRunKeys);
    renderRefreshedState(mergeWebIndex(index), projectName, runID);
  } catch (error) {
    // The primary index request failed; keep the existing view and show its disconnected state.
    document.getElementById("disconnect-banner").classList.add("show");
  }
}
// The Refresh button: what the 2-second poll reads, plus the registered
// basedirs and the other basedirs' project lists.
async function refreshWithBasedirs() {
  if (typeof rewriteStaticLinks !== "function") {
    const data = await fetchWebJSON("/api/basedirs");
    if (data) {
      registeredBasedirs = data.basedirs;
      for (const id of Object.keys(remoteProjectsByBasedir))
        delete remoteProjectsByBasedir[id];
      stateJSON = "";
    }
  }
  await refresh();
}
function render() {
  const queues = state.projects || [];
  renderSidebar(queues);
  const parts = pageParts();
  if (parts[0] === "search") {
    renderHistorySearchPage();
    return;
  }
  if (parts[0] === "activity") {
    renderActivityPage();
    return;
  }
  if (parts[0] !== "project") {
    renderOverview(queues);
    return;
  }
  const queue = queues.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  if (!queue) {
    renderMissing("Project not found");
    return;
  }
  if (parts[2] === "run") {
    renderRun(queue, decodeURIComponent(parts[3]));
    return;
  }
  renderQueue(queue);
}
const expandedSidebarProjects = {};
const expandedSidebarBasedirs = {};
const expandedSidebarAllProjects = {};
const remoteProjectsByBasedir = {};
const notificationBasedirsKey =
  "rotari-notification-basedirs:" + "__ROTARI_NOTIFICATION_SESSION__";
let selectedNotificationBasedirIDsState;
function selectedNotificationBasedirIDs() {
  if (selectedNotificationBasedirIDsState)
    return selectedNotificationBasedirIDsState;
  try {
    const stored = localStorage.getItem(notificationBasedirsKey);
    selectedNotificationBasedirIDsState =
      stored === null ? null : new Set(JSON.parse(stored));
  } catch (error) {
    selectedNotificationBasedirIDsState = null;
  }
  return selectedNotificationBasedirIDsState;
}
function restoreNotificationBasedirs() {
  const selected = selectedNotificationBasedirIDs();
  document
    .querySelectorAll(".basedir-notification-toggle")
    .forEach((checkbox) => {
      checkbox.checked =
        selected === null
          ? checkbox.closest(".basedir-entry").classList.contains("expanded")
          : selected.has(checkbox.dataset.basedirId);
      updateNotificationTooltip(checkbox);
    });
}
function updateNotificationTooltip(checkbox) {
  const text = checkbox.checked
    ? "Disable notifications"
    : "Enable notifications";
  const bell = checkbox.closest(".basedir-bell");
  if (bell) bell.title = text;
}
function toggleNotificationBasedir(checkbox) {
  const selected =
    selectedNotificationBasedirIDs() ||
    new Set(
      [...document.querySelectorAll(".basedir-notification-toggle")]
        .filter((item) => item.checked)
        .map((item) => item.dataset.basedirId),
    );
  if (checkbox.checked) selected.add(checkbox.dataset.basedirId);
  else selected.delete(checkbox.dataset.basedirId);
  selectedNotificationBasedirIDsState = selected;
  localStorage.setItem(notificationBasedirsKey, JSON.stringify([...selected]));
  updateNotificationTooltip(checkbox);
  if (typeof refreshOtherBasedirNotifications === "function")
    refreshOtherBasedirNotifications();
}
function sidebarRunLinksHTML(q, isActive, activeRun, basedirID) {
  const runs = (q.runs || [])
    .slice()
    .sort((a, b) => b.run_id.localeCompare(a.run_id));
  const runLinks = runs
    .map(
      (r) =>
        '<a class="sidebar-run' +
        (isActive && r.run_id === activeRun ? " active" : "") +
        '" href="' +
        basedirURL(
          basedirID,
          "/project/" +
            encodeURIComponent(q.project_name) +
            "/run/" +
            encodeURIComponent(r.run_id),
        ) +
        '"><span class="sidebar-run-dot tone-' +
        statusTone(r.lifecycle || r.status || "") +
        '" aria-hidden="true"></span>' +
        esc(r.run_name || r.run_id) +
        "</a>",
    )
    .join("");
  return runLinks || '<span class="sidebar-run">No runs</span>';
}
// basedirLabelHTML labels a basedir by its parent directory, in bold, and
// its own name, from the label newWebBaseDir (assets.go) computes; it is the
// same markup as Go's basedirLabelHTML.
// basedirLabelText is basedirLabelHTML as plain text, for a select option.
function basedirLabelText(entry) {
  const name = entry.name || entry.path;
  return entry.parent ? entry.parent + "/" + name : name;
}
function basedirLabelHTML(entry) {
  const name = esc(entry.name || entry.path);
  return entry.parent
    ? '<span class="basedir-parent">' +
        esc(entry.parent) +
        '</span><span class="basedir-name">/' +
        name +
        "</span>"
    : '<span class="basedir-name">' + name + "</span>";
}
const bellIconSVG =
  '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 16V11a6 6 0 1 1 12 0v5l1.5 2h-15z"></path><path d="M10 20a2 2 0 0 0 4 0"></path></svg>';
function sidebarProjectLink(name, basedirID, active) {
  return (
    '<div class="sidebar-project-row"><span class="sidebar-toggle-placeholder" aria-hidden="true"></span><a class="sidebar-project-link basedir-switch' +
    (active ? " active" : "") +
    '" href="' +
    basedirURL(basedirID, "/project/" + encodeURIComponent(name)) +
    '">' +
    esc(name) +
    "</a></div>"
  );
}
function activeSidebarProjectsHTML(entry, queues, activeProject, activeRun) {
  return queues
    .map((project) => {
      const projectName = project.project_name;
      const key = entry.id + "/" + projectName;
      const projectActive = projectName === activeProject;
      if (!Object.hasOwn(expandedSidebarProjects, key))
        expandedSidebarProjects[key] = projectActive;
      const expanded = !!expandedSidebarProjects[key];
      let currentRun = "";
      if (projectActive) currentRun = activeRun;
      const runsHTML = expanded
        ? sidebarRunLinksHTML(project, projectActive, currentRun, entry.id)
        : "";
      return (
        '<div class="sidebar-project' +
        (expanded ? " expanded" : "") +
        '" data-basedir-id="' +
        entry.id +
        '" data-project-name="' +
        esc(projectName) +
        '"><div class="sidebar-project-row"><button type="button" class="sidebar-toggle" aria-expanded="' +
        (expanded ? "true" : "false") +
        '" aria-label="Toggle runs" onclick="toggleSidebarProject(this)"></button><a class="sidebar-project-link' +
        (projectActive ? " active" : "") +
        ' basedir-switch" href="' +
        basedirURL(entry.id, "/project/" + encodeURIComponent(projectName)) +
        '">' +
        esc(projectName) +
        '</a></div><div class="sidebar-runs"' +
        (expanded ? "" : " hidden") +
        ">" +
        runsHTML +
        "</div></div>"
      );
    })
    .join("");
}
function sidebarBasedirHTML(
  entry,
  activeID,
  projects,
  activeProject,
  activeRun,
) {
  const isActive = entry.id === activeID;
  const page = pageParts()[0];
  const isJobsPage = page === "jobs";
  const isOverview =
    !activeProject &&
    page !== "jobs" &&
    page !== "search" &&
    page !== "activity";
  if (!Object.hasOwn(expandedSidebarBasedirs, entry.id))
    expandedSidebarBasedirs[entry.id] = isActive;
  const isExpanded = !!expandedSidebarBasedirs[entry.id];
  let projectLinks = "";
  if (isExpanded && isActive) {
    projectLinks = activeSidebarProjectsHTML(
      entry,
      projects,
      activeProject,
      activeRun,
    );
  } else if (isExpanded) {
    projectLinks = (remoteProjectsByBasedir[entry.id] || [])
      .map((name) => sidebarProjectLink(name, entry.id, false))
      .join("");
  }
  if (!Object.hasOwn(expandedSidebarAllProjects, entry.id))
    expandedSidebarAllProjects[entry.id] = true;
  const allProjectsExpanded = !!expandedSidebarAllProjects[entry.id];
  return (
    '<div class="sidebar-project basedir-entry' +
    (isExpanded ? " expanded" : "") +
    '" data-basedir-id="' +
    entry.id +
    '"><div class="sidebar-project-row basedir-row"><button type="button" class="sidebar-toggle" aria-expanded="' +
    (isExpanded ? "true" : "false") +
    '" aria-label="Toggle projects" onclick="toggleSidebarBasedir(this)"></button><a class="sidebar-project-link' +
    (isActive ? " active" : "") +
    ' basedir-path basedir-switch" title="' +
    esc(entry.path) +
    '" href="' +
    basedirURL(entry.id, "/") +
    '">' +
    basedirLabelHTML(entry) +
    '</a><label class="basedir-bell" title="Enable notifications"><input class="basedir-notification-toggle" type="checkbox" data-basedir-id="' +
    entry.id +
    '" aria-label="Notify about runs in this basedir" onchange="toggleNotificationBasedir(this)" />' +
    bellIconSVG +
    '</label></div><div class="sidebar-projects basedir-contents"' +
    (isExpanded ? ">" : " hidden>") +
    '<div class="sidebar-project all-projects' +
    (allProjectsExpanded ? " expanded" : "") +
    '"><div class="sidebar-project-row"><button type="button" class="sidebar-toggle" aria-expanded="' +
    (allProjectsExpanded ? "true" : "false") +
    '" aria-label="Toggle project list" onclick="toggleSidebarAllProjects(this)"></button><a class="sidebar-project-link basedir-switch' +
    (isActive && isOverview ? " active" : "") +
    '" href="' +
    basedirURL(entry.id, "/") +
    '">All projects</a></div><div class="sidebar-projects project-list"' +
    (allProjectsExpanded ? "" : " hidden") +
    ">" +
    projectLinks +
    '</div></div><a class="sidebar-run' +
    (isActive && isJobsPage ? " active" : "") +
    ' basedir-switch" href="' +
    basedirURL(entry.id, "/jobs/") +
    '">Job activity</a></div></div>'
  );
}
function renderSidebar(queues) {
  const container = document.getElementById("sidebar-basedirs");
  if (!container) return;
  const sidebar = container.closest(".sidebar");
  const scrollTop = sidebar.scrollTop;
  const parts = pageParts();
  sidebar
    .querySelector(".sidebar-activity-link")
    ?.classList.toggle("active", parts[0] === "activity");
  const searchLink = sidebar.querySelector(".sidebar-search-link");
  if (searchLink) {
    searchLink.classList.toggle("active", parts[0] === "search");
    // History search runs on the server; a static export has none.
    searchLink.hidden = typeof rewriteStaticLinks === "function";
  }
  const activeProject =
    parts[0] === "project" ? decodeURIComponent(parts[1]) : "";
  const activeRun = parts[2] === "run" ? decodeURIComponent(parts[3]) : "";
  const activeBaseID =
    mountedBasedirID ||
    registeredBasedirs.find((item) => item.current)?.id ||
    "";
  container.innerHTML = registeredBasedirs
    .map((entry) =>
      sidebarBasedirHTML(
        entry,
        activeBaseID,
        queues || [],
        activeProject,
        activeRun,
      ),
    )
    .join("");
  restoreNotificationBasedirs();
  if (!sidebar.dataset.scrollStored) {
    sidebar.dataset.scrollStored = "true";
    sidebar.addEventListener(
      "scroll",
      () => {
        sidebarScrollActive = true;
        sessionStorage.setItem(sidebarScrollKey, String(sidebar.scrollTop));
        clearTimeout(sidebarScrollTimer);
        sidebarScrollTimer = setTimeout(() => {
          sidebarScrollActive = false;
          if (sidebarRenderDeferred) {
            sidebarRenderDeferred = false;
            render();
            if (typeof rewriteStaticLinks === "function") rewriteStaticLinks();
          }
        }, 180);
      },
      { passive: true },
    );
  }
  const savedLocationKey = sidebarScrollKey + ":location";
  const previousLocation = sessionStorage.getItem(savedLocationKey);
  const locationChanged = previousLocation !== location.pathname;
  sessionStorage.setItem(savedLocationKey, location.pathname);
  const storedScroll = Number(sessionStorage.getItem(sidebarScrollKey));
  sidebar.scrollTop = Number.isFinite(storedScroll) ? storedScroll : scrollTop;
  const activeItem =
    sidebar.querySelector(".sidebar-run.active") ||
    sidebar.querySelector(
      ".sidebar-project:not(.basedir-entry) .sidebar-project-link.active",
    ) ||
    sidebar.querySelector(".sidebar-project-link.active");
  if (activeItem && locationChanged) {
    const sidebarBounds = sidebar.getBoundingClientRect();
    const itemBounds = activeItem.getBoundingClientRect();
    if (itemBounds.top < sidebarBounds.top)
      sidebar.scrollTop -= sidebarBounds.top - itemBounds.top;
    else if (itemBounds.bottom > sidebarBounds.bottom)
      sidebar.scrollTop += itemBounds.bottom - sidebarBounds.bottom;
  }
  registeredBasedirs.forEach((entry) => {
    if (
      entry.id !== activeBaseID &&
      expandedSidebarBasedirs[entry.id] &&
      !remoteProjectsByBasedir[entry.id]
    )
      loadBasedirProjects(entry.id);
  });
}
async function loadBasedirProjects(id) {
  try {
    const response = await fetch(
      "/api/projects?basedir_id=" + encodeURIComponent(id),
    );
    if (!response.ok) return;
    const data = await response.json();
    remoteProjectsByBasedir[id] = data.projects || [];
    renderSidebar(state ? state.projects || [] : []);
  } catch (error) {
    // Leave the expanded basedir without projects when its directory is unavailable.
  }
}
function toggleSidebarBasedir(button) {
  const base = button.closest(".basedir-entry");
  const id = base.dataset.basedirId;
  expandedSidebarBasedirs[id] = button.getAttribute("aria-expanded") !== "true";
  renderSidebar(state ? state.projects || [] : []);
}
function toggleSidebarProject(button) {
  const project = button.closest(".sidebar-project");
  const key = project.dataset.basedirId + "/" + project.dataset.projectName;
  const expanded = button.getAttribute("aria-expanded") !== "true";
  expandedSidebarProjects[key] = expanded;
  renderSidebar(state ? state.projects || [] : []);
  const activeID =
    mountedBasedirID || registeredBasedirs.find((item) => item.current)?.id;
  if (
    expanded &&
    project.dataset.basedirId === activeID &&
    typeof rewriteStaticLinks !== "function"
  )
    void loadSidebarProject(project.dataset.projectName);
}
async function loadSidebarProject(projectName) {
  if (!state || !projectNeedsRefresh(state, projectName, false)) return;
  const project = await fetchWebJSON(
    "/api/project?project_name=" + encodeURIComponent(projectName),
  );
  if (!project) return;
  project._loaded = true;
  projectStateCache.set(projectName, project);
  pruneProjectRunCache(projectName, project);
  state.projects = mergeWebIndex(state).projects;
  renderSidebar(state.projects);
}
function toggleSidebarAllProjects(button) {
  const basedir = button.closest(".basedir-entry");
  const id = basedir.dataset.basedirId;
  expandedSidebarAllProjects[id] =
    button.getAttribute("aria-expanded") !== "true";
  renderSidebar(state ? state.projects || [] : []);
}
function applyBasedirLinks() {
  if (!mountedBasedirID) return;
  const prefix = "/_basedir/" + mountedBasedirID;
  document.querySelectorAll('a[href^="/"]').forEach((link) => {
    const href = link.getAttribute("href");
    if (
      link.classList.contains("basedir-switch") ||
      href.startsWith("/_basedir/") ||
      href.startsWith("/_launch/") ||
      href.startsWith(prefix + "/")
    )
      return;
    link.setAttribute("href", prefix + href);
  });
}
function webStatePath(...segments) {
  const separator = String(segments[0] || "").includes("\\") ? "\\" : "/";
  let path = String(segments.shift() || "").replace(/[\\/]+$/, "");
  if (!path) path = separator;
  for (const segment of segments) {
    const part = String(segment).replace(/^[\\/]+|[\\/]+$/g, "");
    if (!part) continue;
    path = path.endsWith(separator) ? path + part : path + separator + part;
  }
  return path;
}
// setCrumbs fills the top bar: the page's basedir, with a button that copies
// copyPath (the page's state path), then trail, [label, href] pairs whose
// last is the page itself. It also names the window after the page.
function setCrumbs(trail, copyPath) {
  const entry = registeredBasedirs.find((item) =>
    mountedBasedirID ? item.id === mountedBasedirID : item.current,
  );
  const basePath = entry ? entry.path : (state && state.base_dir) || "";
  const parts = [];
  if (basePath)
    parts.push(
      '<a class="crumb-basedir" href="/" title="' +
        esc(basePath) +
        '">' +
        (entry ? basedirLabelHTML(entry) : esc(basePath)) +
        "</a>" +
        (copyPath ? copyIconForValue(copyPath, "state path") : ""),
    );
  trail.forEach(([label, href], index) =>
    parts.push(
      index === trail.length - 1 || !href
        ? '<span class="crumb-here">' + esc(label) + "</span>"
        : '<a href="' + href + '">' + esc(label) + "</a>",
    ),
  );
  document.getElementById("crumbs").innerHTML = parts.join(
    '<span class="crumb-sep">/</span>',
  );
  document.title = trail[trail.length - 1][0] + " · rotari";
}
function setModalConfigPaths(paths) {
  const container = document.getElementById("modal-config-paths");
  container.replaceChildren();
  for (const path of paths || []) {
    const item = document.createElement("div");
    item.className = "modal-config-path";
    const value = document.createElement("span");
    value.textContent = path;
    item.append(
      value,
      document
        .createRange()
        .createContextualFragment(copyIconForValue(path, "config path")),
    );
    container.append(item);
  }
  container.hidden = !paths?.length;
}
function pageConfigPaths() {
  const parts = pageParts();
  if (parts[0] !== "project")
    return (
      state.config_sources?.map((source) => source.path) ||
      (state.config_path ? [state.config_path] : [])
    );
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  if (!project) return [];
  return (
    project.config_sources?.map((source) => source.path) ||
    (project.config_path ? [project.config_path] : [])
  );
}
async function showConfig(snapshot = false) {
  const modal = document.getElementById("output-modal");
  const generator = document.getElementById("config-generator");
  const editor = document.getElementById("config-editor");
  const notificationEditor = document.getElementById(
    "notification-config-editor",
  );
  const output = ensureModalOutput();
  document.getElementById("config-source-select")?.remove();
  setModalConfigPaths([]);
  generator.hidden = true;
  editor.hidden = true;
  notificationEditor.hidden = true;
  output.hidden = false;
  const parts = pageParts();
  const params = new URLSearchParams();
  if (parts[0] === "project")
    params.set("project_name", decodeURIComponent(parts[1]));
  if (snapshot) params.set("run_id", decodeURIComponent(parts[3]));
  const response = await fetch("/api/config?" + params);
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  const payload = JSON.parse(text);
  const files = payload.configs || [];
  setModalConfigPaths(files.map((file) => file.path));
  const content = files
    .map((item) => "# " + item.path + "\n" + item.content)
    .join("\n\n");
  selectedLog = null;
  selectedOutput = content;
  const project = configGenerationProject();
  const displayFile = (file) => {
    if (!file) return;
    setModalConfigPaths([file.path]);
    selectedOutput = file.content;
    output.hidden = true;
    editor.hidden = false;
    const textarea = editor.querySelector("textarea");
    const saveButton = editor.querySelector("button");
    textarea.value = file.content;
    textarea.dataset.initial = file.content;
    textarea.oninput = () => updateConfigSaveState(editor);
    updateConfigSaveState(editor);
    saveButton.onclick = async () => {
      saveButton.disabled = true;
      const response = await fetch("/api/save-config", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          project_name: project,
          scope: file.scope,
          path: file.path,
          content: textarea.value,
        }),
      });
      const saveText = await response.text();
      saveButton.disabled = false;
      if (!response.ok) {
        alert(saveText);
        return;
      }
      file.content = textarea.value;
      textarea.dataset.initial = textarea.value;
      updateConfigSaveState(editor);
      selectedOutput = textarea.value;
      await refresh();
      alert("Saved " + JSON.parse(saveText).path);
    };
    modal.dataset.editing = "true";
  };
  if (snapshot) {
    delete modal.dataset.editing;
    output.textContent = content;
  } else {
    if (!files.length) {
      alert("No config file exists to edit.");
      return;
    }
    if (files.length > 1) {
      const chooser = document.createElement("select");
      chooser.id = "config-source-select";
      chooser.setAttribute("aria-label", "Configuration source");
      files.forEach((file, index) => {
        const option = document.createElement("option");
        option.value = index;
        option.textContent = (file.scope || "config") + ": " + file.path;
        chooser.appendChild(option);
      });
      chooser.onchange = () => displayFile(files[Number(chooser.value)]);
      editor.parentNode.insertBefore(chooser, editor);
    }
    displayFile(files[0]);
  }
  modal.querySelector("strong").textContent = snapshot
    ? "Run config snapshot"
    : "Config";
  modal.dataset.view = snapshot ? "config-snapshot" : "config";
  openOutputModal(false);
}
function updateConfigSaveState(editor) {
  const textarea = editor.querySelector("textarea");
  const changed = textarea.value !== textarea.dataset.initial;
  textarea.classList.toggle("dirty", changed);
  editor.querySelector("button").disabled = !changed;
}
function configGenerationProject() {
  const parts = pageParts();
  return parts[0] === "project" ? decodeURIComponent(parts[1]) : "";
}
function notificationConfigProject() {
  return configGenerationProject();
}
function showGenerateConfig() {
  const project = configGenerationProject();
  const modal = document.getElementById("output-modal");
  const output = ensureModalOutput();
  const generator = document.getElementById("config-generator");
  const editor = document.getElementById("config-editor");
  const notificationEditor = document.getElementById(
    "notification-config-editor",
  );
  output.hidden = true;
  editor.hidden = true;
  notificationEditor.hidden = true;
  generator.hidden = false;
  delete modal.dataset.editing;
  generator.replaceChildren();
  generator.textContent = "Loading config locations...";
  const params = new URLSearchParams();
  if (project) params.set("project_name", project);
  fetch("/api/config-targets?" + params)
    .then(async (response) => {
      const text = await response.text();
      if (!response.ok) throw new Error(text);
      return JSON.parse(text);
    })
    .then((payload) => {
      renderConfigTargetOptions(
        generator,
        payload.targets,
        "config.toml",
        async (target, button) => {
          if (
            !confirm(
              "Generate config.toml at " +
                target.path +
                "? Existing contents of that file will be replaced.",
            )
          )
            return;
          button.disabled = true;
          const response = await fetch("/api/generate-config", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({
              project_name: project,
              location: target.location,
            }),
          });
          const text = await response.text();
          button.disabled = false;
          if (!response.ok) {
            alert(text);
            return;
          }
          const result = JSON.parse(text);
          closeOutputModal();
          await refresh();
          alert("Generated " + result.path);
        },
      );
    })
    .catch((error) => {
      generator.textContent =
        "Failed to load config locations: " + error.message;
    });
  modal.querySelector("strong").textContent = "Generate config";
  modal.dataset.view = "generate-config";
  openOutputModal(true);
}
function notificationFieldControl(channel, field, selected) {
  const label = document.createElement("label");
  const input = document.createElement("input");
  input.type = "checkbox";
  input.name = channel + "-field";
  input.value = field;
  input.checked = selected.includes(field);
  if (channel === "webhook" && field === "link") input.disabled = true;
  label.append(input, field.replaceAll("_", " "));
  return label;
}
function notificationChannelEditor(name, settings, fields, maxJobsContainer) {
  const fieldset = document.createElement("fieldset");
  const legend = document.createElement("legend");
  const isWebhook = name === "webhook";
  legend.textContent = isWebhook
    ? "External notifications (webhook)"
    : "Desktop notifications (this browser)";
  fieldset.append(legend);
  const description = document.createElement("p");
  description.className = "notification-channel-description";
  description.textContent = isWebhook
    ? "Deliver selected events to a webhook URL, for example Slack or Discord."
    : "Display selected events as desktop notifications from this browser.";
  fieldset.append(description);
  const eventGroup = document.createElement("div");
  eventGroup.className = "notification-setting-group";
  const eventHeading = document.createElement("h3");
  eventHeading.textContent = "When to notify";
  eventGroup.append(eventHeading);
  for (const [key, labelText] of [
    ["job_failure", "Job failure"],
    ["job_success", "Job success"],
    ["run_failure", "Run failure"],
    ["run_success", "Run success"],
  ]) {
    const label = document.createElement("label");
    const input = document.createElement("input");
    input.type = "checkbox";
    input.name = name + "-" + key;
    input.checked = !!settings[key];
    label.append(input, labelText);
    eventGroup.append(label);
  }
  fieldset.append(eventGroup);
  const contentGroup = document.createElement("div");
  contentGroup.className = "notification-setting-group";
  const contentHeading = document.createElement("h3");
  contentHeading.textContent = "Information to send";
  contentGroup.append(contentHeading);
  const fieldGroup = document.createElement("div");
  fieldGroup.className = "notification-field-grid";
  fields.forEach((field) =>
    fieldGroup.append(
      notificationFieldControl(name, field, settings.fields || []),
    ),
  );
  contentGroup.append(fieldGroup);
  const maxLabel = document.createElement("label");
  maxLabel.className = "notification-max-jobs";
  const maxText = document.createElement("span");
  maxText.textContent = "Maximum jobs";
  const maxInput = document.createElement("input");
  maxInput.type = "number";
  maxInput.name = name + "-max-jobs";
  maxInput.min = "1";
  maxInput.value = settings.max_jobs;
  maxLabel.append(maxText, maxInput);
  if (maxJobsContainer) maxJobsContainer.prepend(maxLabel);
  else contentGroup.append(maxLabel);
  fieldset.append(contentGroup);
  return fieldset;
}
function readNotificationChannel(form, name) {
  const checked = (suffix) => form.elements[name + "-" + suffix].checked;
  return {
    job_failure: checked("job_failure"),
    job_success: checked("job_success"),
    run_failure: checked("run_failure"),
    run_success: checked("run_success"),
    max_jobs: Number(form.elements[name + "-max-jobs"].value),
    fields: [
      ...form.querySelectorAll(`input[name="${name}-field"]:checked`),
    ].map((input) => input.value),
  };
}
function updateNotificationConfigDirty(form) {
  const dirty = [...form.querySelectorAll("input,select")].some((control) => {
    const value =
      control.type === "checkbox" ? String(control.checked) : control.value;
    return value !== control.dataset.initialValue;
  });
  form.classList.toggle("dirty", dirty);
  form
    .querySelectorAll("fieldset")
    .forEach((fieldset) => fieldset.classList.toggle("dirty", dirty));
}
function initializeNotificationConfigDirtyState(form) {
  form.querySelectorAll("input,select").forEach((control) => {
    control.dataset.initialValue =
      control.type === "checkbox" ? String(control.checked) : control.value;
  });
  form.oninput = () => updateNotificationConfigDirty(form);
  form.onchange = () => updateNotificationConfigDirty(form);
  updateNotificationConfigDirty(form);
}
async function generateNotificationConfig(project, target, button) {
  if (
    !confirm(
      "Generate notifications.toml at " +
        target.path +
        "? Existing contents of that file will be replaced.",
    )
  )
    return;
  button.disabled = true;
  const response = await fetch("/api/generate-notification-config", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      project_name: project,
      location: target.location,
    }),
  });
  const text = await response.text();
  button.disabled = false;
  if (!response.ok) {
    alert(text);
    return;
  }
  notificationSettingsByProject.delete(project);
  await showNotificationConfig();
}
function renderConfigTargetOptions(container, targets, fileName, generate) {
  container.replaceChildren(
    Object.assign(document.createElement("p"), {
      textContent: "Choose where to generate " + fileName + ".",
    }),
  );
  const options = document.createElement("div");
  options.className = "config-target-options";
  for (const target of targets || []) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = target.path;
    button.title = "Generate " + fileName + " in " + target.location;
    button.onclick = () => generate(target, button);
    options.append(button);
  }
  container.append(options);
}
async function showGenerateNotificationConfig() {
  const project = notificationConfigProject();
  const modal = document.getElementById("output-modal");
  const output = ensureModalOutput();
  const configEditor = document.getElementById("config-editor");
  const generator = document.getElementById("config-generator");
  const form = document.getElementById("notification-config-editor");
  setModalConfigPaths([]);
  output.hidden = true;
  configEditor.hidden = true;
  generator.hidden = false;
  form.hidden = true;
  form.dataset.editable = "false";
  generator.replaceChildren();
  generator.textContent = "Loading notification config locations...";
  const params = new URLSearchParams();
  if (project) params.set("project_name", project);
  const response = await fetch("/api/notification-config?" + params, {
    cache: "no-store",
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  renderConfigTargetOptions(
    generator,
    JSON.parse(text).targets,
    "notifications.toml",
    (target, button) => generateNotificationConfig(project, target, button),
  );
  modal.querySelector("strong").textContent = "Generate notification config";
  modal.dataset.view = "notification-config-generate";
  openOutputModal(true);
}
async function showNotificationConfig() {
  const project = notificationConfigProject();
  const modal = document.getElementById("output-modal");
  const output = ensureModalOutput();
  const configEditor = document.getElementById("config-editor");
  const generator = document.getElementById("config-generator");
  const form = document.getElementById("notification-config-editor");
  setModalConfigPaths([]);
  output.hidden = true;
  configEditor.hidden = true;
  generator.hidden = true;
  form.hidden = false;
  form.replaceChildren();
  const params = new URLSearchParams();
  if (project) params.set("project_name", project);
  const response = await fetch("/api/notification-config?" + params, {
    cache: "no-store",
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  const payload = JSON.parse(text);
  form.dataset.editable = payload.path ? "true" : "false";
  if (!payload.path) {
    renderConfigTargetOptions(
      form,
      payload.targets,
      "notifications.toml",
      (target, button) => generateNotificationConfig(project, target, button),
    );
  } else {
    const settings = payload.settings;
    const webhookExtras = document.createElement("div");
    webhookExtras.className = "notification-webhook-settings";
    const format = document.createElement("select");
    format.name = "webhook-format";
    for (const value of ["json", "slack", "teams", "discord"]) {
      const option = new Option(
        value,
        value,
        false,
        value === settings.webhook.format,
      );
      format.add(option);
    }
    const url = document.createElement("input");
    const savedWebhookURLMessage = payload.url_set
      ? "Webhook URL is set — edit to replace or clear"
      : "";
    url.type = payload.url_set ? "text" : "password";
    url.name = "webhook-url";
    url.value = savedWebhookURLMessage;
    url.placeholder = "Webhook URL";
    url.title = payload.url_set
      ? "Replace the saved URL, or clear this field to remove it"
      : "Enter a webhook URL";
    url.onfocus = () => {
      if (url.value === savedWebhookURLMessage && savedWebhookURLMessage)
        url.select();
    };
    url.oninput = () => {
      if (url.value !== savedWebhookURLMessage) url.type = "password";
      updateNotificationConfigDirty(form);
    };
    const urlLabel = document.createElement("label");
    urlLabel.className = "notification-webhook-url-label";
    urlLabel.append("Webhook URL ", url);
    const formatLabel = document.createElement("label");
    formatLabel.className = "notification-webhook-format-label";
    formatLabel.append("Webhook format ", format);
    webhookExtras.append(formatLabel, urlLabel);
    const webhook = notificationChannelEditor(
      "webhook",
      settings.webhook,
      payload.fields,
      webhookExtras,
    );
    webhook.append(webhookExtras);
    form.append(
      notificationChannelEditor("browser", settings.browser, payload.fields),
    );
    form.append(webhook);
    setModalConfigPaths([payload.path]);
    initializeNotificationConfigDirtyState(form);
    const save = document.getElementById("notification-config-save");
    form.onsubmit = async (event) => {
      event.preventDefault();
      save.disabled = true;
      const webhookSettings = readNotificationChannel(form, "webhook");
      webhookSettings.format = format.value;
      const webhookURLChanged = url.value !== savedWebhookURLMessage;
      const saveResponse = await fetch("/api/save-notification-config", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          project_name: project,
          settings: {
            webhook: webhookSettings,
            browser: readNotificationChannel(form, "browser"),
          },
          webhook_url: webhookURLChanged ? url.value : "",
          change_webhook_url: webhookURLChanged,
        }),
      });
      const saveText = await saveResponse.text();
      save.disabled = false;
      if (!saveResponse.ok) {
        alert(saveText);
        return;
      }
      notificationSettingsByProject.delete(project);
      await showNotificationConfig();
    };
  }
  modal.querySelector("strong").textContent = "Notification config";
  modal.dataset.view = "notification-config";
  openOutputModal(false);
}
function addConfigButton() {
  document
    .querySelectorAll(
      ".config-button,.notification-config-button,.generate-config-button,.notification-generate-config-button,.launcher-config-button,.launcher-generate-config-button",
    )
    .forEach((button) => button.remove());
  const paths = pageConfigPaths();
  const toolbar = document.getElementById("settings-actions");
  if (!toolbar) return;
  const viewButton = document.createElement("button");
  viewButton.className = "config-button";
  viewButton.textContent = "View config";
  viewButton.disabled = !paths.length;
  viewButton.title = paths.length ? "View config" : "No config file";
  if (paths.length) viewButton.onclick = () => showConfig();
  toolbar.append(viewButton);
  const generateConfigButton = document.createElement("button");
  generateConfigButton.className = "generate-config-button";
  generateConfigButton.textContent = "Generate config";
  generateConfigButton.title = "Generate or replace a config template";
  generateConfigButton.onclick = showGenerateConfig;
  toolbar.append(generateConfigButton);
  const notificationButton = document.createElement("button");
  notificationButton.className = "notification-config-button";
  notificationButton.textContent = "Notification config";
  notificationButton.onclick = showNotificationConfig;
  toolbar.append(notificationButton);
  const generateNotificationButton = document.createElement("button");
  generateNotificationButton.className = "notification-generate-config-button";
  generateNotificationButton.textContent = "Generate notification config";
  generateNotificationButton.title =
    "Generate or replace a notifications.toml template";
  generateNotificationButton.onclick = showGenerateNotificationConfig;
  toolbar.append(generateNotificationButton);
  addLauncherConfigButtons(toolbar);
}
function openRequestedConfigAction() {
  const url = new URL(window.location.href);
  const action = url.searchParams.get("rotari-action");
  if (
    action !== "notification-config" &&
    action !== "generate-notification-config"
  )
    return;
  url.searchParams.delete("rotari-action");
  history.replaceState(null, "", url.pathname + url.search + url.hash);
  const open =
    action === "notification-config"
      ? showNotificationConfig
      : showGenerateNotificationConfig;
  open().catch((error) => alert(error.message));
}
function renderOverview(queues) {
  let runs = 0,
    running = 0;
  queues.forEach((q) => {
    runs += q.run_count || q.runs.length;
    running += q.running_run_id ? 1 : 0;
  });
  setCrumbs([["All projects"]], webStatePath(state.base_dir, "projects"));
  document.getElementById("page-title").textContent = "All projects";
  document.getElementById("summary").innerHTML =
    "<span>" +
    queues.length +
    " projects</span><span>" +
    runs +
    " runs</span><span>" +
    running +
    " running</span>";
  const rows = queues
    .map((q) => {
      const latest = latestRun(q.runs);
      return (
        '<tr><td><a class="link project-name" href="/project/' +
        encodeURIComponent(q.project_name) +
        '">' +
        esc(q.project_name) +
        "</a></td><td>" +
        (q._loaded ? (q.queue.commands || []).length : "—") +
        "</td><td>" +
        (q.run_count || q.runs.length) +
        "</td><td>" +
        (q.running_run_id ? 1 : 0) +
        "</td><td>" +
        (latest
          ? '<a class="link" href="/project/' +
            encodeURIComponent(q.project_name) +
            "/run/" +
            encodeURIComponent(latest.run_id) +
            '">' +
            esc(latest.run_name || latest.run_id) +
            "</a>"
          : "-") +
        "</td><td>" +
        (latest ? statusPill(latest.status) : "-") +
        '</td><td class="time-cell">' +
        esc(latest ? latest.started_at : "-") +
        "</td>" +
        actionsCell([
          pathButton(state.base_dir + "/projects/" + q.project_name),
          deleteProjectButton(q.project_name, !!q.running_run_id),
        ]) +
        "</tr>"
      );
    })
    .join("");
  document.getElementById("app").innerHTML = queues.length
    ? '<table class="runs queue-overview"><thead><tr><th data-sort="name">Project</th><th data-sort="queued">Queued</th><th data-sort="runs">Runs</th><th data-sort="running">Running</th><th>Latest run</th><th data-sort="status">Status</th><th data-sort="started">Started</th><th class="actions"><span class="visually-hidden">Actions</span></th></tr></thead><tbody>' +
      rows +
      "</tbody></table>"
    : '<div class="empty">No projects found.</div>';
}
function renderQueue(q) {
  setCrumbs(
    [["All projects", "/"], [q.project_name]],
    webStatePath(state.base_dir, "projects", q.project_name),
  );
  document.getElementById("page-title").textContent = q.project_name;
  document.getElementById("summary").innerHTML =
    "<span>" +
    (q.queue.commands || []).length +
    " queued</span><span>" +
    q.runs.length +
    " runs</span><span>" +
    q.runs.filter((r) => r.running).length +
    " running</span>";
  const app = document.getElementById("app");
  app.innerHTML = projectRunsHTML(q) + projectRuntimeHTML(q);
  app.prepend(renderQueueCommands(q));
}
// projectRunsHTML is a project page's runs: name over ID, status, client,
// exit code, and times, with Path and Delete last in each row.
function projectRunsHTML(q) {
  const latest = latestRun(q.runs);
  const rows = q.runs
    .map(
      (r) =>
        "<tr" +
        (r === latest ? ' class="latest-run"' : "") +
        '><td class="run-cell" data-sort-value="' +
        esc(r.run_name || r.run_id) +
        '"><a class="link run-name" href="/project/' +
        encodeURIComponent(q.project_name) +
        "/run/" +
        encodeURIComponent(r.run_id) +
        '">' +
        esc(r.run_name || r.run_id) +
        "</a>" +
        (r === latest ? '<span class="latest-badge">latest</span>' : "") +
        '<div class="run-id mono">' +
        esc(r.run_id) +
        copyButtonHTML(r.run_id, "run ID") +
        "</div></td><td>" +
        statusPill(
          r.lifecycle || r.status,
          (r.lifecycle || r.status) + (r.running ? " ..." : ""),
        ) +
        "</td><td>" +
        esc(r.client_label || clientStatusLabel(r.client_status)) +
        "</td><td>" +
        (r.finished_at ? esc(r.exit_code) : "-") +
        '</td><td class="time-cell">' +
        esc(r.started_at || "-") +
        '</td><td class="time-cell">' +
        esc(r.finished_at || "-") +
        "</td>" +
        actionsCell([
          pathButton(
            state.base_dir +
              "/projects/" +
              q.project_name +
              "/runs/" +
              r.run_id,
          ),
          deleteRunButton(q.project_name, r.run_id),
        ]) +
        "</tr>",
    )
    .join("");
  return (
    '<section class="project-runs-section"><div class="table-bar"><h2>Runs</h2><span class="meta">' +
    q.runs.length +
    (q.runs.length === 1 ? " run" : " runs") +
    "</span></div>" +
    (rows
      ? '<table class="runs project-runs"><thead><tr><th data-sort="run_name">Run</th><th data-sort="status">Status</th><th data-sort="client">Client</th><th data-sort="exit">Exit</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th class="actions"><span class="visually-hidden">Actions</span></th></tr></thead><tbody>' +
        rows +
        "</tbody></table>"
      : '<div class="empty">No runs found.</div>') +
    "</section>"
  );
}
// projectRuntimeHTML shows whether a runner holds the project, with the
// lock and coordinator records behind Internal state.
function projectRuntimeHTML(queue) {
  const active = !!queue.running_run_id;
  const runner = active
    ? "Active runner: " +
      queue.running_run_id +
      (queue.runner_host ? " on " + queue.runner_host : "") +
      (queue.runner_pid ? " (PID " + queue.runner_pid + ")" : "")
    : "No runner lock";
  const server = queue.server || {};
  const coordinator = server.pid_file_exists
    ? server.pid
      ? "PID " + server.pid
      : "PID record unreadable"
    : "no PID record";
  return (
    '<section class="project-runtime"><div class="table-bar"><h2>Project runtime</h2><span class="' +
    (active ? statusPillClass("running") : "meta") +
    '">' +
    esc(runner) +
    "</span></div><details" +
    (projectRuntimeDetailsOpen ? " open" : "") +
    '><summary>Internal state</summary><div class="meta runtime-details">Runner lock: ' +
    (active ? "present" : "absent") +
    (queue.runner_started_at
      ? " | Started: " + esc(queue.runner_started_at)
      : "") +
    "<br>Coordinator record: " +
    esc(coordinator) +
    "<br>State lock: advisory and intentionally not probed</div></details></section>"
  );
}
// lineageDiagnosisText lists a run's rule diagnoses as the CLI summary does,
// leaving out no_match, which only says that no rule applied.
function lineageDiagnosisText(diagnoses) {
  return (diagnoses || [])
    .filter((item) => item.name !== "no_match")
    .map((item) => item.name + " " + item.count)
    .join(", ");
}
function renderRun(q, runID) {
  const run = q.runs.find((r) => r.run_id === runID);
  if (!run) {
    renderMissing("Run not found");
    return;
  }
  setCrumbs(
    [
      [q.project_name, "/project/" + encodeURIComponent(q.project_name)],
      [run.run_name || runID],
    ],
    webStatePath(state.base_dir, "projects", q.project_name, "runs", runID),
  );
  document.getElementById("page-title").innerHTML =
    esc(run.run_name || runID) + " " + statusPill(run.lifecycle || run.status);
  const lineage = run.lineage_summary;
  const diagnosisText = lineage ? lineageDiagnosisText(lineage.diagnoses) : "";
  const originText = lineage
    ? (lineage.origins || [])
        .map((item) => (item.run_id || "new") + " " + item.count)
        .join(", ")
    : "";
  const cwd = run.cwd || "-";
  const host = run.context && run.context.hostname;
  document.getElementById("summary").innerHTML = [
    fact("Project", esc(q.project_name)),
    fact(
      "Run ID",
      "<code>" +
        esc(run.run_id) +
        "</code>" +
        copyIconForValue(run.run_id, "run ID"),
    ),
    fact("Status", esc(run.status)),
    fact("Lifecycle", esc(run.lifecycle || "unknown")),
    fact(
      "Client",
      esc(run.client_label || clientStatusLabel(run.client_status)),
    ),
    fact("Exit", run.finished_at ? esc(run.exit_code) : "-"),
    fact("Started", esc(run.started_at || "-")),
    fact("Finished", esc(run.finished_at || "-")),
    host ? fact("Host", esc(host)) : "",
    run.context && run.context.started_by
      ? fact("Started by", esc(startedByLabel(run.context.started_by)))
      : "",
    fact(
      "Working directory",
      "<code>" +
        esc(cwd) +
        "</code>" +
        copyIconForValue(cwd, "working directory"),
    ),
    ...(run.source_labels || []).map((label) =>
      fact("Source", "<code>" + esc(label) + "</code>"),
    ),
    diagnosisText ? fact("Diagnoses", esc(diagnosisText)) : "",
    originText ? fact("Origins", esc(originText)) : "",
  ].join("");
  if (run.unreadable) {
    // The run's files come from a newer rotari; only the reason is known.
    document.getElementById("app").innerHTML =
      '<div class="empty error">' + esc(run.unreadable) + "</div>";
    return;
  }
  if (!Array.isArray(run.jobs)) {
    document.getElementById("app").innerHTML =
      '<div class="empty">Loading run details…</div>';
    return;
  }
  (run.jobs || []).forEach((job) => {
    const attemptID =
      selectedAttemptByJob[q.project_name + "/" + runID + "/" + job.id];
    if (attemptID) selectJobAttemptFields(job, attemptID);
  });
  const latest = latestRun(q.runs);
  const retry =
    "# To retry failed or unfinished jobs from this run\n" +
    (run.cwd && run.cwd !== "-" ? "cd " + shellQuote(run.cwd) + "\n" : "") +
    "rotari retry -r " +
    shellQuote(runID) +
    (latest && latest.run_id === runID
      ? ""
      : "\nrotari retry -r " + shellQuote(runID) + " --job-id JOB_ID");
  document.getElementById("app").innerHTML =
    runNotesHTML(run) +
    commandGuideHTML(retry) +
    runCardsHTML(run) +
    ((run.jobs || []).length
      ? runJobsHTML(q, run, runID)
      : '<div class="empty">No job definitions yet.</div>');
}
// selectJobAttemptFields shows an older attempt in a job's row: the row
// already shows the latest attempt with the job's summary result.
function selectJobAttemptFields(job, attemptID) {
  const attempt = (job.attempts || []).find((item) => item.id === attemptID);
  if (!attempt || attempt === job.attempts[0]) return;
  job.attempt_id = attempt.id;
  job.result = attempt.result;
  job.execution_status = attempt.execution_status;
  job.diagnosis_outdated = false;
  job.submitted_at = attempt.submitted_at;
  job.finished_at = attempt.finished_at;
  job.elapsed = attempt.elapsed;
  job.scheduler_state = attempt.scheduler_state;
}
// runJobColumns are the run page's job table columns (columnTable) between
// the selection box and the row's actions.
const runJobColumns = [
  { key: "name", label: "Job", show: "fixed" },
  { key: "status", label: "Status", show: "fixed" },
  { key: "exit", label: "Exit", show: "fixed" },
  { key: "attempt", label: "Attempt", show: "shown" },
  { key: "id", label: "Job ID", show: "hidden" },
  { key: "hosts", label: "Hosts", show: "shown" },
  { key: "executor", label: "Executor", show: "auto" },
  { key: "options", label: "Executor options", show: "auto" },
  { key: "stage", label: "Stage", show: "auto" },
  { key: "depends", label: "Dependencies", show: "auto" },
  { key: "working_directory", label: "Working directory", show: "auto" },
  { key: "started", label: "Started", show: "shown" },
  { key: "finished", label: "Finished", show: "hidden" },
  { key: "duration", label: "Duration", show: "shown" },
];
// columnTable draws a table whose columns the viewer can turn on and off
// from a Columns menu (a run's jobs, a project's queue). A column is "fixed"
// (always shown), "shown" or "hidden" by default, or "auto": shown only when
// its value differs between rows (cell.same, or else its HTML). The viewer's
// choices are kept per browser under rotari-NAME-columns. Every column stays
// in the table, so sorting and clamping find it by its data-sort key; one
// that is turned off carries the hidden attribute. Each row gives its
// attributes, its cells by column key, and the HTML of its cells before
// (lead) and after (trail) the columns; header gives the matching headers.
function columnTable(name, columns, rows, header, className) {
  const choices = tableColumnChoices(name);
  const shownColumns = columns.map((column) => {
    const same =
      new Set(
        rows.map((row) => {
          const cell = row.cells[column.key];
          return cell.same !== undefined ? cell.same : cell.html;
        }),
      ).size <= 1;
    const auto =
      column.show === "fixed" ||
      column.show === "shown" ||
      (column.show === "auto" && !same);
    const shown =
      column.show === "fixed" || (choices[column.key] ?? auto) === true;
    return { ...column, same, shown };
  });
  const hidden = (column) => (column.shown ? "" : " hidden");
  const menu =
    '<details class="popup-menu columns-menu"' +
    popupMenuAttributes(name + "-columns") +
    '><summary>Columns</summary><div class="popup-menu-items"><p class="meta">Columns whose values are the same for every row start hidden.</p>' +
    shownColumns
      .filter((column) => column.show !== "fixed")
      .map(
        (column) =>
          '<label><input type="checkbox" data-table="' +
          name +
          '" data-column="' +
          column.key +
          '"' +
          (column.shown ? " checked" : "") +
          ' onchange="setTableColumn(this)"> ' +
          column.label +
          (column.show === "auto" && column.same
            ? ' <span class="meta">same for all</span>'
            : "") +
          "</label>",
      )
      .join("") +
    "</div></details>";
  const table =
    '<table class="runs ' +
    className +
    '"><thead><tr>' +
    (header.lead || "") +
    shownColumns
      .map(
        (column) =>
          '<th data-sort="' +
          column.key +
          '"' +
          hidden(column) +
          ">" +
          column.label +
          "</th>",
      )
      .join("") +
    (header.trail || "") +
    "</tr></thead><tbody>" +
    rows
      .map(
        (row) =>
          "<tr" +
          (row.attributes || "") +
          ">" +
          (row.lead || "") +
          shownColumns
            .map((column) => {
              const cell = row.cells[column.key];
              return (
                "<td" +
                (cell.className ? ' class="' + cell.className + '"' : "") +
                (cell.sort !== undefined
                  ? ' data-sort-value="' + esc(cell.sort) + '"'
                  : "") +
                hidden(column) +
                ">" +
                cell.html +
                "</td>"
              );
            })
            .join("") +
          (row.trail || "") +
          "</tr>",
      )
      .join("") +
    "</tbody></table>";
  return { menu, table };
}
function tableColumnChoices(name) {
  try {
    const value = JSON.parse(
      localStorage.getItem("rotari-" + name + "-columns") || "{}",
    );
    return value && typeof value === "object" ? value : {};
  } catch {
    return {};
  }
}
function setTableColumn(input) {
  const choices = tableColumnChoices(input.dataset.table);
  choices[input.dataset.column] = input.checked;
  try {
    localStorage.setItem(
      "rotari-" + input.dataset.table + "-columns",
      JSON.stringify(choices),
    );
  } catch {
    // The choice then lasts until the next render.
  }
  render();
}
// runJobsHTML draws a run's jobs table under its Jobs heading, selection
// buttons, Columns menu, and toolbar.
function runJobsHTML(q, run, runID) {
  const { menu, table } = columnTable(
    "run",
    runJobColumns,
    run.jobs.map((job) => ({
      attributes: ' data-job-id="' + esc(job.id) + '"',
      lead:
        '<td class="selection"><input class="job-selection" type="checkbox" aria-label="Select ' +
        esc(job.id) +
        '"></td>',
      cells: runJobCells(q, run, runID, job),
      trail: runJobActions(q, run, runID, job),
    })),
    {
      lead: '<th class="selection"></th>',
      trail:
        '<th class="actions"><span class="visually-hidden">Actions</span></th>',
    },
    "run-jobs",
  );
  return (
    '<div class="table-bar run-jobs-bar"><h2>Jobs</h2>' +
    runSelectionHTML() +
    menu +
    "</div>" +
    runActionsHTML(q, run, runID) +
    table
  );
}
// runSelectionHTML is the jobs table's selection buttons and count. They
// change only the check boxes; updateSelectedRunJobs (web_app_actions.js)
// enables them and keeps the count.
function runSelectionHTML() {
  return (
    '<div class="toolbar run-selection" role="group" aria-label="Select jobs">' +
    '<button type="button" onclick="selectAllRunJobs()" title="Select every job">Select all</button>' +
    '<button type="button" class="select-failed" onclick="selectFailedJobs()">Select failed</button>' +
    '<button type="button" class="select-failed-unfinished" onclick="selectFailedUnfinishedJobs()">Select failed + unfinished</button>' +
    '<button type="button" class="unselect-all" disabled onclick="clearSelectedJobs()" title="Clear all selected jobs">Unselect all</button>' +
    '<span class="selection-count meta" aria-live="polite"></span></div>'
  );
}
// runActionsHTML is the run page's toolbar: actions on the checked jobs,
// then Files for the directory holding the basedir on the live server,
// Cancel run while the run is running, and Delete run. Buttons that need a selection start disabled until
// updateSelectedRunJobs enables them.
function runActionsHTML(q, run, runID) {
  const data =
    ' data-project="' + esc(q.project_name) + '" data-run="' + esc(runID) + '"';
  const filesButton = runFilesButton(q.project_name, runID);
  return (
    '<div class="toolbar web-copy-controls run-toolbar">' +
    '<button type="button" class="create-selected" disabled' +
    data +
    ' onclick="copySelectedJobs(this.dataset.project, this.dataset.run, false)" title="Replace the queue with the selected jobs">Create queue</button>' +
    '<button type="button" class="append-selected" disabled' +
    data +
    ' onclick="copySelectedJobs(this.dataset.project, this.dataset.run, true)" title="Add the selected jobs to the queue">Append to queue</button>' +
    '<button type="button" class="run-ai" disabled title="Prepare run report"' +
    data +
    ' onclick="showRunReport(this)">Report</button>' +
    '<button type="button" class="cancel-selected-jobs" disabled onclick="controlSelectedRunJobs(\'cancel\')">Cancel selected</button>' +
    '<button type="button" class="suspend-resume-selected-jobs" disabled onclick="controlSelectedRunJobs(\'suspend-resume\')">Suspend selected</button>' +
    '<span class="toolbar-gap"></span>' +
    (filesButton ? filesButton + '<span class="toolbar-gap"></span>' : "") +
    '<button type="button" class="run-config-snapshot" onclick="showConfig(true)"' +
    (run.context?.config_snapshot_paths?.length
      ? ' title="View the configuration saved with this run"'
      : ' disabled title="No config snapshot"') +
    ">View config snapshot</button>" +
    (run.running
      ? '<button type="button" class="cancel-run"' +
        data +
        ' onclick="cancelRun(this.dataset.project, this.dataset.run)">Cancel run</button>'
      : "") +
    '<button type="button" class="delete-run"' +
    data +
    (run.running
      ? ' disabled title="Running runs cannot be deleted"'
      : ' title="Delete run history"') +
    ' onclick="deleteRun(this.dataset.project, this.dataset.run)">Delete run</button></div>'
  );
}
function showRunReport(button) {
  const project = state.projects.find(
    (item) => item.project_name === button.dataset.project,
  );
  const run =
    project && project.runs.find((item) => item.run_id === button.dataset.run);
  if (run) showAIReport(project, run, null, selectedRunJobIDs());
}
// copyButtonHTML is a small icon that copies value; label names it.
function copyButtonHTML(value, label) {
  return (
    '<button class="table-copy identity-copy" type="button" title="Copy ' +
    label +
    '" aria-label="Copy ' +
    label +
    '" data-copy-value="' +
    esc(value) +
    '" data-copy-title="Copy ' +
    label +
    '" onclick="copyIdentityValue(this)">' +
    copyIconSVG +
    "</button>"
  );
}
// idTailHTML shows a long ID by its end, which tells a run's attempts
// apart (.id-tail in web_shell.css); its text and title keep the whole ID.
// assets.go writes the same markup on the Job activity page.
function idTailHTML(id) {
  return (
    '<span class="id-tail" title="' +
    esc(id) +
    '"><bdi>' +
    esc(id) +
    "</bdi></span>"
  );
}
// runTimeCell shows a job's time without the date when it is the run's start
// date; the full time is in its title and sorts the column.
function runTimeCell(value, run) {
  if (!value) return { className: "time-cell", html: "-" };
  const day = (run.started_at || "").slice(0, 11);
  const short =
    day.length === 11 && value.startsWith(day)
      ? value.slice(11).replace(/ [A-Z]+$/, "")
      : value;
  return {
    className: "time-cell",
    sort: value,
    html: '<span title="' + esc(value) + '">' + esc(short) + "</span>",
  };
}
// elapsedSeconds reads a duration as rotari formats it ("5s", "3m 05s",
// "2h 07m") for sorting.
function elapsedSeconds(text) {
  const units = { h: 3600, m: 60, s: 1 };
  let total = 0;
  for (const match of String(text).matchAll(/(\d+)([hms])/g))
    total += Number(match[1]) * units[match[2]];
  return total;
}
// runJobCells is a job's cell in each of runJobColumns: its HTML, and its
// sort value when that differs from the text.
function runJobCells(q, run, runID, j) {
  const result = j.result;
  const carried =
    j.origin && (!j.attempt_id || j.attempt_id === (j.origin.attempt_id || ""));
  const hosts = (result && result.hosts) || [];
  const commandText = (j.command || []).join(" ");
  const plain = (value, copyLabel) => ({
    html:
      esc(value || "-") +
      (value && copyLabel ? copyButtonHTML(value, copyLabel) : ""),
  });
  const attemptMenu =
    (j.attempts || []).length > 1
      ? '<details class="popup-menu attempt-menu"' +
        popupMenuAttributes(attemptMenuKey(q.project_name, runID, j.id)) +
        '><summary title="Select attempt" aria-label="Select attempt"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="m7 10 5 5 5-5"></path></svg></summary><div class="attempt-options">' +
        j.attempts
          .map(
            (attempt, index) =>
              '<button type="button" class="' +
              (attempt.id === j.attempt_id ? "selected" : "") +
              '" onclick="selectJobAttempt(\'' +
              esc(q.project_name) +
              "','" +
              esc(runID) +
              "','" +
              esc(j.id) +
              "','" +
              esc(attempt.id) +
              "')\">" +
              (index === 0 ? "Latest: " : "") +
              esc(attempt.id) +
              "</button>",
          )
          .join("") +
        "</div></details>"
      : "";
  const status = jobDisplayStatus(j, run);
  return {
    name: {
      className: "job-cell",
      sort: j.name || j.id,
      html:
        '<div class="job-title"><strong>' +
        esc(j.name || "-") +
        "</strong>" +
        (j.name ? copyButtonHTML(j.name, "job name") : "") +
        "</div>" +
        (carried
          ? '<div class="meta">carried from ' + esc(j.origin.run_id) + "</div>"
          : "") +
        '<div class="job-command" data-clamp="command">' +
        esc(commandText) +
        copyButtonHTML(commandText, "command") +
        "</div>",
    },
    status: { sort: status, html: statusPill(status) },
    exit: {
      html:
        (result ? esc(result.exit_code) : "-") +
        (result && result.error
          ? '<div class="error">' + esc(result.error) + "</div>"
          : ""),
    },
    attempt: {
      className: "id-cell",
      sort: j.attempt_id || "-",
      html: j.attempt_id
        ? idTailHTML(j.attempt_id) +
          copyButtonHTML(j.attempt_id, "attempt ID") +
          attemptMenu
        : "-" + attemptMenu,
    },
    id: {
      className: "id-cell",
      html:
        '<span class="mono">' +
        esc(j.id) +
        "</span>" +
        copyButtonHTML(j.id, "job ID"),
    },
    hosts: plain(hosts.join(","), "hosts"),
    executor: plain(j.executor || "default"),
    options: plain((j.executor_options || []).join(" "), "executor options"),
    stage: plain(j.stage, "stage name"),
    depends: plain(formatDependencies(j)),
    working_directory: plain(j.working_directory, "working directory"),
    started: runTimeCell(j.submitted_at, run),
    finished: runTimeCell(j.finished_at, run),
    duration: {
      className: "time-cell",
      sort: j.elapsed ? String(elapsedSeconds(j.elapsed)) : "-",
      html: esc(j.elapsed || "-"),
    },
  };
}
// runJobActions is a job row's actions cell, last in the row: the log, the
// artifacts of a finished job and, where launching is available, a launch
// on its working directory, the diagnosis of a failed job, the job's notes,
// its report, and its path.
function runJobActions(q, run, runID, j) {
  const result = j.result;
  // A carried job's log and artifacts are asked for in this run without an
  // attempt; the server follows every carry to the attempt that produced
  // the result (jobstatus.ResultAttemptDir).
  const carried =
    j.origin && (!j.attempt_id || j.attempt_id === (j.origin.attempt_id || ""));
  const logRun = runID;
  const logJob = j.id;
  const logAttemptID = carried ? "" : j.attempt_id;
  const jobData =
    ' data-project="' +
    esc(q.project_name) +
    '" data-run="' +
    esc(runID) +
    '" data-job="' +
    esc(j.id) +
    '"';
  // A finished job's log is its result's attempt, in the origin run for a
  // carried job; an unfinished job's log is its current attempt.
  const started = !!j.submitted_at || !!result || run.running;
  const viewLog = result
    ? '<button class="view-log" data-project="' +
      esc(q.project_name) +
      '" data-log-run="' +
      esc(logRun) +
      '" data-log-job="' +
      esc(logJob) +
      '" data-attempt="' +
      esc(logAttemptID) +
      '" data-log-mode="' +
      esc(j.log_mode || "merge") +
      '" onclick="log(this.dataset.project, this.dataset.logRun, this.dataset.logJob, this.dataset.attempt, this.dataset.logMode)">Log</button>'
    : '<button class="view-log"' +
      jobData +
      ' data-attempt="' +
      esc(j.attempt_id || "") +
      '" data-log-mode="' +
      esc(j.log_mode || "") +
      '"' +
      (started ? "" : ' disabled title="Job has not started yet"') +
      " onclick=\"showLog(this.dataset.project, this.dataset.run, this.dataset.job, this.dataset.attempt, 'stdout', this.dataset.logMode)\">Log</button>";
  const diagnoses = (result && result.diagnoses) || [];
  const diagnosisStatus = (result && result.diagnosis_status) || "";
  const failed = !!(result && result.exit_code !== 0);
  const diagnosisControl = !failed
    ? ""
    : diagnosisStatus || diagnoses.length
      ? '<button class="diagnosis" data-diagnoses="' +
        esc(
          JSON.stringify({
            status: diagnosisStatus,
            note: result.diagnosis_note || "",
            outdated: !!j.diagnosis_outdated,
            diagnoses: diagnoses,
          }),
        ) +
        '" onclick="showDiagnosis(this)">Diagnosis</button>'
      : '<button class="diagnosis" disabled title="Available after a finalized failed result with saved analysis">Diagnosis</button>';
  const noteLabels = j.note_labels || [];
  const notesControl = noteLabels.length
    ? '<button class="view-notes" data-notes="' +
      esc(JSON.stringify(noteLabels)) +
      '" onclick="showNotes(this)">Notes (' +
      noteLabels.length +
      ")</button>"
    : '<button class="view-notes" disabled title="No notes on this job; add one with rotari note ATTEMPT_ID TEXT">Notes</button>';
  // An unfinished job's artifacts are its current attempt's, as its log is;
  // its record exists once the attempt has started.
  const artifactAttemptID = result ? logAttemptID : j.attempt_id || "";
  const artifactsReady = !!result || !!j.submitted_at;
  const notStarted = ' disabled title="Job has not started yet"';
  const artifacts = artifactsReady
    ? '<button class="view-artifacts" onclick="showArtifacts(\'' +
      esc(q.project_name) +
      "','" +
      esc(logRun) +
      "','" +
      esc(logJob) +
      "','" +
      esc(artifactAttemptID) +
      "')\">Artifacts</button>"
    : '<button class="view-artifacts"' + notStarted + ">Artifacts</button>";
  // Launch opens the same attempt's Artifacts at its working directory's
  // row of commands, where a tracker's default store usually is.
  const launchAvailable =
    launcherAvailability && launcherAvailability.available;
  const launch = !launchAvailable
    ? ""
    : artifactsReady
      ? '<button class="launch-job" onclick="launchOnWorkingDirectory(' +
        jsArg(q.project_name) +
        "," +
        jsArg(logRun) +
        "," +
        jsArg(logJob) +
        "," +
        jsArg(artifactAttemptID) +
        ')">Launch</button>'
      : '<button class="launch-job"' + notStarted + ">Launch</button>";
  return actionsCell([
    viewLog,
    artifacts,
    launch,
    diagnosisControl,
    notesControl,
    '<button class="job-ai" title="Prepare job report"' +
      jobData +
      ' onclick="showJobReport(this)">Report</button>',
    pathButton(
      state.base_dir +
        "/projects/" +
        q.project_name +
        "/runs/" +
        runID +
        "/" +
        j.id,
    ),
  ]);
}
// startedByLabel shows how a run was started as model.StartedBy.Label
// does: "cli", or "cli · claude-code" with an actor.
function startedByLabel(startedBy) {
  return startedBy.actor
    ? startedBy.interface + " · " + startedBy.actor
    : startedBy.interface;
}
// fact is one "Label: value" entry of a page's facts line; valueHTML is
// already escaped.
function fact(label, valueHTML) {
  return (
    '<span class="fact"><span class="fact-label">' +
    label +
    ":</span> " +
    valueHTML +
    "</span>"
  );
}
// runNotesHTML shows the notes on a run itself, oldest first; notes on a job
// are behind that job's Notes button.
function runNotesHTML(run) {
  const notes = (run.notes || []).filter((note) => !note.job_id);
  if (!notes.length) return "";
  return (
    '<div class="run-notes">' +
    notes
      .map(
        (note) =>
          '<div class="run-note"><div class="run-note-meta">Note · ' +
          esc(note.at ? formatDateTime(note.at) : "") +
          '</div><div class="run-note-body">' +
          renderMarkdown(note.text) +
          "</div></div>",
      )
      .join("") +
    "</div>"
  );
}
// jobCountGroups are the Jobs card's groups, in the order its bar and
// counts list them.
const jobCountGroups = [
  ["ok", "succeeded"],
  ["bad", "failed"],
  ["run", "in progress"],
  ["pending", "pending"],
  ["warn", "blocked"],
  ["off", "other"],
];
// jobCountGroup groups a job's displayed status (jobDisplayStatus) by its
// tone, telling jobs waiting to start from the rest of the "off" tone.
function jobCountGroup(status) {
  const tone = statusTone(status);
  if (tone !== "off") return tone;
  const base = statusBase(status);
  return base === "pending" || base === "waiting" ? "pending" : "off";
}
// runCardsHTML summarises a run in four cards: its jobs by result, its
// failure causes, its wall time, and the host load when it ended.
function runCardsHTML(run) {
  const jobs = run.jobs || [];
  const counts = { ok: 0, bad: 0, run: 0, pending: 0, warn: 0, off: 0 };
  jobs.forEach((job) => counts[jobCountGroup(jobDisplayStatus(job, run))]++);
  const lineage = run.lineage_summary;
  const failures = (lineage && lineage.failures) || [];
  const failureCount = failures.reduce((sum, group) => sum + group.count, 0);
  const elapsed = lineage && lineage.run && lineage.run.elapsed;
  const context = run.context || {};
  const samples = context.load_samples || [];
  const load = samples.length
    ? samples[samples.length - 1]
    : context.started_load;
  const fixed = (value) => Number(value).toFixed(2);
  const card = (label, value, detail, extra = "") =>
    '<div class="run-card"><div class="run-card-label">' +
    label +
    '</div><div class="run-card-value">' +
    value +
    (detail ? " <small>" + detail + "</small>" : "") +
    "</div>" +
    extra +
    "</div>";
  const bar =
    '<div class="run-card-bar" aria-hidden="true">' +
    jobCountGroups
      .filter(([group]) => counts[group])
      .map(
        ([group]) =>
          '<span class="tone-' +
          (group === "pending" ? "off" : group) +
          '" style="flex:' +
          counts[group] +
          '"></span>',
      )
      .join("") +
    "</div>";
  // Succeeded, failed, in progress, and pending are always counted; blocked
  // and other only when a job is.
  const jobDetail = jobCountGroups
    .filter(([group]) => counts[group] || !["warn", "off"].includes(group))
    .map(([group, label]) => counts[group] + " " + label)
    .join(" · ");
  return (
    '<div class="run-cards">' +
    card("Jobs", jobs.length, esc(jobDetail), jobs.length ? bar : "") +
    card(
      "Failure causes",
      failureCount,
      esc(failures.map((group) => group.cause + " " + group.count).join(" · ")),
    ) +
    card("Wall time", esc(elapsed || "-"), "") +
    card(
      "Load at end",
      load ? fixed(load.one) : "-",
      load ? "/ " + fixed(load.five) + " / " + fixed(load.fifteen) : "",
    ) +
    "</div>"
  );
}
const copyIconSVG =
  '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
// commandGuideHTML is a command a reader can copy to a terminal.
function commandGuideHTML(text) {
  return (
    '<pre class="command-guide execution-guide command-example" data-copy-text="' +
    esc(text) +
    '">' +
    esc(text) +
    '<span class="command-guide-copied" role="status">Copied!</span><button class="command-guide-copy" type="button" title="Copy command" aria-label="Copy command" data-icon="' +
    esc(copyIconSVG) +
    '" onclick="copyCommandGuide(this.closest(\'pre\'), this)">' +
    copyIconSVG +
    "</button></pre>"
  );
}

function clientStatusLabel(status) {
  if (!status) return "unknown";
  switch (status.state) {
    case "attached":
      return "attached";
    case "detached":
      return (
        {
          async: "detached (async)",
          "ctrl-d": "detached (Ctrl-D)",
          disconnect: "detached (disconnect)",
        }[status.reason] || "detached"
      );
    case "cancelling":
      return "disconnecting/cancelling";
    case "completed":
      if (status.mode === "async") return "async (completed)";
      return (
        {
          "ctrl-d": "sync (completed; Ctrl-D detached)",
          disconnect: "sync (completed; disconnected)",
          "disconnect-cancel": "sync (completed; cancelled after disconnect)",
          "ctrl-c": "sync (completed; Ctrl-C)",
        }[status.reason] || "sync (completed)"
      );
    default:
      return status.mode === "async" ? "async (unknown)" : "unknown";
  }
}
function attemptMenuKey(projectName, runID, jobID) {
  return "attempt:" + projectName + "/" + runID + "/" + jobID;
}
function selectJobAttempt(projectName, runID, jobID, attemptID) {
  selectedAttemptByJob[projectName + "/" + runID + "/" + jobID] = attemptID;
  setPopupMenuOpen(attemptMenuKey(projectName, runID, jobID), false);
  render();
}
function setAttemptMenuOpen(projectName, runID, jobID, open) {
  setPopupMenuOpen(attemptMenuKey(projectName, runID, jobID), open);
}
// A popup menu in a table (a job's attempts or other actions, the Columns
// menu) is a <details class="popup-menu"> whose open state is kept by key,
// so the periodic re-render leaves it open; a press outside closes it. Its
// items are placed in the viewport beside the menu's button, so the
// sideways-scrolling #app does not clip them.
const openPopupMenus = new Set();
function popupMenuAttributes(key) {
  return (
    ' data-menu-key="' +
    esc(key) +
    '" ontoggle="popupMenuToggled(this)"' +
    (openPopupMenus.has(key) ? " open" : "")
  );
}
function setPopupMenuOpen(key, open) {
  if (open) openPopupMenus.add(key);
  else openPopupMenus.delete(key);
}
function popupMenuToggled(menu) {
  setPopupMenuOpen(menu.dataset.menuKey, menu.open);
  placePopupMenu(menu);
}
// placePopupMenu puts an open menu's items under its button, aligned to the
// button's right edge, or above the button when there is no room below.
function placePopupMenu(menu) {
  const items = menu.querySelector(":scope > div");
  const button = menu.querySelector(":scope > summary");
  if (!menu.open || !items || !button) return;
  const rect = button.getBoundingClientRect();
  items.style.position = "fixed";
  items.style.left = "auto";
  items.style.right =
    Math.max(8, document.documentElement.clientWidth - rect.right) + "px";
  const height = items.offsetHeight;
  const below = rect.bottom + 4;
  items.style.top =
    below + height > window.innerHeight && rect.top - 4 - height > 0
      ? rect.top - 4 - height + "px"
      : below + "px";
}
function placePopupMenus() {
  document.querySelectorAll("details.popup-menu[open]").forEach(placePopupMenu);
}
document.addEventListener("scroll", placePopupMenus, true);
window.addEventListener("resize", placePopupMenus);
function closePopupMenus(except) {
  document.querySelectorAll("details.popup-menu[open]").forEach((menu) => {
    if (menu === except) return;
    menu.open = false;
    setPopupMenuOpen(menu.dataset.menuKey, false);
  });
}
document.addEventListener("pointerdown", (event) => {
  const target = event.target instanceof Element ? event.target : null;
  closePopupMenus(target && target.closest("details.popup-menu"));
});
function renderMissing(message) {
  setCrumbs([["Not found"]], "");
  document.getElementById("page-title").textContent = "Not found";
  document.getElementById("summary").textContent = "";
  document.getElementById("app").innerHTML =
    '<a class="link" href="/">All projects</a><p>' + esc(message) + "</p>";
}
function copyIconForValue(value, label) {
  return (
    '<button class="command-guide-copy identity-copy" type="button" title="Copy ' +
    label +
    '" aria-label="Copy ' +
    label +
    '" data-copy-value="' +
    esc(value) +
    '" data-copy-title="Copy ' +
    label +
    '" onclick="copyIdentityValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>'
  );
}
async function copyIdentityValue(button) {
  try {
    await copyText(button.dataset.copyValue || "");
    button.classList.add("copied");
    button.title = "Copied!";
    button.setAttribute("aria-label", "Copied!");
    button.innerHTML =
      '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"></path></svg>';
    clearTimeout(button.copyResetTimer);
    button.copyResetTimer = setTimeout(() => {
      button.classList.remove("copied");
      button.title = button.dataset.copyTitle || "Copy value";
      button.setAttribute(
        "aria-label",
        button.dataset.copyTitle || "Copy value",
      );
      button.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
    }, 1200);
  } catch (error) {
    alert(error.message);
  }
}
// queueJobColumns are the queue table's columns (columnTable) after its
// actions. Hidden editors stay in the row, so Save still reads them.
const queueJobColumns = [
  { key: "name", label: "Job name / ID", show: "fixed" },
  { key: "array", label: "Array", show: "auto" },
  { key: "status", label: "Status", show: "auto" },
  { key: "executor", label: "Executor", show: "auto" },
  { key: "options", label: "Executor options", show: "auto" },
  { key: "stage", label: "Stage", show: "auto" },
  { key: "depends", label: "Dependencies", show: "auto" },
  { key: "working_directory", label: "Working directory", show: "auto" },
  { key: "command", label: "Command", show: "fixed" },
  { key: "source_run", label: "Source run", show: "auto" },
  { key: "source_status", label: "Source status", show: "shown" },
  { key: "source_started", label: "Source started", show: "hidden" },
  { key: "source_finished", label: "Source finished", show: "hidden" },
];
// queueJobActions is a queued job's actions cell, first in its row beside
// the editors: its source attempt's log, Save, and Remove.
function queueJobActions(queue, j) {
  const origin = j.origin;
  const jobData =
    ' data-project="' +
    esc(queue.project_name) +
    '" data-job="' +
    esc(j.id) +
    '"';
  return actionsCell([
    origin
      ? '<button class="view-log" data-run="' +
        esc(origin.run_id) +
        '" data-job="' +
        esc(origin.job_id) +
        '" onclick="showOriginalOutput(this.dataset.run, this.dataset.job, this)">View log</button>'
      : "",
    '<button class="save-job"' +
      jobData +
      " disabled onclick=\"saveQueueJob(this.dataset.project, this.dataset.job, this.closest('tr'))\">Save</button>",
    '<button class="remove-job"' +
      jobData +
      ' data-label="' +
      esc(j.name || j.id) +
      '" onclick="removeQueueJob(this.dataset.project, this.dataset.job, this.dataset.label)">Remove</button>',
  ]);
}
// queueJobCells is a queued job's cell in each of queueJobColumns.
function queueJobCells(j, executorOptions, input) {
  const origin = j.origin;
  const recorded = queuedStatusText(
    origin ? origin.status : "",
    j.marked_status,
  );
  const options = JSON.stringify(j.executor_options || []);
  const depends = JSON.stringify(j.depends_on || []);
  return {
    name: {
      html:
        input("job-name-input", j.name || "") +
        '<div class="meta">' +
        esc(j.id) +
        "</div>",
    },
    array: { html: j.array ? esc(j.array.first + "-" + j.array.last) : "-" },
    status: { html: statusPill("pending") },
    executor: {
      same: j.executor || "local",
      html: '<select class="executor-input">' + executorOptions + "</select>",
    },
    options: { same: options, html: input("executor-option-input", options) },
    stage: {
      html:
        esc(j.stage || "-") +
        (j.stage ? copyIconForValue(j.stage, "stage name") : ""),
    },
    depends: { same: depends, html: input("depends-input", depends) },
    working_directory: {
      same: j.working_directory || "",
      html: input(
        "working-directory-input",
        j.working_directory || "",
        "working directory",
      ),
    },
    command: { html: input("command-input", JSON.stringify(j.command)) },
    source_run: {
      html: origin ? esc(origin.run_id + "/" + origin.job_id) : "-",
    },
    source_status: { html: recorded === "-" ? "-" : statusPill(recorded) },
    source_started: { html: esc((origin && origin.submitted_at) || "-") },
    source_finished: { html: esc((origin && origin.finished_at) || "-") },
  };
}
// renderQueueCommands returns a project's queue section: the jobs' source,
// one table with the Actions cell first (the source run's log, Save,
// Remove), editors for the fields a queued job can change, and the run each
// job was copied from, then the command that runs the queue.
function renderQueueCommands(queue) {
  const commands = queue.queue.commands || [];
  const section = document.createElement("section");
  section.className = "web-queue-commands";
  const executorOptions = executorNames
    .map(
      (name) => '<option value="' + esc(name) + '">' + esc(name) + "</option>",
    )
    .join("");
  const input = (className, value, placeholder) =>
    '<input class="' +
    className +
    '"' +
    (placeholder ? ' placeholder="' + placeholder + '"' : "") +
    ' value="' +
    esc(value) +
    '">';
  const { menu, table } = columnTable(
    "queue",
    queueJobColumns,
    commands.map((j) => ({
      attributes: ' data-job-id="' + esc(j.id) + '"',
      lead: queueJobActions(queue, j),
      cells: queueJobCells(j, executorOptions, input),
    })),
    {
      lead: '<th class="actions">Actions</th>',
    },
    "web-queue-jobs",
  );
  const origins = commands.filter((job) => job.origin);
  const sourceRuns = [...new Set(origins.map((job) => job.origin.run_id))];
  const sourceDirectories = [
    ...new Set(origins.map((job) => job.origin.cwd).filter(Boolean)),
  ];
  section.innerHTML =
    '<div class="table-bar"><h2>Queue</h2><span class="meta">' +
    commands.length +
    (commands.length === 1 ? " job" : " jobs") +
    "</span>" +
    (commands.length ? menu : "") +
    "</div>" +
    (origins.length
      ? '<p class="meta queue-source">Source run: ' +
        esc(sourceRuns.join(", ")) +
        " · Source working directory: " +
        esc(sourceDirectories.join(", ") || "-") +
        "</p>"
      : "") +
    (commands.length ? table : '<div class="empty">Queue is empty.</div>');
  const directories = [
    ...new Set(
      commands.map((job) => job.origin && job.origin.cwd).filter(Boolean),
    ),
  ];
  section.insertAdjacentHTML(
    "beforeend",
    commandGuideHTML(
      "# To execute jobs in current queue\n" +
        (directories.length === 1
          ? "cd " + shellQuote(directories[0]) + "\n"
          : "") +
        "rotari run" +
        (state && state.base_dir ? " -b " + shellQuote(state.base_dir) : " ") +
        " --project-name " +
        shellQuote(queue.project_name),
    ),
  );
  // Each field remembers its first value so Save is enabled only after an
  // edit.
  section.querySelectorAll("tbody tr").forEach((row, index) => {
    row.querySelector(".executor-input").value =
      commands[index].executor || "local";
    row.querySelectorAll("input,select").forEach((field) => {
      field.dataset.initial = field.value;
      field.addEventListener("input", () => updateDirtyField(field));
      field.addEventListener("change", () => updateDirtyField(field));
    });
  });
  return section;
}
// formatDependencies lists a job's prerequisites, marking those that only
// need to finish (depends_on_finished) with a "finished:" prefix, like the CLI.
function formatDependencies(job) {
  return (job.depends_on || [])
    .concat((job.depends_on_finished || []).map((name) => "finished:" + name))
    .join(", ");
}
