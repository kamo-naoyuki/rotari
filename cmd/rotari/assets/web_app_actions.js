function selectedRunJobIDs() {
  return [...document.querySelectorAll(".job-selection:checked")].map(
    (input) => input.closest("tr").dataset.jobId,
  );
}
function updateSelectedRunJobs() {
  const selected = selectedRunJobIDs();
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
  const selectAll = document.getElementById("select-all-jobs");
  if (!selectAll) return;
  selectAll.onchange = () => {
    document
      .querySelectorAll(".job-selection")
      .forEach((input) => (input.checked = selectAll.checked));
    updateSelectedRunJobs();
  };
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
  window.location.href = "/project/" + encodeURIComponent(queue);
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
  window.location.href = "/project/" + encodeURIComponent(queue);
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
function fixQueueSourceColumns(commands) {
  const table = document.querySelector(".web-queue-commands table");
  if (!table) return;
  const sourceRunHeader = document.createElement("th");
  sourceRunHeader.textContent = "Source run";
  sourceRunHeader.dataset.sort = "source_run";
  const sourceStatusHeader = document.createElement("th");
  sourceStatusHeader.textContent = "Source status";
  sourceStatusHeader.dataset.sort = "source_status";
  const sourceStartedHeader = document.createElement("th");
  sourceStartedHeader.textContent = "Source started";
  sourceStartedHeader.dataset.sort = "source_started";
  const sourceFinishedHeader = document.createElement("th");
  sourceFinishedHeader.textContent = "Source finished";
  sourceFinishedHeader.dataset.sort = "source_finished";
  const sourceOutputHeader = document.createElement("th");
  sourceOutputHeader.textContent = "Source output";
  table
    .querySelector("thead tr")
    .append(
      sourceRunHeader,
      sourceStatusHeader,
      sourceStartedHeader,
      sourceFinishedHeader,
      sourceOutputHeader,
    );
  const rows = table.querySelectorAll("tbody tr");
  commands.forEach((job, index) => {
    if (!rows[index]) return;
    const sourceRun = document.createElement("td");
    const sourceStatus = document.createElement("td");
    const sourceStarted = document.createElement("td");
    const sourceFinished = document.createElement("td");
    const sourceOutput = document.createElement("td");
    if (job.origin) {
      sourceRun.textContent = job.origin.run_id + "/" + job.origin.job_id;
      sourceStatus.textContent = job.origin.status;
      sourceStarted.textContent = job.origin.submitted_at || "-";
      sourceFinished.textContent = job.origin.finished_at || "-";
      sourceOutput.innerHTML =
        "<button onclick=\"showOriginalOutput('" +
        esc(job.origin.run_id) +
        "','" +
        esc(job.origin.job_id) +
        "',this)\">Output</button>";
    } else {
      sourceRun.textContent = "-";
      sourceStatus.textContent = "-";
      sourceStarted.textContent = "-";
      sourceFinished.textContent = "-";
      sourceOutput.textContent = "-";
    }
    rows[index].append(
      sourceRun,
      sourceStatus,
      sourceStarted,
      sourceFinished,
      sourceOutput,
    );
  });
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
  host.className = "meta";
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
  guide.className = "command-guide execution-guide";
  const copyButton = document.createElement("button");
  copyButton.className = "command-guide-copy";
  copyButton.type = "button";
  copyButton.title = "Copy command";
  copyButton.setAttribute("aria-label", "Copy command");
  copyButton.innerHTML =
    '<svg viewBox="0 0 24 24" aria-hidden="true"><rect x="4" y="9" width="11" height="11" rx="1"></rect><rect x="9" y="4" width="11" height="11" rx="1"></rect></svg>';
  copyButton.dataset.icon = copyButton.innerHTML;
  copyButton.onclick = () => copyCommandGuide(guide, copyButton);
  const basedir =
    state && state.base_dir ? " -b " + shellQuote(state.base_dir) : " ";
  if (parts[2] === "run") {
    const runID = decodeURIComponent(parts[3]);
    const run = queue.runs.find((item) => item.run_id === runID);
    if (!run) return;
    const latest = queue.runs.reduce(
      (current, item) =>
        !current || item.started_at > current.started_at ? item : current,
      null,
    );
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
  window.location.href = "/project/" + encodeURIComponent(queue);
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
function addPathTableActions() {
  const parts = pageParts();
  if (parts[0] !== "project") return;
  const queueName = decodeURIComponent(parts[1]);
  const queue = state.projects.find((item) => item.project_name === queueName);
  if (!queue) return;
  if (parts[2] === "run") {
    const runID = decodeURIComponent(parts[3]);
    const run = queue.runs.find((item) => item.run_id === runID);
    const table = document.querySelector("#app table.runs");
    if (!run || !table) return;
    const header = document.createElement("th");
    header.textContent = "Actions";
    table.querySelector("thead tr").append(header);
    table.querySelectorAll("tbody tr").forEach((row, index) => {
      const cell = document.createElement("td");
      const job = run.jobs[index];
      if (job)
        addPathButton(
          cell,
          state.base_dir +
            "/projects/" +
            queueName +
            "/runs/" +
            runID +
            "/" +
            job.id,
        );
      row.append(cell);
    });
  } else {
    const runTable = [...document.querySelectorAll("#app table.runs")].find(
      (table) => !table.closest(".web-queue-commands"),
    );
    if (runTable) {
      const header = document.createElement("th");
      header.textContent = "Actions";
      runTable.querySelector("thead tr").append(header);
      runTable.querySelectorAll("tbody tr").forEach((row, index) => {
        const run = queue.runs[index];
        const cell = document.createElement("td");
        if (run) {
          addPathButton(
            cell,
            state.base_dir + "/projects/" + queueName + "/runs/" + run.run_id,
          );
          const deleteButton = document.createElement("button");
          deleteButton.textContent = "Delete";
          deleteButton.onclick = () => deleteRun(queueName, run.run_id);
          cell.append(" ", deleteButton);
        }
        row.append(cell);
      });
    }
  }
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
function rowCell(row, key) {
  const headers = [...row.closest("table").querySelectorAll("thead th")];
  const index = headers.findIndex((header) => header.dataset.sort === key);
  return index < 0 ? null : row.children[index];
}
function ensureQueueWorkingDirectoryColumn(commands) {
  const table = document.querySelector(".web-queue-commands table");
  if (!table) return;
  const headerRow = table.querySelector("thead tr");
  if (!headerRow || headerRow.querySelector('[data-sort="working_directory"]'))
    return;
  const header = document.createElement("th");
  header.dataset.sort = "working_directory";
  header.textContent = "Working directory";
  const commandHeader = headerRow.querySelector('[data-sort="command"]');
  if (commandHeader) headerRow.insertBefore(header, commandHeader);
  else headerRow.append(header);
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const cell = document.createElement("td");
    cell.textContent =
      (commands[index] && commands[index].working_directory) || "-";
    const commandCell = rowCell(row, "command");
    if (commandCell) row.insertBefore(cell, commandCell);
    else row.append(cell);
  });
}
function addQueueEditors(queue, commands) {
  const table = document.querySelector(".web-queue-commands table");
  if (!table) return;
  const actionHeader = document.createElement("th");
  actionHeader.textContent = "Actions";
  table.querySelector("thead tr").append(actionHeader);
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const job = commands[index];
    if (!job) return;
    rowCell(row, "name").innerHTML =
      '<input class="job-name-input" value="' +
      esc(job.name || "") +
      '"><div class="meta">' +
      esc(job.id) +
      "</div>";
    rowCell(row, "executor").innerHTML =
      '<select class="executor-input">' +
      executorNames
        .map(
          (name) =>
            '<option value="' + esc(name) + '">' + esc(name) + "</option>",
        )
        .join("") +
      "</select>";
    rowCell(row, "executor").querySelector("select").value =
      job.executor || "local";
    rowCell(row, "options").innerHTML =
      '<input class="executor-option-input" value="' +
      esc(JSON.stringify(job.executor_options || [])) +
      '">';
    rowCell(row, "depends").innerHTML =
      '<input class="depends-input" value="' +
      esc(JSON.stringify(job.depends_on || [])) +
      '">';
    rowCell(row, "command").innerHTML =
      '<input class="command-input" value="' +
      esc(JSON.stringify(job.command)) +
      '">';
    rowCell(row, "working_directory").innerHTML =
      '<input class="working-directory-input" placeholder="working directory" value="' +
      esc(job.working_directory || "") +
      '">';
    row.querySelectorAll("input,select").forEach((field) => {
      field.dataset.initial = field.value;
      field.addEventListener("input", () => updateDirtyField(field));
      field.addEventListener("change", () => updateDirtyField(field));
    });
    const save = document.createElement("button");
    save.className = "save-job";
    save.textContent = "Save";
    save.disabled = true;
    save.onclick = () => saveQueueJob(queue.project_name, job.id, row);
    const remove = document.createElement("button");
    remove.textContent = "Remove";
    remove.onclick = () =>
      removeQueueJob(queue.project_name, job.id, job.name || job.id);
    const actions = document.createElement("td");
    actions.append(save, " ", remove);
    row.append(actions);
  });
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
