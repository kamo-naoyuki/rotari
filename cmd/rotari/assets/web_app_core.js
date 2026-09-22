const executorNames = __ROTARI_EXECUTORS__;
let state;
let projectRuntimeDetailsOpen = false;
let stateJSON = "";
const expandedRunGraphics = {};
let selectedOutput = "";
let selectedLog = null;
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
  return typeof routeParts === "function"
    ? routeParts()
    : location.pathname.split("/").filter(Boolean);
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
function configText(paths) {
  return paths && paths.length ? "\nConfig: " + esc(paths.join(", ")) : "";
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
    return (run && run.context && run.context.config_paths) || [];
  }
  return project.config_path ? [project.config_path] : [];
}
async function showConfig() {
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
  const output = ensureModalOutput();
  output.textContent = (payload.configs || [])
    .map((item) => "# " + item.path + "\n" + item.content)
    .join("\n\n");
  document.querySelector("#output-modal strong").textContent = "Config";
  document.getElementById("output-modal").dataset.view = "config";
  openOutputModal(false);
}
function addConfigButton() {
  document
    .querySelectorAll(".config-button")
    .forEach((button) => button.remove());
  const paths = pageConfigPaths();
  const button = document.createElement("button");
  button.className = "config-button";
  button.textContent = "Config";
  button.disabled = !paths.length;
  button.title = paths.length ? "View config" : "No config file";
  if (paths.length) button.onclick = showConfig;
  document.querySelector(".toolbar").append(button);
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
  document.getElementById("location").textContent =
    state.base_dir +
    " / all projects" +
    configText(state.config_path ? [state.config_path] : []);
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
        '</td><td class="status-' +
        (latest ? latest.status : "") +
        '">' +
        esc(latest ? latest.status : "-") +
        "</td><td>" +
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
  document.getElementById("location").textContent =
    state.base_dir +
    " / " +
    q.project_name +
    configText(q.config_path ? [q.config_path] : []);
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
        '</a></td><td class="status-' +
        r.status +
        '">' +
        esc(r.status) +
        (r.running ? " ..." : "") +
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
    run.context && run.context.config_paths,
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
  const attemptKey = (jobID) => q.project_name + "/" + runID + "/" + jobID;
  const selectAttempt = (job, attemptID) => {
    const attempt = (job.attempts || []).find((item) => item.id === attemptID);
    if (!attempt) return;
    job.attempt_id = attempt.id;
    job.result = attempt.result;
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
      const dependencies = (j.depends_on || []).join(", ");
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
      const diagnoses = (result && result.diagnoses) || [];
      const canDiagnose = !!(
        result &&
        result.exit_code !== 0 &&
        diagnoses.length
      );
      const diagnosisControl = canDiagnose
        ? ' <button class="diagnosis" data-diagnoses="' +
          esc(JSON.stringify(diagnoses)) +
          '" onclick="showDiagnosis(this)">Diagnosis</button>'
        : ' <button class="diagnosis" disabled title="Available after a finalized failed result with saved analysis">Diagnosis</button>';
      const output = result
        ? "<button onclick=\"log('" +
          esc(q.project_name) +
          "','" +
          esc(logRun) +
          "','" +
          esc(logJob) +
          "','" +
          esc(logAttemptID) +
          "')\">Output</button>" +
          diagnosisControl
        : diagnosisControl;
      const jobName = esc(j.name || "-");
      const carriedFrom = carried
        ? '<div class="meta">carried from ' + esc(j.origin.run_id) + "</div>"
        : "";
      const copyIcon = (value, label) =>
        '<button class="command-guide-copy identity-copy" type="button" title="Copy ' +
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
        '"> <strong>' +
        '<span class="identity-line">' +
        jobName +
        jobNameCopy +
        "</span>" +
        '<span class="identity-line">' +
        esc(j.id) +
        jobIDCopy +
        "</span>" +
        '<span class="identity-line">' +
        esc(j.attempt_id || "-") +
        attemptCopy +
        attemptMenu +
        "</span>" +
        carriedFrom +
        "</strong></td><td>" +
        esc(j.executor || "default") +
        "</td><td>" +
        esc(options || "-") +
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
    '</p><pre class="log">Retry from a terminal:\n' +
    esc(copy) +
    "</pre>" +
    (jobs
      ? '<table class="runs"><thead><tr><th data-sort="name"><input id="select-all-jobs" type="checkbox" aria-label="Select all jobs"> job_name / job_id / attempt_id</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="depends">Dependencies</th><th data-sort="working_directory">Working directory</th><th data-sort="command">Command</th><th data-sort="started">Started</th><th data-sort="finished">Finished</th><th data-sort="exit">Exit / error</th><th data-sort="output"></th></tr></thead><tbody>' +
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
      ? '<table class="runs web-queue-jobs"><thead><tr><th data-sort="name">Job name / ID</th><th data-sort="array">Array</th><th data-sort="status">Status</th><th data-sort="executor">Executor</th><th data-sort="options">Executor options</th><th data-sort="depends">Dependencies</th><th data-sort="command">Command</th></tr></thead><tbody>' +
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
              esc((j.depends_on || []).join(", ") || "-") +
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
