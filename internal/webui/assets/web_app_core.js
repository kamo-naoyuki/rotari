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
const expandedRunGraphics = {};
let selectedOutput = "";
let selectedLog = null;
let selectedReportContext = null;
let followTimer = null;
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
async function refresh() {
  if (
    document.activeElement &&
    document.activeElement.closest(
      ".web-queue-commands input,.web-queue-commands select",
    )
  )
    return;
  let r;
  try {
    r = await fetch("/api/state");
  } catch (error) {
    document.getElementById("disconnect-banner").classList.add("show");
    return;
  }
  document.getElementById("disconnect-banner").classList.remove("show");
  if (!r.ok) {
    document.getElementById("app").textContent = await r.text();
    return;
  }
  const nextState = await r.json();
  const nextStateJSON = JSON.stringify(nextState);
  if (nextStateJSON === stateJSON) return;
  stateJSON = nextStateJSON;
  state = nextState;
  render();
  if (typeof rewriteStaticLinks === "function") rewriteStaticLinks();
}
function render() {
  const queues = state.projects || [];
  renderSidebar(queues);
  const parts = pageParts();
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
    });
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
      if (projectActive) expandedSidebarProjects[key] = true;
      const expanded = projectActive || !!expandedSidebarProjects[key];
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
    '"><div class="basedir-notification-row"><label><input class="basedir-notification-toggle" type="checkbox" data-basedir-id="' +
    entry.id +
    '" aria-label="Monitor notifications for basedir" onchange="toggleNotificationBasedir(this)" />Monitor notifications</label></div><div class="sidebar-project-row basedir-row"><button type="button" class="sidebar-toggle" aria-expanded="' +
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
      () => sessionStorage.setItem(sidebarScrollKey, String(sidebar.scrollTop)),
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
  expandedSidebarProjects[key] =
    button.getAttribute("aria-expanded") !== "true";
  renderSidebar(state ? state.projects || [] : []);
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
function setLocation(base, paths) {
  const location = document.getElementById("location");
  location.textContent = base;
  if (!paths || !paths.length) return;
  location.append("\nConfig: ");
  paths.forEach((path, index) => {
    if (index) location.append(", ");
    const copy = document
      .createRange()
      .createContextualFragment(copyIconForValue(path, "config path"));
    location.append(path, copy);
  });
}
function pageConfigPaths() {
  const parts = pageParts();
  if (parts[0] !== "project")
    return state.config_path ? [state.config_path] : [];
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  if (!project) return [];
  if (parts[2] === "run") {
    const run = project.runs.find(
      (item) => item.run_id === decodeURIComponent(parts[3]),
    );
    return (run && run.context && run.context.config_snapshot_paths) || [];
  }
  return project.config_path ? [project.config_path] : [];
}
async function showConfig() {
  const modal = document.getElementById("output-modal");
  const generator = document.getElementById("config-generator");
  const editor = document.getElementById("config-editor");
  const output = ensureModalOutput();
  generator.hidden = true;
  editor.hidden = true;
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
  const files = payload.configs || [];
  const content = files
    .map((item) => "# " + item.path + "\n" + item.content)
    .join("\n\n");
  selectedLog = null;
  selectedOutput = content;
  const project = configGenerationProject();
  if (project === null) {
    delete modal.dataset.editing;
    output.textContent = content;
  } else {
    const file = files[0];
    if (!file) {
      alert("No config file exists to edit.");
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
      const button = editor.querySelector("button");
      button.disabled = true;
      const response = await fetch("/api/save-config", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          project_name: project,
          content: textarea.value,
        }),
      });
      const saveText = await response.text();
      button.disabled = false;
      if (!response.ok) {
        alert(saveText);
        return;
      }
      textarea.dataset.initial = textarea.value;
      updateConfigSaveState(editor);
      selectedOutput = textarea.value;
      await refresh();
      alert("Saved " + JSON.parse(saveText).path);
    };
    modal.dataset.editing = "true";
  }
  modal.querySelector("strong").textContent = "Config";
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
function showGenerateConfig() {
  const project = configGenerationProject();
  const modal = document.getElementById("output-modal");
  const output = ensureModalOutput();
  const generator = document.getElementById("config-generator");
  const editor = document.getElementById("config-editor");
  output.hidden = true;
  editor.hidden = true;
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
      generator.replaceChildren(
        Object.assign(document.createElement("p"), {
          textContent: "Choose where to generate config.toml.",
        }),
      );
      const options = document.createElement("div");
      options.className = "config-target-options";
      (payload.targets || []).forEach((target) => {
        const button = document.createElement("button");
        button.type = "button";
        button.textContent = target.path;
        button.title = "Generate config.toml in " + target.location;
        button.onclick = async () => {
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
        };
        options.append(button);
      });
      generator.append(options);
    })
    .catch((error) => {
      generator.textContent =
        "Failed to load config locations: " + error.message;
    });
  modal.querySelector("strong").textContent = "Generate config";
  modal.dataset.view = "generate-config";
  openOutputModal(true);
}
function addConfigButton() {
  document
    .querySelectorAll(".config-button,.generate-config-button")
    .forEach((button) => button.remove());
  const paths = pageConfigPaths();
  const viewButton = document.createElement("button");
  viewButton.className = "config-button";
  viewButton.textContent = "View config";
  viewButton.disabled = !paths.length;
  viewButton.title = paths.length ? "View config" : "No config file";
  if (paths.length) viewButton.onclick = showConfig;
  const toolbar = document.querySelector(".toolbar");
  toolbar.insertBefore(viewButton, document.getElementById("notify-toggle"));
  if (configGenerationProject() !== null) {
    const generateButton = document.createElement("button");
    generateButton.className = "generate-config-button";
    generateButton.textContent = "Generate config";
    generateButton.title = "Generate or replace a config template";
    generateButton.onclick = showGenerateConfig;
    toolbar.insertBefore(
      generateButton,
      document.getElementById("notify-toggle"),
    );
  }
}
function renderOverview(queues) {
  let queued = 0,
    runs = 0,
    running = 0;
  queues.forEach((q) => {
    queued += (q.queue.commands || []).length;
    runs += q.runs.length;
    running += q.runs.filter((r) => r.running).length;
  });
  setLocation(
    state.base_dir + " / all projects",
    state.config_path ? [state.config_path] : [],
  );
  document.getElementById("page-title").textContent = "All projects";
  document.getElementById("summary").innerHTML =
    "<span>" +
    queues.length +
    " projects</span><span>" +
    queued +
    " queued</span><span>" +
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
        (q.queue.commands || []).length +
        "</td><td>" +
        q.runs.length +
        "</td><td>" +
        q.runs.filter((r) => r.running).length +
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
  setLocation(
    state.base_dir + " / " + q.project_name,
    q.config_path ? [q.config_path] : [],
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
        r.status +
        '">' +
        esc(r.status) +
        (r.running ? " ..." : "") +
        "</span></td><td>" +
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
      ? '<table class="runs"><thead><tr><th data-sort="run_name">run-name</th><th data-sort="run_id">run-id</th><th data-sort="status">Status</th><th data-sort="exit">Exit</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th></tr></thead><tbody>' +
        rows +
        "</tbody></table>"
      : '<div class="empty">No runs found.</div>');
}
function renderRun(q, runID) {
  const run = q.runs.find((r) => r.run_id === runID);
  if (!run) {
    renderMissing("Run not found");
    return;
  }
  setLocation(
    state.base_dir + " / " + q.project_name + " / " + runID,
    run.context && run.context.config_snapshot_paths,
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
    "</span><span>Exit: " +
    (run.finished_at ? esc(run.exit_code) : "-") +
    "</span>";
  if (run.unreadable) {
    // The run's files come from a newer rotari; only the reason is known.
    document.getElementById("app").innerHTML =
      '<div class="empty error">' + esc(run.unreadable) + "</div>";
    return;
  }
  const attemptKey = (jobID) => q.project_name + "/" + runID + "/" + jobID;
  const selectAttempt = (job, attemptID) => {
    const attempt = (job.attempts || []).find((item) => item.id === attemptID);
    // The job row already shows the latest attempt with its summary result.
    if (!attempt || attempt === job.attempts[0]) return;
    job.attempt_id = attempt.id;
    job.result = attempt.result;
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
          diagnosisControl
        : diagnosisControl;
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
        '" onclick="copyIdentityValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>';
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
    '</a></div><p class="meta">Started: ' +
    esc(run.started_at || "-") +
    " | Finished: " +
    esc(run.finished_at || "-") +
    " | Jobs: " +
    (run.jobs || []).length +
    "</p><p>Working directory: " +
    "<code>" +
    esc(cwd) +
    "</code>" +
    cwdCopy +
    '</p><pre class="log command-example">Retry from a terminal:\n' +
    esc(copy) +
    "</pre>" +
    (jobs
      ? '<table class="runs"><thead><tr><th><input id="select-all-jobs" type="checkbox" aria-label="Select all jobs"></th><th data-sort="name">Job name</th><th data-sort="id">Job ID</th><th data-sort="attempt">Attempt ID</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="stage">Stage</th><th data-sort="depends">Dependencies</th><th data-sort="working_directory">Working directory</th><th data-sort="command">Command</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="exit">Exit / error</th><th data-sort="output"></th></tr></thead><tbody>' +
        jobs +
        "</tbody></table>"
      : '<div class="empty">No job definitions yet.</div>') +
    '<pre id="log" class="log">Select a job output.</pre>';
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
    '" onclick="copyIdentityValue(this)"><svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg></button>'
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
        '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
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
        '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
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
