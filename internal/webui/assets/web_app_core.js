const executorNames = __ROTARI_EXECUTORS__;
const registeredBasedirs = __ROTARI_BASEDIRS__;
const mountedBasedirID = (() => {
  const parts = location.pathname.split("/").filter(Boolean);
  return parts[0] === "_basedir" ? parts[1] : "";
})();
const sidebarScrollKey =
  "rotari-sidebar-scroll:" +
  (registeredBasedirs.find((item) => item.current)?.id || mountedBasedirID);
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
const expandedRunGraphics = {};
let selectedOutput = "";
let selectedLog = null;
let selectedReportContext = null;
let followTimer = null;
let sidebarScrollActive = false;
let sidebarRenderDeferred = false;
let sidebarScrollTimer = 0;
const selectedAttemptByJob = {};
const openAttemptMenuByJob = {};
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
function render() {
  const queues = state.projects || [];
  renderSidebar(queues);
  const parts = pageParts();
  if (parts[0] === "search") {
    renderHistorySearchPage();
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
  checkbox.title = text;
  const tooltip = checkbox.parentElement.querySelector(
    ".basedir-notification-tooltip",
  );
  if (tooltip) tooltip.textContent = text;
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
        '">' +
        esc(r.run_name || r.run_id) +
        "</a>",
    )
    .join("");
  return runLinks || '<span class="sidebar-run">No runs</span>';
}
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
  const isJobsPage = pageParts()[0] === "jobs";
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
    '"><div class="sidebar-project-row basedir-row"><span class="basedir-notification-control"><input class="basedir-notification-toggle" type="checkbox" data-basedir-id="' +
    entry.id +
    '" aria-label="Monitor notifications for basedir" title="Enable notifications" onchange="toggleNotificationBasedir(this)" /><span class="basedir-notification-tooltip">Enable notifications</span></span><button type="button" class="sidebar-toggle" aria-expanded="' +
    (isExpanded ? "true" : "false") +
    '" aria-label="Toggle projects" onclick="toggleSidebarBasedir(this)"></button><a class="sidebar-project-link' +
    (isActive ? " active" : "") +
    ' basedir-path basedir-switch" data-full-path="' +
    esc(entry.path) +
    '" title="' +
    esc(entry.path) +
    '" href="' +
    basedirURL(entry.id, "/") +
    '">' +
    esc(entry.path) +
    '</a></div><div class="sidebar-projects basedir-contents"' +
    (isExpanded ? ">" : " hidden>") +
    '<div class="sidebar-project all-projects' +
    (allProjectsExpanded ? " expanded" : "") +
    '"><div class="sidebar-project-row"><button type="button" class="sidebar-toggle" aria-expanded="' +
    (allProjectsExpanded ? "true" : "false") +
    '" aria-label="Toggle project list" onclick="toggleSidebarAllProjects(this)"></button><a class="sidebar-project-link basedir-switch' +
    (isActive && !activeProject && !isJobsPage ? " active" : "") +
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
function fitBasedirPaths(sidebar) {
  sidebar.querySelectorAll(".basedir-path[data-full-path]").forEach((link) => {
    if (link.textContent !== link.dataset.fullPath)
      link.textContent = link.dataset.fullPath;
  });
}
function initSidebarResizer() {
  const sidebar = document.querySelector(".sidebar");
  const handle = document.querySelector(".sidebar-resizer");
  if (!sidebar || !handle) return;
  const applySavedWidth = () => {
    if (window.innerWidth <= 760) {
      sidebar.style.width = "";
      return;
    }
    const savedWidth = Number(localStorage.getItem("rotari-sidebar-width"));
    if (Number.isFinite(savedWidth) && savedWidth > 0)
      sidebar.style.width = Math.max(190, Math.min(520, savedWidth)) + "px";
  };
  applySavedWidth();
  window.addEventListener("resize", applySavedWidth);
  const resizeObserver =
    typeof ResizeObserver === "undefined"
      ? null
      : new ResizeObserver(() => fitBasedirPaths(sidebar));
  if (resizeObserver) resizeObserver.observe(sidebar);
  else window.addEventListener("resize", () => fitBasedirPaths(sidebar));
  handle.addEventListener("pointerdown", (event) => {
    event.preventDefault();
    const startX = event.clientX;
    const startWidth = sidebar.getBoundingClientRect().width;
    handle.classList.add("dragging");
    const move = (moveEvent) => {
      const width = Math.max(
        190,
        Math.min(520, startWidth + moveEvent.clientX - startX),
      );
      sidebar.style.width = width + "px";
      localStorage.setItem("rotari-sidebar-width", String(width));
    };
    const end = () => {
      handle.classList.remove("dragging");
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", end);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", end);
  });
  handle.addEventListener("keydown", (event) => {
    if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return;
    event.preventDefault();
    const direction = event.key === "ArrowRight" ? 1 : -1;
    const width = Math.max(
      190,
      Math.min(520, sidebar.getBoundingClientRect().width + direction * 12),
    );
    sidebar.style.width = width + "px";
    localStorage.setItem("rotari-sidebar-width", String(width));
  });
  fitBasedirPaths(sidebar);
}
function renderSidebar(queues) {
  const container = document.getElementById("sidebar-basedirs");
  if (!container) return;
  const sidebar = container.closest(".sidebar");
  const scrollTop = sidebar.scrollTop;
  const parts = pageParts();
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
  fitBasedirPaths(sidebar);
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
function setLocation(path) {
  const location = document.getElementById("location");
  location.replaceChildren(
    path,
    document
      .createRange()
      .createContextualFragment(copyIconForValue(path, "state path")),
  );
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
function isNotificationConfigPath(path) {
  const value = String(path || "");
  const separator = String.fromCodePoint(92);
  const basename = value.slice(
    Math.max(value.lastIndexOf("/"), value.lastIndexOf(separator)) + 1,
  );
  return basename === "notifications.toml";
}
function pageConfigPaths(notifications = false) {
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
  if (parts[2] === "run") {
    const run = project.runs.find(
      (item) => item.run_id === decodeURIComponent(parts[3]),
    );
    return (run?.context?.config_snapshot_paths || []).filter(
      (path) => isNotificationConfigPath(path) === notifications,
    );
  }
  return (
    project.config_sources?.map((source) => source.path) ||
    (project.config_path ? [project.config_path] : [])
  );
}
async function showConfig(notifications = false) {
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
  if (parts[2] === "run") params.set("run_id", decodeURIComponent(parts[3]));
  const response = await fetch("/api/config?" + params);
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  const payload = JSON.parse(text);
  const isRunPage = parts[0] === "project" && parts[2] === "run";
  const files = (payload.configs || []).filter(
    (file) =>
      !isRunPage || isNotificationConfigPath(file.path) === notifications,
  );
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
    if (project === null) {
      delete modal.dataset.editing;
      output.hidden = false;
      output.textContent = "# " + file.path + "\n" + file.content;
      return;
    }
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
  if (isRunPage) {
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
  modal.querySelector("strong").textContent = notifications
    ? "Notification config"
    : "Config";
  modal.dataset.view = "config";
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
  if (parts[2] === "run") return null;
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
  if (project === null) return;
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
      ".config-button,.notification-config-button,.generate-config-button,.notification-generate-config-button",
    )
    .forEach((button) => button.remove());
  const paths = pageConfigPaths();
  const toolbar = document.querySelector(".toolbar");
  const viewButton = document.createElement("button");
  viewButton.className = "config-button";
  viewButton.textContent = "View config";
  viewButton.disabled = !paths.length;
  viewButton.title = paths.length ? "View config" : "No config file";
  if (paths.length) viewButton.onclick = () => showConfig();
  toolbar.insertBefore(viewButton, document.getElementById("refresh-button"));
  if (configGenerationProject() === null) {
    const notificationPaths = pageConfigPaths(true);
    const notificationButton = document.createElement("button");
    notificationButton.className = "notification-config-button";
    notificationButton.textContent = "Notification config";
    notificationButton.disabled = !notificationPaths.length;
    notificationButton.title = notificationPaths.length
      ? "View the notification config copied for this run"
      : "No notification config snapshot";
    notificationButton.onclick = () => showConfig(true);
    toolbar.insertBefore(
      notificationButton,
      document.getElementById("refresh-button"),
    );
    return;
  }
  const generateConfigButton = document.createElement("button");
  generateConfigButton.className = "generate-config-button";
  generateConfigButton.textContent = "Generate config";
  generateConfigButton.title = "Generate or replace a config template";
  generateConfigButton.onclick = showGenerateConfig;
  toolbar.insertBefore(
    generateConfigButton,
    document.getElementById("refresh-button"),
  );
  if (notificationConfigProject() !== null) {
    const notificationButton = document.createElement("button");
    notificationButton.className = "notification-config-button";
    notificationButton.textContent = "Notification config";
    notificationButton.onclick = showNotificationConfig;
    toolbar.insertBefore(
      notificationButton,
      document.getElementById("refresh-button"),
    );
    const generateNotificationButton = document.createElement("button");
    generateNotificationButton.className =
      "notification-generate-config-button";
    generateNotificationButton.textContent = "Generate notification config";
    generateNotificationButton.title =
      "Generate or replace a notifications.toml template";
    generateNotificationButton.onclick = showGenerateNotificationConfig;
    toolbar.insertBefore(
      generateNotificationButton,
      document.getElementById("refresh-button"),
    );
  }
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
  setLocation(webStatePath(state.base_dir, "projects"));
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
        '<tr><td><a class="link" href="/project/' +
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
        '</td><td><span class="status-' +
        (latest ? latest.status : "") +
        '\">' +
        esc(latest ? latest.status : "-") +
        "</span></td><td>" +
        esc(latest ? latest.started_at : "-") +
        "</td></tr>"
      );
    })
    .join("");
  document.getElementById("app").innerHTML = queues.length
    ? '<table class="runs queue-overview"><thead><tr><th data-sort="name">Project</th><th data-sort="queued">Queued</th><th data-sort="runs">Runs</th><th data-sort="running">Running</th><th>Latest run</th><th data-sort="status">Status</th><th data-sort="started">Started</th></tr></thead><tbody>' +
      rows +
      "</tbody></table>"
    : "No projects found.";
}
function renderQueue(q) {
  setLocation(webStatePath(state.base_dir, "projects", q.project_name));
  document.getElementById("page-title").textContent = q.project_name;
  document.getElementById("summary").innerHTML =
    "<span>" +
    (q.queue.commands || []).length +
    " queued</span><span>" +
    q.runs.length +
    " runs</span><span>" +
    q.runs.filter((r) => r.running).length +
    " running</span>";
  const rows = q.runs
    .map(
      (r) =>
        "<tr><td>" +
        esc(r.run_name || "-") +
        '</td><td><a class="link run-id" href="/project/' +
        encodeURIComponent(q.project_name) +
        "/run/" +
        encodeURIComponent(r.run_id) +
        '">' +
        esc(r.run_id) +
        '</a></td><td><span class="status-' +
        esc(r.lifecycle || r.status) +
        '">' +
        esc(r.lifecycle || r.status) +
        (r.running ? " ..." : "") +
        "</span></td><td>" +
        esc(r.client_label || clientStatusLabel(r.client_status)) +
        "</td><td>" +
        (r.finished_at ? esc(r.exit_code) : "-") +
        "</td><td>" +
        esc(r.started_at || "-") +
        "</td><td>" +
        esc(r.finished_at || "-") +
        "</td></tr>",
    )
    .join("");
  document.getElementById("app").innerHTML =
    '<div class="toolbar"><a class="link" href="/">All projects</a></div>' +
    (rows
      ? '<table class="runs"><thead><tr><th data-sort="run_name">run-name</th><th data-sort="run_id">run-id</th><th data-sort="status">Status</th><th data-sort="client">Client</th><th data-sort="exit">Exit</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th></tr></thead><tbody>' +
        rows +
        "</tbody></table>"
      : '<div class="empty">No runs found.</div>');
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
  setLocation(
    webStatePath(state.base_dir, "projects", q.project_name, "runs", runID),
  );
  document.getElementById("page-title").innerHTML = esc(run.run_name || runID);
  document.getElementById("summary").innerHTML =
    "<span>Project: " +
    esc(q.project_name) +
    "</span><span>Run ID: " +
    esc(run.run_id) +
    copyIconForValue(run.run_id, "run ID") +
    "</span><span>Status: " +
    esc(run.status) +
    "</span><span>Lifecycle: " +
    esc(run.lifecycle || "unknown") +
    "</span><span>Client: " +
    esc(run.client_label || clientStatusLabel(run.client_status)) +
    "</span><span>Exit: " +
    (run.finished_at ? esc(run.exit_code) : "-") +
    "</span>";
  if (run.lineage_summary) {
    const lineage = run.lineage_summary;
    const diagnosisText = lineageDiagnosisText(lineage.diagnoses);
    const failureText = (lineage.failures || [])
      .map((group) => group.cause + " " + group.count)
      .join(", ");
    const originText = (lineage.origins || [])
      .map((item) => (item.run_id || "new") + " " + item.count)
      .join(", ");
    document.getElementById("summary").innerHTML +=
      "<span>Jobs: " +
      esc(lineage.counts.jobs) +
      " (ok " +
      esc(lineage.counts.succeeded) +
      ", failed " +
      esc(lineage.counts.failed) +
      ")</span>" +
      (failureText
        ? "<span>Failure causes: " + esc(failureText) + "</span>"
        : "") +
      (diagnosisText
        ? "<span>Diagnoses: " + esc(diagnosisText) + "</span>"
        : "") +
      (originText ? "<span>Origins: " + esc(originText) + "</span>" : "");
  }
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
  const attemptKey = (jobID) => q.project_name + "/" + runID + "/" + jobID;
  const selectAttempt = (job, attemptID) => {
    const attempt = (job.attempts || []).find((item) => item.id === attemptID);
    // The job row already shows the latest attempt with its summary result.
    if (!attempt || attempt === job.attempts[0]) return;
    job.attempt_id = attempt.id;
    job.result = attempt.result;
    job.execution_status = attempt.execution_status;
    job.diagnosis_outdated = false;
    job.submitted_at = attempt.submitted_at;
    job.finished_at = attempt.finished_at;
    job.scheduler_state = attempt.scheduler_state;
  };
  (run.jobs || []).forEach((job) => {
    const selectedAttempt = selectedAttemptByJob[attemptKey(job.id)];
    if (selectedAttempt) selectAttempt(job, selectedAttempt);
  });
  const jobs = (run.jobs || [])
    .map((j) => {
      const result = j.result;
      const options = (j.executor_options || []).join(" ");
      const dependencies = formatDependencies(j);
      const stage = j.stage || "-";
      const exit = result ? esc(result.exit_code) : "-";
      const error =
        result && result.error
          ? '<div class="error">' + esc(result.error) + "</div>"
          : "";
      const carried =
        j.origin &&
        (!j.attempt_id || j.attempt_id === (j.origin.attempt_id || ""));
      const logRun = carried ? j.origin.run_id : runID;
      const logJob = carried ? j.origin.job_id : j.id;
      const logAttemptID = carried ? "" : j.attempt_id;
      const logMode = j.log_mode || "merge";
      const diagnoses = (result && result.diagnoses) || [];
      const diagnosisStatus = (result && result.diagnosis_status) || "";
      const canDiagnose = !!(
        result &&
        result.exit_code !== 0 &&
        (diagnosisStatus || diagnoses.length)
      );
      const diagnosisControl = canDiagnose
        ? ' <button class="diagnosis" data-diagnoses="' +
          esc(
            JSON.stringify({
              status: diagnosisStatus,
              note: result.diagnosis_note || "",
              outdated: !!j.diagnosis_outdated,
              diagnoses: diagnoses,
            }),
          ) +
          '" onclick="showDiagnosis(this)">Diagnosis</button>'
        : ' <button class="diagnosis" disabled title="Available after a finalized failed result with saved analysis">Diagnosis</button>';
      const noteLabels = j.note_labels || [];
      const notesControl = noteLabels.length
        ? ' <button class="view-notes" data-notes="' +
          esc(JSON.stringify(noteLabels)) +
          '" onclick="showNotes(this)">Notes (' +
          noteLabels.length +
          ")</button>"
        : ' <button class="view-notes" disabled title="No notes on this job; add one with rotari note ATTEMPT_ID TEXT">Notes</button>';
      const output = result
        ? '<button class="view-log" onclick="log(\'' +
          esc(q.project_name) +
          "','" +
          esc(logRun) +
          "','" +
          esc(logJob) +
          "','" +
          esc(logAttemptID) +
          "','" +
          esc(logMode) +
          "')\">Output</button>" +
          ' <button class="view-artifacts" onclick="showArtifacts(\'' +
          esc(q.project_name) +
          "','" +
          esc(logRun) +
          "','" +
          esc(logJob) +
          "','" +
          esc(logAttemptID) +
          "')\">Artifacts</button>" +
          diagnosisControl +
          notesControl
        : diagnosisControl + notesControl;
      const jobName = esc(j.name || "-");
      const carriedFrom = carried
        ? '<div class="meta">carried from ' + esc(j.origin.run_id) + "</div>"
        : "";
      const copyIcon = (value, label) =>
        '<button class="table-copy identity-copy" type="button" title="Copy ' +
        label +
        '" aria-label="Copy ' +
        label +
        '" data-copy-value="' +
        esc(value) +
        '" data-copy-title="Copy ' +
        label +
        '" onclick="copyIdentityValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>';
      const jobNameCopy = j.name ? copyIcon(j.name, "job name") : "";
      const jobIDCopy = copyIcon(j.id, "job ID");
      const stageCopy = j.stage ? copyIcon(j.stage, "stage name") : "";
      const attemptCopy = j.attempt_id
        ? copyIcon(j.attempt_id, "attempt ID")
        : "";
      const attemptMenu =
        (j.attempts || []).length > 1
          ? '<details class="attempt-menu"' +
            (openAttemptMenuByJob[attemptKey(j.id)] ? " open" : "") +
            " ontoggle=\"setAttemptMenuOpen('" +
            esc(q.project_name) +
            "','" +
            esc(runID) +
            "','" +
            esc(j.id) +
            '\',this.open)"><summary title="Select attempt" aria-label="Select attempt"><svg view="0 0 24 24" aria-hidden="true"><path d="m7 10 5 5 5-5"></path></svg></summary><div class="attempt-options">' +
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
      const commandText = (j.command || []).join(" ");
      const commandCopy = copyIcon(commandText, "command");
      return (
        '<tr data-job-id="' +
        esc(j.id) +
        '"><td><input class="job-selection" type="checkbox" aria-label="Select ' +
        esc(j.id) +
        '"></td><td><strong>' +
        jobName +
        jobNameCopy +
        carriedFrom +
        "</strong></td><td>" +
        esc(j.id) +
        jobIDCopy +
        "</td><td>" +
        esc(j.attempt_id || "-") +
        attemptCopy +
        attemptMenu +
        "</td><td>" +
        esc(j.executor || "default") +
        "</td><td>" +
        esc(options || "-") +
        "</td><td>" +
        esc(stage) +
        stageCopy +
        "</td><td>" +
        esc(dependencies || "-") +
        "</td><td>" +
        esc(j.working_directory || "-") +
        '</td><td class="command">' +
        esc(commandText) +
        commandCopy +
        "</td><td>" +
        esc(j.submitted_at || "-") +
        "</td><td>" +
        esc(j.finished_at || "-") +
        "</td><td>" +
        exit +
        error +
        "</td><td>" +
        output +
        "</td></tr>"
      );
    })
    .join("");
  const cwd = run.cwd || "-";
  const copy =
    "cd " + shellQuote(cwd) + " && rotari retry -r " + shellQuote(runID) + "";
  const cwdCopy = copyIconForValue(cwd, "working directory");
  document.getElementById("app").innerHTML =
    '<div class="toolbar"><a class="link" href="/project/' +
    encodeURIComponent(q.project_name) +
    '">Back to ' +
    esc(q.project_name) +
    '</a></div><p class="meta run-detail">Started: ' +
    esc(run.started_at || "-") +
    " | Finished: " +
    esc(run.finished_at || "-") +
    " | Jobs: " +
    (run.jobs || []).length +
    '</p><p class="meta run-detail">Working directory: ' +
    "<code>" +
    esc(cwd) +
    "</code>" +
    cwdCopy +
    "</p>" +
    (run.note_labels || [])
      .map(
        (label) =>
          '<p class="meta run-detail run-note">Note: ' + esc(label) + "</p>",
      )
      .join("") +
    (run.source_labels || [])
      .map(
        (label) =>
          '<p class="meta run-detail">Source: <code>' +
          esc(label) +
          "</code></p>",
      )
      .join("") +
    '<pre class="log command-example">Retry from a terminal:\n' +
    esc(copy) +
    "</pre>" +
    (jobs
      ? '<table class="runs"><thead><tr><th><input id="select-all-jobs" type="checkbox" aria-label="Select all jobs"></th><th data-sort="name">Job name</th><th data-sort="id">Job ID</th><th data-sort="attempt">Attempt ID</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="stage">Stage</th><th data-sort="depends">Dependencies</th><th data-sort="working_directory">Working directory</th><th data-sort="command">Command</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="exit">Exit / error</th><th data-sort="output"></th></tr></thead><tbody>' +
        jobs +
        "</tbody></table>"
      : '<div class="empty">No job definitions yet.</div>') +
    '<pre id="log" class="log">Select a job output.</pre>';
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
function selectJobAttempt(projectName, runID, jobID, attemptID) {
  const key = projectName + "/" + runID + "/" + jobID;
  selectedAttemptByJob[key] = attemptID;
  openAttemptMenuByJob[key] = false;
  render();
}
function setAttemptMenuOpen(projectName, runID, jobID, open) {
  openAttemptMenuByJob[projectName + "/" + runID + "/" + jobID] = open;
}
function closeAttemptMenus() {
  document.querySelectorAll(".attempt-menu[open]").forEach((menu) => {
    menu.open = false;
  });
}
document.addEventListener("pointerdown", (event) => {
  if (event.target instanceof Element && event.target.closest(".attempt-menu"))
    return;
  closeAttemptMenus();
});
function renderMissing(message) {
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
async function copyAttemptID(button) {
  try {
    await copyText(button.dataset.attemptId || "");
    button.classList.add("copied");
    button.title = "Copied!";
    button.setAttribute("aria-label", "Copied!");
    button.innerHTML =
      '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"></path></svg>';
    clearTimeout(button.copyResetTimer);
    button.copyResetTimer = setTimeout(() => {
      button.classList.remove("copied");
      button.title = "Copy attempt ID";
      button.setAttribute("aria-label", "Copy attempt ID");
      button.innerHTML =
        '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
    }, 1200);
  } catch (error) {
    alert(error.message);
  }
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
function enhancePage() {
  document
    .querySelectorAll(".web-copy-controls,.web-queue-commands,.web-origin")
    .forEach((e) => e.remove());
  const parts = pageParts();
  if (parts[0] !== "project") return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  if (!queue) return;
  if (parts[2] === "run") {
    const runID = decodeURIComponent(parts[3]);
    const run = queue.runs.find((item) => item.run_id === runID);
    const controls = document.createElement("div");
    controls.className = "toolbar web-copy-controls";
    controls.innerHTML =
      "<button onclick=\"copyRun('" +
      esc(queue.project_name) +
      "','" +
      esc(runID) +
      "','failed')\">Copy failed jobs</button><button onclick=\"copyRun('" +
      esc(queue.project_name) +
      "','" +
      esc(runID) +
      "','all')\">Copy all jobs</button>" +
      (run && run.running
        ? "<button onclick=\"cancelRun('" +
          esc(queue.project_name) +
          "','" +
          esc(runID) +
          "')\">Cancel run</button>"
        : "");
    document.getElementById("app").prepend(controls);
    syncRunControls();
    return;
  }
  const commands = queue.queue.commands || [];
  const section = document.createElement("section");
  section.className = "web-queue-commands";
  section.innerHTML =
    "<h2>Current queue</h2>" +
    (commands.length
      ? '<table class="runs web-queue-jobs"><thead><tr><th data-sort="name">Job name / ID</th><th data-sort="array">Array</th><th data-sort="status">Status</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="stage">Stage</th><th data-sort="depends">Dependencies</th><th data-sort="command">Command</th></tr></thead><tbody>' +
        commands
          .map(
            (j) =>
              "<tr><td><strong>" +
              esc(j.name || "-") +
              '</strong><div class="meta">' +
              esc(j.id) +
              "</div></td><td>" +
              (j.array ? esc(j.array.first + "-" + j.array.last) : "-") +
              '</td><td class="status-value">pending</td><td>' +
              esc(j.executor || "default") +
              "</td><td>" +
              esc((j.executor_options || []).join(" ") || "-") +
              "</td><td>" +
              esc(j.stage || "-") +
              (j.stage ? copyIconForValue(j.stage, "stage name") : "") +
              "</td><td>" +
              esc(formatDependencies(j) || "-") +
              '</td><td class="command">' +
              esc((j.command || []).join(" ")) +
              "</td></tr>",
          )
          .join("") +
        "</tbody></table>"
      : '<div class="empty">Queue is empty.</div>');
  document.getElementById("app").prepend(section);
  fixQueueSourceColumns(commands);
}
// formatDependencies lists a job's prerequisites, marking those that only
// need to finish (depends_on_finished) with a "finished:" prefix, like the CLI.
function formatDependencies(job) {
  return (job.depends_on || [])
    .concat((job.depends_on_finished || []).map((name) => "finished:" + name))
    .join(", ");
}
