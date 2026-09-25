function applyStatusColors() {
  const colors = {
    pending: "#f3c969",
    unfinished: "#f3c969",
    running: "#f3c969",
    success: "#63d297",
    finished: "#63d297",
    failed: "#ff7c7c",
    blocked: "#ff9f68",
  };
  document.querySelectorAll(".status-value").forEach((element) => {
    const value = element.textContent.trim().toLowerCase();
    if (colors[value]) element.style.color = colors[value];
  });
}
function sortTable(table, key, stateKey) {
  const headers = [...table.querySelectorAll("thead th")];
  const index = headers.findIndex((header) => header.dataset.sort === key);
  if (index < 0) return;
  const state = sortState[stateKey];
  const cellValue = (cell) => {
    const field = cell.querySelector("input,select");
    return field ? field.value.trim() : cell.textContent.trim();
  };
  const isTimeKey = (key) =>
    key === "started" ||
    key === "finished" ||
    key === "source_started" ||
    key === "source_finished";
  const rows = [...table.querySelectorAll("tbody tr")];
  rows.sort((left, right) => {
    const a = cellValue(left.children[index]),
      b = cellValue(right.children[index]);
    if (a === "-") return b === "-" ? 0 : 1;
    if (b === "-") return -1;
    if (isTimeKey(key)) {
      const ta = Date.parse(a),
        tb = Date.parse(b);
      if (Number.isFinite(ta) && Number.isFinite(tb))
        return (ta - tb) * state.direction;
    }
    const na = Number(a),
      nb = Number(b);
    if (Number.isFinite(na) && Number.isFinite(nb))
      return (na - nb) * state.direction;
    return a.localeCompare(b, undefined, { numeric: true }) * state.direction;
  });
  const body = table.querySelector("tbody");
  rows.forEach((row) => body.append(row));
  headers.forEach((header) => {
    if (!header.dataset.sort) return;
    header.style.cursor = "pointer";
    const label = header.dataset.label || header.textContent.trim();
    header.dataset.label = label;
    header.textContent =
      label +
      (header.dataset.sort === state.key
        ? state.direction === 1
          ? " ↑"
          : " ↓"
        : " ↕");
    header.onclick = () => {
      if (state.key === header.dataset.sort) state.direction *= -1;
      else {
        state.key = header.dataset.sort;
        state.direction = 1;
      }
      sortTable(table, state.key, stateKey);
    };
  });
}
function markJobHeaders() {}
const paginationState = {
  queue: { page: 0, context: "" },
  run: { page: 0, context: "" },
  job: { page: 0, context: "" },
  queueJobs: { page: 0, context: "" },
};
const paginationPageSize = 20;
function paginateTable(table, key) {
  if (!table) return;
  const state = paginationState[key];
  const contextKey = location.pathname;
  if (state.context !== contextKey) {
    state.context = contextKey;
    state.page = 0;
  }
  const rows = [...table.querySelectorAll("tbody tr")];
  const totalPages = Math.max(1, Math.ceil(rows.length / paginationPageSize));
  if (state.page >= totalPages) state.page = totalPages - 1;
  if (state.page < 0) state.page = 0;
  const start = state.page * paginationPageSize;
  const end = start + paginationPageSize;
  rows.forEach((row, index) => {
    row.style.display = index >= start && index < end ? "" : "none";
  });
  let controls = table.nextElementSibling;
  if (!controls || !controls.classList.contains("table-pagination")) {
    controls = document.createElement("div");
    controls.className = "table-pagination";
    controls.style.alignItems = "center";
    controls.style.gap = "10px";
    controls.style.margin = "10px 0";
    controls.style.color = "var(--muted)";
    controls.style.fontSize = "13px";
    table.after(controls);
  }
  const prev = document.createElement("button");
  prev.textContent = "Prev";
  prev.disabled = state.page <= 0;
  prev.onclick = () => {
    state.page--;
    paginateTable(table, key);
  };
  const next = document.createElement("button");
  next.textContent = "Next";
  next.disabled = state.page >= totalPages - 1;
  next.onclick = () => {
    state.page++;
    paginateTable(table, key);
  };
  const label = document.createElement("span");
  label.textContent =
    "Page " +
    (state.page + 1) +
    " / " +
    totalPages +
    " (" +
    rows.length +
    " rows)";
  controls.innerHTML = "";
  controls.append(prev, label, next);
  controls.style.display = rows.length <= paginationPageSize ? "none" : "flex";
}
function enableTableSorting() {
  const queueTable = document.querySelector(".queue-overview");
  if (queueTable) {
    sortTable(queueTable, sortState.queue.key, "queue");
    paginateTable(queueTable, "queue");
  }
  const parts = pageParts();
  const runTable = [...document.querySelectorAll("#app table.runs")].find(
    (item) => !item.closest(".web-queue-commands"),
  );
  if (runTable && !parts[2]) {
    sortTable(runTable, sortState.run.key, "run");
    paginateTable(runTable, "run");
  }
  if (runTable && parts[2] === "run") {
    sortTable(runTable, sortState.job.key, "job");
    paginateTable(runTable, "job");
  }
  const queueJobsTable = document.querySelector(".web-queue-jobs");
  if (queueJobsTable && !parts[2]) {
    sortTable(queueJobsTable, sortState.queueJobs.key, "queueJobs");
    paginateTable(queueJobsTable, "queueJobs");
  }
}
function runOrderKey(run) {
  const sample = run?.context?.load_samples?.[0];
  return (sample && sample.at) || run.started_at || "";
}
function latestRun(runs) {
  return (runs || []).reduce((latest, run) => {
    if (!latest) return run;
    const key = runOrderKey(run);
    const latestKey = runOrderKey(latest);
    return key > latestKey || (key === latestKey && run.run_id > latest.run_id)
      ? run
      : latest;
  }, null);
}
function enhanceQueueOverview() {
  const parts = pageParts();
  if (parts.length) return;
  const queues = state.projects || [];
  document.querySelectorAll("#app section").forEach((section, index) => {
    const queue = queues[index];
    if (!queue) return;
    const latest = latestRun(queue.runs);
    const latestHTML = latest
      ? '<div class="meta">Latest run: <a class="link" href="/project/' +
        encodeURIComponent(queue.project_name) +
        "/run/" +
        encodeURIComponent(latest.run_id) +
        '">' +
        esc(latest.run_name || latest.run_id) +
        '</a></div><div class="summary"><span class="status-' +
        esc(latest.status) +
        '">' +
        esc(latest.status) +
        "</span><span>Started: " +
        esc(latest.started_at || "-") +
        "</span><span>Finished: " +
        esc(latest.finished_at || "-") +
        "</span></div>"
      : '<div class="meta">No runs yet</div>';
    section.innerHTML =
      '<h2><a class="link" href="/project/' +
      encodeURIComponent(queue.project_name) +
      '">' +
      esc(queue.project_name) +
      '</a></h2><div class="summary"><span>' +
      (queue.queue.commands || []).length +
      " queued</span><span>" +
      queue.runs.length +
      " runs</span><span>" +
      queue.runs.filter((r) => r.running).length +
      " running</span></div>" +
      latestHTML;
  });
}
function markLatestRun() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2]) return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  const table = [...document.querySelectorAll("#app table.runs")].find(
    (item) => !item.closest(".web-queue-commands"),
  );
  if (!queue || !table) return;
  table.querySelectorAll("tbody tr").forEach((row) => {
    row.classList.remove("latest-run");
    const badge = row.querySelector(".latest-badge");
    if (badge) badge.remove();
  });
  const latest = latestRun(queue.runs);
  if (!latest) return;
  table.querySelectorAll("tbody tr").forEach((row) => {
    const link = row.querySelector("a.run-id");
    if (
      link &&
      decodeURIComponent(link.getAttribute("href")).endsWith(
        "/run/" + latest.run_id,
      )
    ) {
      row.classList.add("latest-run");
      link.insertAdjacentHTML(
        "afterend",
        '<span class="latest-badge">latest</span>',
      );
    }
  });
}
function addQueueOverviewPathActions() {
  if (pageParts().length !== 0) return;
  const table = document.querySelector(".queue-overview");
  if (!table) return;
  const header = document.createElement("th");
  header.textContent = "Actions";
  table.querySelector("thead tr").append(header);
  const queues = state.projects || [];
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const queue = queues[index];
    const cell = document.createElement("td");
    if (queue)
      addPathButton(cell, state.base_dir + "/projects/" + queue.project_name);
    row.append(cell);
  });
}
function jobDisplayStatus(job, run) {
  const result = job.result;
  if (!result)
    return job.scheduler_state || (run.running ? "running" : "pending");
  if (result.accepted) return "success (accepted)";
  if (result.error === "blocked by failed dependency") return "blocked";
  return result.exit_code === 0 ? "success" : "failed";
}
function addRunJobStatusColumn() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const table = document.querySelector("#app table.runs");
  if (!run || !table || table.querySelector(".job-status-header")) return;
  const header = document.createElement("th");
  header.className = "job-status-header";
  header.dataset.sort = "status";
  header.textContent = "Status";
  table
    .querySelector("thead tr")
    .insertBefore(header, table.querySelector("thead tr").children[1]);
  const rows = table.querySelectorAll("tbody tr");
  (run.jobs || []).forEach((job, index) => {
    if (!rows[index]) return;
    const status = document.createElement("td");
    status.className = "status-value";
    status.textContent = jobDisplayStatus(job, run);
    rows[index].insertBefore(status, rows[index].children[1]);
  });
}
function addRunningOutputButtons() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const table = document.querySelector("#app table.runs");
  if (!run || !table) return;
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const cell = row.children[row.children.length - 2];
    if (cell && cell.textContent.trim() === "-") {
      const job = run.jobs[index];
      if (job) {
        const started = !!job.submitted_at || !!job.result || run.running;
        const button = document.createElement("button");
        button.className = "view-log";
        button.textContent = "View log";
        button.disabled = !started;
        button.title = started ? "" : "Job has not started yet";
        button.onclick = () =>
          showLog(queue.project_name, run.run_id, job.id, job.attempt_id);
        cell.textContent = "";
        cell.append(button);
      }
    }
  });
}
function addRunningCancelButtons() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const table = document.querySelector("#app table.runs");
  if (!run || !table) return;
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const job = run.jobs[index];
    const actions = row.lastElementChild;
    if (!job || !actions || actions.querySelector(".cancel-job")) return;
    const status = jobDisplayStatus(job, run);
    const suspended = status === "suspended";
    const controllable = status === "running" || suspended;
    const cancelButton = document.createElement("button");
    cancelButton.className = "cancel-job";
    cancelButton.textContent = "Cancel";
    cancelButton.disabled = !controllable;
    cancelButton.title = controllable ? "" : "Job is not running";
    cancelButton.onclick = () =>
      cancelJob(queue.project_name, job.id, job.name || job.id);
    const suspendButton = document.createElement("button");
    suspendButton.className = suspended ? "suspend-job dirty" : "suspend-job";
    suspendButton.textContent = suspended ? "Resume" : "Suspend";
    suspendButton.disabled = !controllable;
    suspendButton.title = controllable ? "" : "Job is not running";
    suspendButton.onclick = () =>
      suspendOrResumeJob(
        queue.project_name,
        job.id,
        job.name || job.id,
        suspended,
      );
    actions.append(" ", cancelButton, " ", suspendButton);
  });
}
async function suspendOrResumeJob(queue, jobID, label, resume) {
  const endpoint = resume ? "/api/resume-job" : "/api/suspend-job";
  const response = await fetch(endpoint, {
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
async function cancelJob(queue, jobID, label) {
  if (!confirm("Cancel " + label + "?")) return;
  const response = await fetch("/api/cancel-job", {
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
function mergeActionColumns() {
  document.querySelectorAll("#app table.runs").forEach((table) => {
    const headerRow = table.querySelector("thead tr");
    const bodyRows = table.querySelectorAll("tbody tr");
    if (!headerRow || !bodyRows.length) return;
    const headers = headerRow.children;
    if (
      headers.length < 2 ||
      headers[headers.length - 1].textContent.trim() !== "Actions"
    )
      return;
    const actionIndex = headers.length - 1;
    const outputIndex = actionIndex - 1;
    const outputLabel = headers[outputIndex].textContent.trim();
    if (outputLabel !== "Output" && outputLabel !== "Source output") return;
    headers[outputIndex].remove();
    bodyRows.forEach((row) => {
      const outputCell = row.children[outputIndex];
      const actionCell = row.children[actionIndex];
      if (outputCell && actionCell) {
        const nodes = [
          ...outputCell.childNodes,
          ...actionCell.childNodes,
        ].filter((node) => node.nodeType !== 3 || node.textContent.trim());
        actionCell.textContent = "";
        nodes.forEach((node) => actionCell.append(node));
        outputCell.remove();
      }
    });
  });
}
function normalizeJobActionHeaders() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const table = document.querySelector("#app table.runs");
  if (!table) return;
  const headers = table.querySelectorAll("thead th");
  if (headers.length >= 2) headers[headers.length - 2].textContent = "Output";
}
function mergeActionColumns() {}
function labelJobActionHeaders() {
  document.querySelectorAll("#app table.runs").forEach((table) => {
    const headers = table.querySelectorAll("thead th");
    if (headers.length) {
      headers[headers.length - 1].textContent = "Actions";
      headers[headers.length - 1].dataset.sort = "";
    }
  });
}
function clarifyLogControls() {
  if (
    document.getElementById("output-modal").dataset.view !== "config" &&
    document.getElementById("output-modal").dataset.view !==
      "generate-config" &&
    document.getElementById("output-modal").dataset.view !== "diagnosis" &&
    document.getElementById("output-modal").dataset.view !== "path" &&
    document.getElementById("output-modal").dataset.view !== "ai"
  )
    document.querySelector("#output-modal strong").textContent = "Job log";
  document.querySelectorAll("#app table.runs th").forEach((header) => {
    if (header.textContent.trim() === "Output") header.textContent = "Logs";
    if (header.textContent.trim() === "Source output")
      header.textContent = "Source log";
  });
  document.querySelectorAll("#app table.runs button").forEach((button) => {
    if (button.textContent.trim() === "Output") button.textContent = "View log";
  });
}
function styleActionColumns() {
  clarifyLogControls();
  moveActionColumnsLeft();
  mergeLogButtonIntoActions();
  document
    .querySelectorAll("#app table.runs th:first-child")
    .forEach((cell) => {
      if (cell.textContent.trim() === "Actions") {
        cell.style.width = "170px";
        cell.style.textAlign = "left";
      }
    });
  document
    .querySelectorAll("#app table.runs td:first-child")
    .forEach((cell) => {
      if (!cell.querySelector("button")) return;
      cell.style.width = "170px";
      cell.style.minWidth = "170px";
      cell.style.whiteSpace = "normal";
      cell.style.textAlign = "left";
      cell.style.display = "table-cell";
      cell.style.verticalAlign = "top";
      cell
        .querySelectorAll("button")
        .forEach((button) => (button.style.margin = "0 6px 6px 0"));
    });
}
function mergeLogButtonIntoActions() {
  document.querySelectorAll("#app table.runs").forEach((table) => {
    const headerRow = table.querySelector("thead tr");
    if (
      !headerRow ||
      !headerRow.children.length ||
      headerRow.children[0].textContent.trim() !== "Actions"
    )
      return;
    const rows = [...table.querySelectorAll("tbody tr")];
    let logIndex = -1;
    for (const row of rows) {
      const index = [...row.children].findIndex((cell) => {
        const button = cell.querySelector("button");
        return button && button.textContent.trim() === "View log";
      });
      if (index > 0) {
        logIndex = index;
        break;
      }
    }
    if (logIndex < 1) return;
    const header = headerRow.children[logIndex];
    if (header) header.remove();
    rows.forEach((row) => {
      const actionCell = row.children[0];
      const logCell = row.children[logIndex];
      if (!actionCell || !logCell) return;
      const buttons = [...logCell.querySelectorAll("button")];
      if (buttons.length) {
        const controls = document.createDocumentFragment();
        buttons.forEach((button, index) => {
          if (index) controls.append(" ");
          controls.append(button);
        });
        actionCell.insertBefore(controls, actionCell.firstChild);
      }
      logCell.remove();
    });
  });
}
// Long values in these job and queue table columns start clamped to a few
// lines. Clicking the text or its More/Less toggle expands or collapses it;
// a click that ends a text selection does not. Expanded cells stay open across
// re-renders.
const clampedTableColumns = [
  "command",
  "working_directory",
  "depends",
  "options",
];
const clampedCellMinLength = 60;
const expandedTableCells = new Set();
function clampLongTableCells() {
  document.querySelectorAll("#app table.runs").forEach((table) => {
    const columns = [...table.querySelectorAll("thead th")]
      .map((header, index) => ({ index, key: header.dataset.sort }))
      .filter((column) => clampedTableColumns.includes(column.key));
    if (!columns.length) return;
    table.querySelectorAll("tbody tr").forEach((row, rowIndex) => {
      columns.forEach(({ index, key }) => {
        const cell = row.children[index];
        if (
          !cell ||
          cell.querySelector("input,select,textarea,.cell-clamp") ||
          cell.textContent.trim().length <= clampedCellMinLength
        )
          return;
        const id = (row.dataset.jobId || "row-" + rowIndex) + "\u0000" + key;
        // Keep buttons such as the copy icon outside the clamped text so
        // they stay visible.
        const text = document.createElement("div");
        text.className = "cell-clamp";
        [...cell.childNodes]
          .filter((node) => !(node.nodeType === 1 && node.matches("button")))
          .forEach((node) => text.append(node));
        cell.prepend(text);
        const toggle = document.createElement("button");
        toggle.type = "button";
        toggle.className = "cell-toggle";
        const apply = () => {
          const expanded = expandedTableCells.has(id);
          text.classList.toggle("collapsed", !expanded);
          toggle.textContent = expanded ? "▴ Less" : "▾ More";
          toggle.setAttribute("aria-expanded", String(expanded));
          text.title = expanded ? "Click to collapse" : "Click to expand";
        };
        const flip = (event) => {
          event.stopPropagation();
          if (expandedTableCells.has(id)) expandedTableCells.delete(id);
          else expandedTableCells.add(id);
          apply();
        };
        toggle.onclick = flip;
        text.onclick = (event) => {
          const selection = window.getSelection && window.getSelection();
          if (selection && String(selection)) return;
          flip(event);
        };
        // The toggle comes before the cell's own buttons, such as copy.
        cell.insertBefore(toggle, cell.querySelector(":scope > button"));
        apply();
        // Drop the toggle when the text already fits in the clamped lines.
        if (
          !expandedTableCells.has(id) &&
          text.scrollHeight > 0 &&
          text.scrollHeight <= text.clientHeight + 1
        ) {
          toggle.remove();
          text.classList.remove("collapsed");
          text.onclick = null;
          text.removeAttribute("title");
        }
      });
    });
  });
}
