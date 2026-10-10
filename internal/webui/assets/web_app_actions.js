const selectedRunJobsByRun = {};
let runJobControlBusy = false;
function selectedRunKey() {
  const parts = pageParts();
  return parts[0] === "project" && parts[2] === "run"
    ? decodeURIComponent(parts[1]) + "/" + decodeURIComponent(parts[3])
    : "";
}
function selectedRunJobIDs() {
  return [...document.querySelectorAll(".job-selection:checked")].map(
    (input) => input.closest("tr").dataset.jobId,
  );
}
function selectedRunJobs() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return [];
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    project &&
    project.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  if (!run) return [];
  const selected = new Set(selectedRunJobIDs());
  return (run.jobs || [])
    .filter((job) => selected.has(job.id))
    .map((job) => ({
      job,
      status: jobDisplayStatus(job, run),
      run,
      project,
    }));
}
function updateSelectedRunControlButtons() {
  const selected = selectedRunJobs();
  const hasRunning = selected.some((item) => item.status === "running");
  const hasSuspended = selected.some((item) => item.status === "suspended");
  const cancel = document.querySelector(".cancel-selected-jobs");
  const suspendResume = document.querySelector(".suspend-resume-selected-jobs");
  if (cancel) cancel.disabled = runJobControlBusy || !hasRunning;
  if (suspendResume) {
    suspendResume.disabled =
      runJobControlBusy || (!hasRunning && !hasSuspended);
    const operation = hasRunning || !hasSuspended ? "suspend" : "resume";
    suspendResume.textContent =
      operation === "suspend" ? "Suspend selected" : "Resume selected";
    suspendResume.dataset.operation = operation;
  }
}
async function controlSelectedRunJobs(operation) {
  if (runJobControlBusy) return;
  const selected = selectedRunJobs();
  const hasRunning = selected.some((item) => item.status === "running");
  if (operation === "suspend-resume")
    operation = hasRunning ? "suspend" : "resume";
  let targets;
  let endpoint;
  if (operation === "cancel") {
    if (!hasRunning) return;
    targets = selected.filter((item) =>
      ["pending", "running", "suspended"].includes(item.status),
    );
    endpoint = "/api/cancel-job";
    if (!confirm("Cancel " + targets.length + " selected jobs?")) return;
  } else if (operation === "suspend") {
    if (!hasRunning) return;
    targets = selected.filter((item) => item.status === "running");
    endpoint = "/api/suspend-job";
  } else if (operation === "resume") {
    if (hasRunning) return;
    targets = selected.filter((item) => item.status === "suspended");
    endpoint = "/api/resume-job";
  } else {
    return;
  }
  if (!targets.length) return;
  runJobControlBusy = true;
  updateSelectedRunControlButtons();
  const { project, run } = targets[0];
  try {
    const response = await fetch(endpoint, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        project_name: project.project_name,
        run_id: run.run_id,
        job_ids: targets.map((item) => item.job.id),
      }),
    });
    const text = await response.text();
    if (!response.ok) alert(text);
    await refresh();
  } finally {
    runJobControlBusy = false;
    updateSelectedRunControlButtons();
  }
}
function restoreSelectedRunJobs() {
  const selected = selectedRunJobsByRun[selectedRunKey()];
  if (!selected) return;
  document.querySelectorAll(".job-selection").forEach((input) => {
    input.checked = selected.has(input.closest("tr").dataset.jobId);
  });
}
function updateSelectedRunJobs() {
  const selected = selectedRunJobIDs();
  const key = selectedRunKey();
  if (key) selectedRunJobsByRun[key] = new Set(selected);
  const all = [...document.querySelectorAll(".job-selection")];
  const selectAll = document.getElementById("select-all-jobs");
  if (selectAll)
    selectAll.checked = all.length > 0 && selected.length === all.length;
  document
    .querySelectorAll(".create-selected,.append-selected")
    .forEach((button) => (button.disabled = selected.length === 0));
  document
    .querySelectorAll(".unselect-all")
    .forEach((button) => (button.disabled = selected.length === 0));
  document
    .querySelectorAll(".run-ai")
    .forEach((button) => (button.disabled = selected.length === 0));
  updateSelectedRunControlButtons();
  const failed = document.querySelector(".select-failed");
  const failedUnfinished = document.querySelector(".select-failed-unfinished");
  if (failed || failedUnfinished) {
    const parts = pageParts();
    const project = state.projects.find(
      (item) => item.project_name === decodeURIComponent(parts[1]),
    );
    const run =
      project &&
      project.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
    const selectable = (run && run.jobs ? run.jobs : []).filter((job) => {
      const status = jobDisplayStatus(job, run);
      return (
        status === "failed" ||
        status === "pending" ||
        status === "running" ||
        status === "suspended"
      );
    });
    if (failed)
      failed.disabled = !((run && run.jobs) || []).some(
        (job) => jobDisplayStatus(job, run) === "failed",
      );
    if (failedUnfinished) failedUnfinished.disabled = selectable.length === 0;
  }
}
function addRunJobSelection() {
  if (!document.querySelector(".job-selection")) return;
  restoreSelectedRunJobs();
  document
    .querySelectorAll(".job-selection")
    .forEach((input) => (input.onchange = updateSelectedRunJobs));
  updateSelectedRunJobs();
}
async function copySelectedJobs(queue, run, append) {
  const jobIDs = selectedRunJobIDs();
  if (!jobIDs.length) return;
  const q = state.projects.find((item) => item.project_name === queue);
  const existing = ((q && q.queue.commands) || []).length;
  if (
    !append &&
    existing &&
    !confirm("This will replace " + existing + " queued jobs. Continue?")
  )
    return;
  const response = await fetch("/api/copy", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      project_name: queue,
      run_id: run,
      job_ids: jobIDs,
      selection: "job-id",
      append: append,
      overwrite: !append && existing > 0,
    }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  alert(JSON.parse(text).message);
  window.location.href = appURL("/project/" + encodeURIComponent(queue));
}
function selectJobsByStatus(statuses) {
  const parts = pageParts();
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    project &&
    project.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  if (!run) return;
  (run.jobs || []).forEach((job) => {
    const row = [...document.querySelectorAll("tr[data-job-id]")].find(
      (item) => item.dataset.jobId === job.id,
    );
    const status = jobDisplayStatus(job, run);
    if (row)
      row.querySelector(".job-selection").checked = statuses.includes(status);
  });
  updateSelectedRunJobs();
}
function selectFailedJobs() {
  selectJobsByStatus(["failed"]);
}
function selectFailedUnfinishedJobs() {
  selectJobsByStatus(["failed", "pending", "running", "suspended"]);
}
function clearSelectedJobs() {
  document
    .querySelectorAll(".job-selection,#select-all-jobs")
    .forEach((input) => (input.checked = false));
  updateSelectedRunJobs();
}
function syncRunControls() {
  const controls = document.querySelector(".web-copy-controls");
  if (!controls) return;
  const parts = pageParts();
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const runID = decodeURIComponent(parts[3]);
  const buttons = controls.querySelectorAll("button");
  if (buttons.length >= 2) {
    buttons[0].outerHTML =
      '<button class="create-selected" disabled onclick="copySelectedJobs(\'' +
      esc(project.project_name) +
      "','" +
      esc(runID) +
      "',false)\">Create</button>";
    buttons[1].outerHTML =
      '<button class="append-selected" disabled onclick="copySelectedJobs(\'' +
      esc(project.project_name) +
      "','" +
      esc(runID) +
      "',true)\">Append</button>";
  }
  if (!controls.querySelector(".select-failed")) {
    const select = document.createElement("button");
    select.className = "select-failed";
    select.textContent = "Select failed";
    select.onclick = selectFailedJobs;
    controls.append(select);
  }
  if (!controls.querySelector(".select-failed-unfinished")) {
    const select = document.createElement("button");
    select.className = "select-failed-unfinished";
    select.textContent = "Select failed + unfinished";
    select.onclick = selectFailedUnfinishedJobs;
    const clear = document.createElement("button");
    clear.textContent = "Clear selection";
    clear.onclick = clearSelectedJobs;
    controls.append(select, clear);
  }
}
async function copyRun(queue, run, selection) {
  const q = state.projects.find((item) => item.project_name === queue);
  const existing = ((q && q.queue.commands) || []).length;
  if (
    existing &&
    !confirm("This will replace " + existing + " queued jobs. Continue?")
  )
    return;
  const response = await fetch("/api/copy", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      project_name: queue,
      run_id: run,
      selection: selection,
      overwrite: existing > 0,
    }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  alert(JSON.parse(text).message);
  window.location.href = appURL("/project/" + encodeURIComponent(queue));
}
async function cancelRun(queue, run) {
  if (!confirm("Cancel this run and all running jobs?")) return;
  const response = await fetch("/api/cancel-run", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ project_name: queue, run_id: run }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  await refresh();
}
// queuedStatusText mirrors model.QueuedStatusText: a status the job was
// marked with replaces its source status.
function queuedStatusText(recorded, marked) {
  if (!marked || marked === recorded) return recorded || "-";
  if (marked === "success") return "success (accepted)";
  return marked + " (marked)";
}
function enhanceQueueSourceContext(commands) {
  const section = document.querySelector(".web-queue-commands");
  if (!section) return;
  const origins = commands.filter((job) => job.origin);
  if (!origins.length) return;
  const runs = [...new Set(origins.map((job) => job.origin.run_id))];
  const cwds = [
    ...new Set(origins.map((job) => job.origin.cwd).filter(Boolean)),
  ];
  const context = document.createElement("p");
  context.className = "meta";
  context.textContent =
    "Source run: " +
    runs.join(", ") +
    " | Source working directory: " +
    (cwds.join(", ") || "-");
  section.prepend(context);
}
function addRunHostLine() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const hostname = run && run.context && run.context.hostname;
  if (!hostname) return;
  const workingDirectory = [...document.querySelectorAll("#app p")].find(
    (element) => element.textContent.startsWith("Working directory:"),
  );
  if (!workingDirectory) return;
  const host = document.createElement("p");
  host.className = "meta run-detail";
  host.textContent = "Host: " + hostname;
  workingDirectory.before(host);
}
function addExecutionGuide() {
  document
    .querySelectorAll(".execution-guide")
    .forEach((element) => element.remove());
  const parts = pageParts();
  if (parts[0] !== "project") return;
  const queueName = decodeURIComponent(parts[1]);
  const queue = state.projects.find((item) => item.project_name === queueName);
  if (!queue) return;
  const guide = document.createElement("pre");
  guide.className = "command-guide execution-guide command-example";
  const copyButton = document.createElement("button");
  copyButton.className = "command-guide-copy";
  copyButton.type = "button";
  copyButton.title = "Copy command";
  copyButton.setAttribute("aria-label", "Copy command");
  copyButton.innerHTML =
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M9 9H5a1 1 0 0 0-1 1v9a1 1 0 0 0 1 1h9a1 1 0 0 0 1-1v-4"></path><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
  copyButton.dataset.icon = copyButton.innerHTML;
  copyButton.onclick = () => copyCommandGuide(guide, copyButton);
  const basedir =
    state && state.base_dir ? " -b " + shellQuote(state.base_dir) : " ";
  if (parts[2] === "run") {
    const runID = decodeURIComponent(parts[3]);
    const run = queue.runs.find((item) => item.run_id === runID);
    if (!run) return;
    const latest = latestRun(queue.runs);
    const prefix =
      run.cwd && run.cwd !== "-" ? "cd " + shellQuote(run.cwd) + "\n" : "";
    if (latest && latest.run_id === runID) {
      guide.textContent =
        "# To retry failed or unfinished jobs from this run\n" +
        prefix +
        "rotari retry -r " +
        shellQuote(runID) +
        "";
    } else {
      guide.textContent =
        "# To retry failed or unfinished jobs from this run\n" +
        prefix +
        "rotari retry -r " +
        shellQuote(runID) +
        "\nrotari retry -r " +
        shellQuote(runID) +
        " --job-id JOB_ID";
    }
    addCommandGuideCopyButton(guide, copyButton);
    const workingDirectory = [...document.querySelectorAll("#app p")].find(
      (element) => element.textContent.startsWith("Working directory:"),
    );
    if (workingDirectory) workingDirectory.after(guide);
    else document.getElementById("app").prepend(guide);
    return;
  }
  const origins = (queue.queue.commands || [])
    .map((job) => job.origin)
    .filter(Boolean);
  const directories = [
    ...new Set(origins.map((origin) => origin.cwd).filter(Boolean)),
  ];
  const prefix =
    directories.length === 1 ? "cd " + shellQuote(directories[0]) + "\n" : "";
  guide.textContent =
    "# To execute jobs in current queue\n" +
    prefix +
    "rotari run" +
    basedir +
    " --project-name " +
    shellQuote(queueName);
  addCommandGuideCopyButton(guide, copyButton);
  const section = document.querySelector(".web-queue-commands");
  if (section) section.append(guide);
}
async function copyCommandGuide(guide, button) {
  try {
    await copyText(guide.dataset.copyText || "");
    clearTimeout(button.copyResetTimer);
    guide.classList.add("copied");
    button.classList.add("copied");
    button.title = "Copied!";
    button.setAttribute("aria-label", "Copied!");
    button.innerHTML =
      '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"></path></svg>';
    button.copyResetTimer = setTimeout(() => {
      guide.classList.remove("copied");
      button.classList.remove("copied");
      button.title = "Copy command";
      button.setAttribute("aria-label", "Copy command");
      button.innerHTML = button.dataset.icon;
    }, 1200);
  } catch (error) {
    alert(error.message);
  }
}
function addCommandGuideCopyButton(guide, button) {
  guide.dataset.copyText = guide.textContent;
  const copied = document.createElement("span");
  copied.className = "command-guide-copied";
  copied.setAttribute("role", "status");
  copied.textContent = "Copied!";
  guide.append(copied);
  guide.append(button);
}
function addPathButton(cell, path) {
  const button = document.createElement("button");
  button.className = "show-path";
  button.textContent = "Path";
  button.onclick = () => showPath(path, button);
  cell.append(" ", button);
}
function addDeleteRunButton() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const controls = document.querySelector(".web-copy-controls");
  if (!controls) return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const button = document.createElement("button");
  button.className = "delete-run";
  button.textContent = "Delete run";
  button.disabled = !!(run && run.running);
  button.title = button.disabled
    ? "Running runs cannot be deleted"
    : "Delete run history";
  button.onclick = () =>
    deleteRun(decodeURIComponent(parts[1]), decodeURIComponent(parts[3]));
  controls.append(button);
}
async function deleteRun(queue, run) {
  if (!confirm("Delete run history " + run + "?")) return;
  const response = await fetch("/api/clear-run", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ project_name: queue, run_id: run }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  window.location.href = appURL("/project/" + encodeURIComponent(queue));
}
function isCompactOutput(value) {
  return value.length < 1200 && value.split("\n").length <= 18;
}
function showPath(path) {
  selectedOutput = path;
  const output = ensureModalOutput();
  output.textContent = path;
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "path";
  modal.querySelector("strong").textContent = "Job path";
  openOutputModal(true);
}
function updateDirtyField(field) {
  field.classList.toggle("dirty", field.value !== field.dataset.initial);
  updateRowSaveState(field.closest("tr"));
}
function updateRowSaveState(row) {
  if (!row) return;
  const save = row.querySelector(".save-job");
  if (!save) return;
  save.disabled = [...row.querySelectorAll("input,select")].every(
    (field) => field.value === field.dataset.initial,
  );
}
async function saveQueueJob(queue, jobID, row) {
  const parse = (selector, label) => {
    try {
      const value = JSON.parse(row.querySelector(selector).value);
      if (!Array.isArray(value)) throw new Error(label + " must be an array");
      return value;
    } catch (error) {
      throw new Error(label + ": " + error.message);
    }
  };
  let command, options, depends;
  try {
    command = parse(".command-input", "command");
    options = parse(".executor-option-input", "Executor options");
    depends = parse(".depends-input", "dependencies");
    if (!command.length) throw new Error("command must not be empty");
  } catch (error) {
    alert(error.message);
    return;
  }
  const response = await fetch("/api/change", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({
      project_name: queue,
      job_id: jobID,
      set_job_name: row.querySelector(".job-name-input").value,
      command: command,
      executor: row.querySelector(".executor-input").value,
      executor_options: options,
      clear_executor_options: options.length === 0,
      depends_on: depends,
      clear_depends_on: depends.length === 0,
      working_directory: row.querySelector(".working-directory-input").value,
      clear_working_directory:
        row.querySelector(".working-directory-input").value === "",
    }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  await refresh();
}
async function removeQueueJob(queue, jobID, label) {
  if (!confirm("Remove " + label + " from the queue?")) return;
  const response = await fetch("/api/remove", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ project_name: queue, job_id: jobID }),
  });
  const text = await response.text();
  if (!response.ok) {
    alert(text);
    return;
  }
  await refresh();
}
