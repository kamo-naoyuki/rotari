async function loadLogChunk(queue, run, job, before) {
  const response = await fetch(
    "/api/log?project_name=" +
      encodeURIComponent(queue) +
      "&run_id=" +
      encodeURIComponent(run) +
      "&job_id=" +
      encodeURIComponent(job) +
      "&tail=200&before=" +
      before,
  );
  return response.text();
}
function attachLogLoader(output) {
  output.onscroll = async () => {
    if (
      output.scrollTop > 20 ||
      !selectedLog ||
      selectedLog.loading ||
      selectedLog.done
    )
      return;
    if (followTimer) clearInterval(followTimer);
    followTimer = null;
    selectedLog.loading = true;
    const previousHeight = output.scrollHeight;
    const chunk = await loadLogChunk(
      selectedLog.queue,
      selectedLog.run,
      selectedLog.job,
      selectedLog.before + 200,
    );
    if (!chunk) {
      selectedLog.done = true;
    } else {
      selectedLog.before += 200;
      output.textContent = chunk + selectedOutput;
      selectedOutput = output.textContent;
      openOutputModal(isCompactOutput(selectedOutput));
      output.scrollTop = output.scrollHeight - previousHeight;
    }
    selectedLog.loading = false;
  };
}
async function showOriginalOutput(run, job, trigger) {
  const parts = pageParts();
  await showLog(
    decodeURIComponent(parts[1]),
    run,
    job,
    trigger || (window.event && window.event.currentTarget),
  );
}
async function showLog(queue, run, job) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "log";
  modal.querySelector("strong").textContent = "Job log";
  if (followTimer) clearInterval(followTimer);
  selectedLog = {
    queue: queue,
    run: run,
    job: job,
    before: 0,
    loading: false,
    done: false,
  };
  selectedOutput = await loadLogChunk(queue, run, job, 0);
  const output = ensureModalOutput();
  output.textContent = selectedOutput;
  openOutputModal(isCompactOutput(selectedOutput));
  output.scrollTop = output.scrollHeight;
  attachLogLoader(output);
  followTimer = setInterval(followOutput, 2000);
}
function showDiagnosis(trigger) {
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  const diagnoses = JSON.parse(trigger.dataset.diagnoses || "[]");
  selectedOutput = diagnoses
    .map(
      (item) =>
        item.name +
        "\nEvidence: " +
        item.evidence +
        "\nNext: " +
        item.suggestion,
    )
    .join("\n\n");
  const output = ensureModalOutput();
  output.textContent = selectedOutput;
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "diagnosis";
  modal.querySelector("strong").textContent = "Diagnosis";
  openOutputModal(true);
}
async function followOutput() {
  if (!selectedLog || selectedLog.before > 0 || selectedLog.loading) return;
  const latest = await loadLogChunk(
    selectedLog.queue,
    selectedLog.run,
    selectedLog.job,
    0,
  );
  if (latest && latest !== selectedOutput) {
    selectedOutput = latest;
    const output = ensureModalOutput();
    output.textContent = latest;
    output.scrollTop = output.scrollHeight;
    openOutputModal(isCompactOutput(latest));
  }
}
async function log(queue, run, job, trigger) {
  await showLog(
    queue,
    run,
    job,
    trigger || (window.event && window.event.currentTarget),
  );
}
function lastLogLines(value, count) {
  const lines = String(value || "").split("\n");
  return lines.slice(Math.max(0, lines.length - count)).join("\n");
}
async function copyText(value) {
  if (navigator.clipboard && navigator.clipboard.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const area = document.createElement("textarea");
  area.value = value;
  area.style.position = "fixed";
  area.style.opacity = "0";
  document.body.append(area);
  area.select();
  document.execCommand("copy");
  area.remove();
}
function copied(button) {
  const status = button
    .closest(".output-box")
    ?.querySelector(".modal-copy-status");
  clearTimeout(button.copyResetTimer);
  button.classList.add("copied");
  if (status) status.classList.add("visible");
  button.title = "Copied!";
  button.setAttribute("aria-label", "Copied!");
  button.innerHTML =
    '<svg viewBox="0 0 24 24" aria-hidden="true"><path d="m5 12 4 4L19 6"></path></svg>';
  button.copyResetTimer = setTimeout(() => {
    button.classList.remove("copied");
    if (status) status.classList.remove("visible");
    button.title = button.dataset.copyTitle;
    button.setAttribute("aria-label", button.dataset.copyTitle);
    button.innerHTML = button.dataset.copyIcon;
  }, 1200);
}
function copiedTextButton(button) {
  const label = button.textContent;
  button.textContent = "Copied";
  clearTimeout(button.copyResetTimer);
  button.copyResetTimer = setTimeout(() => (button.textContent = label), 1200);
}
async function fetchSelectedLog(tail) {
  if (!selectedLog) return selectedOutput;
  const suffix = tail ? "&tail=" + tail : "";
  const response = await fetch(
    "/api/log?project_name=" +
      encodeURIComponent(selectedLog.queue) +
      "&run_id=" +
      encodeURIComponent(selectedLog.run) +
      "&job_id=" +
      encodeURIComponent(selectedLog.job) +
      suffix,
  );
  if (!response.ok) throw new Error(await response.text());
  const value = await response.text();
  return tail ? lastLogLines(value, tail) : value;
}
async function copyModalOutput(button) {
  try {
    await copyText(
      document.getElementById("output-modal").dataset.view === "log"
        ? await fetchSelectedLog(0)
        : selectedOutput,
    );
    copied(button);
  } catch (error) {
    alert(error.message);
  }
}
async function copyLogTail(button) {
  try {
    await copyText(await fetchSelectedLog(100));
    copiedTextButton(button);
  } catch (error) {
    alert(error.message);
  }
}
function updateModalActions() {
  const view = document.getElementById("output-modal").dataset.view;
  const copyTail = document.getElementById("copy-tail");
  copyTail.hidden = view !== "log";
  copyTail.dataset.copyTitle = "Copy last 100 lines";
  copyTail.dataset.copyIcon ||= copyTail.innerHTML;
  document.getElementById("report-note").hidden = view !== "ai";
  ["open-gemini", "open-chatgpt", "open-claude"].forEach((id) => {
    const button = document.getElementById(id);
    button.hidden = view !== "ai";
    button.dataset.copyTitle ||= button.textContent.trim();
    button.dataset.copyIcon ||= button.innerHTML;
  });
  const copyButton = document.getElementById("copy-modal");
  const copyTitle = view === "log" ? "Copy log" : "Copy";
  copyButton.title = copyTitle;
  copyButton.setAttribute("aria-label", copyTitle);
  copyButton.dataset.copyTitle = copyTitle;
  copyButton.dataset.copyIcon ||= copyButton.innerHTML;
}
async function openAI(url, button) {
  window.open(url, "_blank", "noopener");
  await copyText(selectedOutput);
  copied(button);
}
async function showAIReport(project, run, job, jobIDs) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "ai";
  modal.querySelector("strong").textContent = job ? "Job report" : "Run report";
  selectedLog = null;
  selectedOutput = "Preparing...";
  ensureModalOutput().textContent = selectedOutput;
  openOutputModal(false);
  const params = new URLSearchParams({
    project_name: project.project_name,
    run_id: run.run_id,
  });
  if (job) params.set("job_id", job.id);
  if (!job && jobIDs)
    jobIDs.forEach((jobID) => params.append("job_ids", jobID));
  const response = await fetch("/api/report?" + params);
  selectedOutput = await response.text();
  if (!response.ok)
    selectedOutput = "Failed to prepare report: " + selectedOutput;
  ensureModalOutput().textContent = selectedOutput;
  openOutputModal(false);
}
function addAIButtons() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const project = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    project &&
    project.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  if (!run) return;
  const controls = document.querySelector(".web-copy-controls");
  if (controls && !controls.querySelector(".run-ai")) {
    const button = document.createElement("button");
    button.className = "run-ai";
    button.textContent = "Report";
    button.title = "Prepare run report";
    button.onclick = () =>
      showAIReport(project, run, null, selectedRunJobIDs());
    controls.append(button);
  }
  const table = document.querySelector("#app table.runs");
  if (!table) return;
  const actionIndex = [...table.querySelectorAll("thead th")].findIndex(
    (header) => header.textContent.trim() === "Actions",
  );
  if (actionIndex < 0) return;
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const actions = row.children[actionIndex];
    const job = run.jobs[index];
    if (!actions || !job || actions.querySelector(".job-ai")) return;
    const button = document.createElement("button");
    button.className = "job-ai";
    button.textContent = "Report";
    button.title = "Prepare job report";
    button.onclick = () => showAIReport(project, run, job);
    actions.append(" ", button);
  });
}
function arrangeRunControls() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const controls = document.querySelector(".web-copy-controls");
  const table = document.querySelector("#app table.runs");
  if (!controls || !table) return;
  const selectAll = document.createElement("button");
  selectAll.textContent = "Select all";
  selectAll.title = "Select all jobs, or clear the current selection";
  selectAll.onclick = () => {
    const inputs = [...document.querySelectorAll(".job-selection")];
    inputs.forEach((input) => (input.checked = true));
    updateSelectedRunJobs();
  };
  const unselectAll = document.createElement("button");
  unselectAll.className = "unselect-all";
  unselectAll.textContent = "Unselect all";
  unselectAll.title = "Clear all selected jobs";
  unselectAll.disabled = true;
  unselectAll.onclick = clearSelectedJobs;
  const selectFailed = controls.querySelector(".select-failed");
  const failed = controls.querySelector(".select-failed-unfinished");
  const create = controls.querySelector(".create-selected");
  const append = controls.querySelector(".append-selected");
  const report = controls.querySelector(".run-ai");
  const deleteButton = [...controls.querySelectorAll("button")].find((button) =>
    button.classList.contains("delete-run"),
  );
  const cancel = [...controls.querySelectorAll("button")].find(
    (button) => button.textContent.trim() === "Cancel run",
  );
  if (create) create.textContent = "Create queue";
  if (append) append.textContent = "Append to queue";
  controls.querySelectorAll("button").forEach((button) => {
    if (button.textContent.trim() === "Clear selection") button.remove();
  });
  controls.replaceChildren(
    ...[
      selectAll,
      selectFailed,
      failed,
      unselectAll,
      create,
      append,
      report,
      deleteButton,
      cancel,
    ].filter(Boolean),
  );
  const load = document.querySelector(".run-environment");
  if (load) load.after(controls);
  else table.before(controls);
  const headerSelect = document.getElementById("select-all-jobs");
  if (headerSelect) headerSelect.remove();
  updateSelectedRunJobs();
}
function orderJobActions() {
  document.querySelectorAll("#app table.runs tbody tr").forEach((row) => {
    const cell = row.firstElementChild;
    if (!cell) return;
    const buttons = [...cell.querySelectorAll("button")];
    const order = [
      "view-log",
      "job-ai",
      "diagnosis",
      "show-path",
      "suspend-job",
      "resume-job",
      "cancel-job",
    ];
    const ordered = [];
    order.forEach((className) => {
      buttons
        .filter((button) => button.classList.contains(className))
        .forEach((button) => ordered.push(button));
    });
    buttons
      .filter((button) => !ordered.includes(button))
      .forEach((button) => ordered.push(button));
    if (!ordered.length) return;
    cell.replaceChildren();
    ordered.forEach((button, index) => {
      if (index) cell.append(" ");
      cell.append(button);
    });
  });
}
function shellQuote(v) {
  return "'" + String(v || "").replace(/'/g, "'\\''") + "'";
}
function keepGlobalOutputBox() {}
function removeLegacyOutputBox() {
  document
    .querySelectorAll("#app pre.log:not(.row-log)")
    .forEach((element) => element.remove());
}
function ensureOutputBox() {
  let output = document.getElementById("log");
  if (!output) {
    output = document.createElement("pre");
    output.id = "log";
    output.className = "log";
    const main = document.querySelector("main");
    const pageTitle = document.getElementById("page-title");
    const section = pageTitle && pageTitle.parentElement;
    if (main && section) main.insertBefore(output, section);
  }
  return output;
}
function ensureModalOutput() {
  return document.getElementById("modal-log") || ensureOutputBox();
}
function openOutputModal(compact) {
  const modal = document.getElementById("output-modal");
  updateModalActions();
  modal.style.display = "flex";
  modal.querySelector(".output-panel").classList.toggle("compact", !!compact);
}
function closeOutputModal() {
  document.getElementById("output-modal").style.display = "none";
  delete document.getElementById("output-modal").dataset.view;
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  selectedOutput = "";
}
function restoreSelectedOutput() {
  if (selectedLog && selectedOutput) {
    const output = ensureModalOutput();
    output.textContent = selectedOutput;
    openOutputModal(isCompactOutput(selectedOutput));
    attachLogLoader(output);
  }
}
function placeOutputBox() {}
function renameCopyButtons() {
  document.querySelectorAll(".web-copy-controls button").forEach((button) => {
    if (button.textContent === "Copy failed jobs")
      button.textContent = "Create queue from failed jobs";
    if (button.textContent === "Copy all jobs")
      button.textContent = "Create queue from all jobs";
  });
}
function labelEquivalentCommand() {
  document.querySelectorAll("pre.log").forEach((pre) => {
    pre.textContent = pre.textContent.replace(
      "Retry from a terminal:",
      "Equivalent command:",
    );
  });
}
