// Matrix panels summarize each matrix group of a run as a grid above the job
// table: the first dimension forms the rows, the second the columns, and any
// further dimensions split the group into one grid per remaining combination.
// A cell's color is the worst state of its jobs (several for array tasks), and
// clicking it opens a box with each job's table-row actions.

const matrixStateOrder = ["failed", "blocked", "running", "pending", "success"];

// matrixJobState classifies a job like the job table: a result decides
// success, blocked, or failed; a job without one is running while the run is.
function matrixJobState(job, running) {
  const result = job.result;
  if (!result) return running ? "running" : "pending";
  if (result.accepted || result.exit_code === 0) return "success";
  if ((result.error || "").startsWith("blocked")) return "blocked";
  return "failed";
}

function matrixKey(values) {
  return values.map((value) => value.name + "=" + value.value).join("\u0000");
}

function matrixCombinations(dimensions) {
  return dimensions.reduce(
    (combinations, dimension) =>
      combinations.flatMap((combination) =>
        dimension.values.map((value) =>
          combination.concat([{ name: dimension.name, value: value }]),
        ),
      ),
    [[]],
  );
}

function renderMatrixCell(groupID, key, cellJobs, running) {
  if (!cellJobs || cellJobs.length === 0) {
    return '<td class="matrix-cell matrix-missing">-</td>';
  }
  const states = cellJobs.map((job) => matrixJobState(job, running));
  const state = matrixStateOrder.find((candidate) =>
    states.includes(candidate),
  );
  const succeeded = states.filter((value) => value === "success").length;
  const label = cellJobs.length > 1 ? succeeded + "/" + cellJobs.length : state;
  // List the jobs worth opening first, so a click shows a failure if any.
  const ordered = cellJobs
    .map((job, index) => ({
      job,
      rank: matrixStateOrder.indexOf(states[index]),
    }))
    .sort((left, right) => left.rank - right.rank)
    .map((entry) => entry.job.id);
  const title = cellJobs
    .map((job, index) => (job.name || job.id) + ": " + states[index])
    .join("\n");
  const jobsByID = {};
  cellJobs.forEach((job, index) => {
    jobsByID[job.id] = {
      id: job.id,
      name: job.name || job.id,
      state: states[index],
    };
  });
  return (
    '<td class="matrix-cell matrix-' +
    state +
    '" data-cell-key="' +
    esc(groupID + "\u0001" + key) +
    '" data-jobs="' +
    esc(JSON.stringify(ordered.map((id) => jobsByID[id]))) +
    '" title="' +
    esc(title) +
    '" onclick="openMatrixCell(this)">' +
    esc(label) +
    "</td>"
  );
}

function renderMatrixGrid(groupID, dimensions, fixed, cells, running) {
  const rows = dimensions[0];
  const columns = dimensions.length > 1 ? dimensions[1] : null;
  const columnValues = columns ? columns.values : [null];
  const header =
    "<tr><th>" +
    (columns ? esc(rows.name + " \\ " + columns.name) : esc(rows.name)) +
    "</th>" +
    columnValues
      .map((value) =>
        value === null
          ? "<th></th>"
          : "<th>" + esc(columns.name + "=" + value) + "</th>",
      )
      .join("") +
    "</tr>";
  const body = rows.values
    .map((rowValue) => {
      const cellsHTML = columnValues
        .map((columnValue) => {
          const values = [{ name: rows.name, value: rowValue }];
          if (columnValue !== null) {
            values.push({ name: columns.name, value: columnValue });
          }
          const key = matrixKey(values.concat(fixed));
          return renderMatrixCell(groupID, key, cells[key], running);
        })
        .join("");
      return (
        "<tr><th>" +
        esc(rows.name + "=" + rowValue) +
        "</th>" +
        cellsHTML +
        "</tr>"
      );
    })
    .join("");
  const caption = fixed.length
    ? '<div class="meta">' +
      esc(fixed.map((value) => value.name + "=" + value.value).join(", ")) +
      "</div>"
    : "";
  return caption + '<table class="matrix-grid">' + header + body + "</table>";
}

// renderMatrixPanels returns the grids for every matrix group among jobs, or
// an empty string when the run has none.
function renderMatrixPanels(jobs, running) {
  const groups = [];
  const byGroup = {};
  jobs.forEach((job) => {
    const matrix = job.matrix;
    if (!matrix || !matrix.group_id || !(matrix.dimensions || []).length)
      return;
    if (!byGroup[matrix.group_id]) {
      byGroup[matrix.group_id] = { matrix: matrix, cells: {} };
      groups.push(byGroup[matrix.group_id]);
    }
    // Order values by dimension so keys match the grid's lookups.
    const values = matrix.dimensions.map((dimension) =>
      (matrix.values || []).find((value) => value.name === dimension.name),
    );
    if (values.some((value) => !value)) return;
    const key = matrixKey(values);
    (byGroup[matrix.group_id].cells[key] =
      byGroup[matrix.group_id].cells[key] || []).push(job);
  });
  return groups
    .map((group) => {
      const dimensions = group.matrix.dimensions;
      const shown = dimensions.slice(0, 2);
      const rest = dimensions.slice(2);
      // Keys list dimensions in order, so the fixed values follow the
      // shown ones when looked up.
      const grids = matrixCombinations(rest)
        .map((fixed) =>
          renderMatrixGrid(
            group.matrix.group_id,
            shown,
            fixed,
            group.cells,
            running,
          ),
        )
        .join("");
      const title = group.matrix.base_name || group.matrix.group_id;
      return (
        '<section class="matrix-panel"><h3>Matrix: ' +
        esc(title) +
        ' <span class="meta">(' +
        esc(dimensions.map((dimension) => dimension.name).join(" × ")) +
        ")</span></h3>" +
        grids +
        "</section>"
      );
    })
    .join("");
}

// The open action box lives outside #app, so the periodic re-render does not
// remove it; restoreMatrixActions reattaches it to the re-rendered cell.
let openMatrixCellKey = "";
let openMatrixCellJobs = "";

function matrixJobRow(jobID) {
  return Array.from(document.querySelectorAll("tr[data-job-id]")).find(
    (row) => row.dataset.jobId === jobID,
  );
}

function matrixRowActions(row) {
  const table = row && row.closest("table");
  if (!table) return [];
  const headers = Array.from(table.querySelectorAll("thead th"));
  const index = headers.findIndex(
    (header) => header.textContent.trim() === "Actions",
  );
  const cell = index >= 0 ? row.children[index] : null;
  return cell ? Array.from(cell.querySelectorAll("button")) : [];
}

// focusMatrixJob scrolls to a job's table row and highlights it briefly.
function focusMatrixJob(jobID) {
  const row = matrixJobRow(jobID);
  if (!row) return;
  if (row.scrollIntoView) row.scrollIntoView({ block: "center" });
  row.classList.add("matrix-focus");
  setTimeout(() => row.classList.remove("matrix-focus"), 2000);
}

function closeMatrixActions() {
  openMatrixCellKey = "";
  openMatrixCellJobs = "";
  const box = document.getElementById("matrix-actions");
  if (box) box.remove();
}

function positionMatrixActions(box, cell) {
  const rect = cell.getBoundingClientRect();
  box.style.left = rect.left + window.scrollX + "px";
  box.style.top = rect.bottom + window.scrollY + 4 + "px";
}

// openMatrixCell opens a box listing each of the cell's jobs with a copy of
// its table row's action buttons. A copy presses the current row's button,
// so it behaves exactly like the table, including buttons that later
// render steps add.
function openMatrixCell(cell) {
  closeMatrixActions();
  openMatrixCellKey = cell.dataset.cellKey || "";
  openMatrixCellJobs = cell.dataset.jobs || "[]";
  const box = document.createElement("div");
  box.id = "matrix-actions";
  box.className = "matrix-actions";
  box.setAttribute("role", "dialog");
  const close = document.createElement("button");
  close.type = "button";
  close.className = "matrix-actions-close";
  close.textContent = "×";
  close.title = "Close";
  close.setAttribute("aria-label", "Close");
  close.onclick = closeMatrixActions;
  box.append(close);
  JSON.parse(openMatrixCellJobs).forEach((job) => {
    const entry = document.createElement("div");
    entry.className = "matrix-actions-job";
    const heading = document.createElement("div");
    heading.className = "matrix-actions-heading";
    const name = document.createElement("strong");
    name.textContent = job.name;
    const state = document.createElement("span");
    state.className = "matrix-" + job.state;
    state.textContent = " " + job.state;
    heading.append(name, state);
    const buttons = document.createElement("div");
    buttons.className = "matrix-actions-buttons";
    matrixRowActions(matrixJobRow(job.id)).forEach((original, index) => {
      const copy = document.createElement("button");
      copy.type = "button";
      copy.innerHTML = original.innerHTML;
      copy.title = original.title;
      copy.disabled = original.disabled;
      const label = original.getAttribute("aria-label");
      if (label) copy.setAttribute("aria-label", label);
      copy.onclick = () => {
        const current = matrixRowActions(matrixJobRow(job.id))[index];
        closeMatrixActions();
        if (current) current.click();
      };
      buttons.append(copy, " ");
    });
    const show = document.createElement("button");
    show.type = "button";
    show.textContent = "Show in table";
    show.onclick = () => {
      closeMatrixActions();
      focusMatrixJob(job.id);
    };
    buttons.append(show);
    entry.append(heading, buttons);
    box.append(entry);
  });
  document.body.append(box);
  positionMatrixActions(box, cell);
}

// restoreMatrixActions runs after each render: it moves the open box to the
// re-rendered cell, rebuilds it when the cell's jobs changed, and closes it
// when the cell is gone.
function restoreMatrixActions() {
  if (!openMatrixCellKey) return;
  const cell = Array.from(document.querySelectorAll("td.matrix-cell")).find(
    (candidate) => candidate.dataset.cellKey === openMatrixCellKey,
  );
  const box = document.getElementById("matrix-actions");
  if (!cell) {
    closeMatrixActions();
  } else if (!box || cell.dataset.jobs !== openMatrixCellJobs) {
    openMatrixCell(cell);
  } else {
    positionMatrixActions(box, cell);
  }
}

document.addEventListener("click", (event) => {
  const target = event.target;
  if (
    openMatrixCellKey &&
    target.closest &&
    !target.closest("#matrix-actions") &&
    !target.closest("td.matrix-cell")
  ) {
    closeMatrixActions();
  }
});
document.addEventListener("keydown", (event) => {
  if (event.key === "Escape") closeMatrixActions();
});
