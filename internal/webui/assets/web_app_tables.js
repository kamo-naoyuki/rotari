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
        '</a></div><div class="summary">' +
        statusPill(latest.status) +
        "<span>Started: " +
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
function jobDisplayStatus(job, run) {
  if (job.execution_status) return job.execution_status;
  const result = job.result;
  if (!result)
    return job.scheduler_state || (run.running ? "running" : "pending");
  if (result.accepted) return "success (accepted)";
  if (result.error === "blocked by failed dependency") return "blocked";
  return result.exit_code === 0 ? "success" : "failed";
}
// Long values in these job and queue table columns start clamped to a few
// lines. Clicking the text, or pressing Enter or Space on it, expands or
// collapses it; a click that ends a text selection does not. Expanded cells
// stay open across re-renders.
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
        const apply = () => {
          const expanded = expandedTableCells.has(id);
          text.classList.toggle("collapsed", !expanded);
          text.setAttribute("aria-expanded", String(expanded));
          text.title = expanded ? "Click to collapse" : "Click to expand";
        };
        const flip = (event) => {
          event.stopPropagation();
          if (expandedTableCells.has(id)) expandedTableCells.delete(id);
          else expandedTableCells.add(id);
          apply();
        };
        text.onclick = (event) => {
          const selection = window.getSelection && window.getSelection();
          if (selection && String(selection)) return;
          flip(event);
        };
        text.onkeydown = (event) => {
          if (event.key !== "Enter" && event.key !== " ") return;
          event.preventDefault();
          flip(event);
        };
        text.tabIndex = 0;
        text.setAttribute("role", "button");
        apply();
        // Leave text that already fits in the clamped lines alone.
        if (
          !expandedTableCells.has(id) &&
          text.scrollHeight > 0 &&
          text.scrollHeight <= text.clientHeight + 1
        ) {
          text.classList.remove("collapsed", "cell-clamp-toggle");
          text.onclick = null;
          text.onkeydown = null;
          text.removeAttribute("tabindex");
          text.removeAttribute("role");
          text.removeAttribute("aria-expanded");
          text.removeAttribute("title");
          return;
        }
        text.classList.add("cell-clamp-toggle");
      });
    });
  });
}
