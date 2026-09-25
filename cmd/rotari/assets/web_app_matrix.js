// Matrix panels summarize each matrix group of a run as a collapsible grid
// section between the run graphics and the job table controls. Two chosen
// dimensions form the rows and columns, and any further dimensions split the
// group into one grid per remaining combination. A cell's color is the worst
// state of its jobs (several for array tasks), and clicking it opens a box
// with each job's table-row actions.

const matrixStateOrder = ["failed", "blocked", "running", "pending", "success"];
// Per matrix group: whether its section is expanded, and the chosen axes.
const expandedMatrixGroups = {};
const matrixAxes = {};

// matrixJobState classifies a job like the job table: a result decides
// success, blocked, or failed; a job without one is running while the run is.
function matrixJobState(job, running) {
  const result = job.result;
  if (!result) return running ? "running" : "pending";
  if (result.accepted || result.exit_code === 0) return "success";
  if ((result.error || "").startsWith("blocked")) return "blocked";
  return "failed";
}

// matrixKey identifies a combination by its values in the group's dimension
// order, whatever order the values are given in.
function matrixKey(dimensions, values) {
  const byName = {};
  values.forEach((value) => (byName[value.name] = value.value));
  return dimensions
    .map((dimension) => dimension.name + "=" + byName[dimension.name])
    .join("\u0000");
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
  // List the jobs worth opening first, so a failure comes first in the box.
  const entries = cellJobs
    .map((job, index) => ({
      id: job.id,
      name: job.name || job.id,
      state: states[index],
    }))
    .sort(
      (left, right) =>
        matrixStateOrder.indexOf(left.state) -
        matrixStateOrder.indexOf(right.state),
    );
  const title = entries
    .map((entry) => entry.name + ": " + entry.state)
    .join("\n");
  return (
    '<td class="matrix-cell matrix-' +
    state +
    '" data-cell-key="' +
    esc(groupID + "\u0001" + key) +
    '" data-jobs="' +
    esc(JSON.stringify(entries)) +
    '" title="' +
    esc(title) +
    '" onclick="openMatrixCell(this)">' +
    esc(label) +
    "</td>"
  );
}

function renderMatrixGrid(group, rows, columns, fixed, running) {
  const dimensions = group.matrix.dimensions;
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
          const values = fixed.concat([{ name: rows.name, value: rowValue }]);
          if (columnValue !== null) {
            values.push({ name: columns.name, value: columnValue });
          }
          const key = matrixKey(dimensions, values);
          return renderMatrixCell(
            group.matrix.group_id,
            key,
            group.cells[key],
            running,
          );
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

// matrixGroupAxes returns the group's row and column dimensions, defaulting
// to its first two.
function matrixGroupAxes(group) {
  const dimensions = group.matrix.dimensions;
  const names = dimensions.map((dimension) => dimension.name);
  const chosen = matrixAxes[group.matrix.group_id] || {};
  const rows = names.includes(chosen.rows) ? chosen.rows : names[0];
  let columns = names.includes(chosen.columns) ? chosen.columns : names[1];
  if (columns === rows) columns = names.find((name) => name !== rows);
  const find = (name) =>
    dimensions.find((dimension) => dimension.name === name);
  return { rows: find(rows), columns: columns ? find(columns) : null };
}

function renderMatrixGroupContent(group, running) {
  const dimensions = group.matrix.dimensions;
  const axes = matrixGroupAxes(group);
  const shown = [axes.rows.name, axes.columns && axes.columns.name];
  const rest = dimensions.filter(
    (dimension) => !shown.includes(dimension.name),
  );
  const select = (axis, selected) =>
    "<label>" +
    (axis === "rows" ? "Rows" : "Columns") +
    ' <select class="matrix-axis" data-axis="' +
    axis +
    '" data-group-id="' +
    esc(group.matrix.group_id) +
    '" onchange="setMatrixAxis(this)">' +
    dimensions
      .map(
        (dimension) =>
          '<option value="' +
          esc(dimension.name) +
          '"' +
          (dimension.name === selected ? " selected" : "") +
          ">" +
          esc(dimension.name) +
          "</option>",
      )
      .join("") +
    "</select></label>";
  const controls =
    dimensions.length > 1
      ? '<div class="matrix-axes">' +
        select("rows", axes.rows.name) +
        select("columns", axes.columns.name) +
        "</div>"
      : "";
  return (
    controls +
    matrixCombinations(rest)
      .map((fixed) =>
        renderMatrixGrid(group, axes.rows, axes.columns, fixed, running),
      )
      .join("")
  );
}

// matrixGroups collects the run's matrix members by group, in job order.
function matrixGroups(jobs) {
  const groups = [];
  const byGroup = {};
  jobs.forEach((job) => {
    const matrix = job.matrix;
    if (!matrix || !matrix.group_id || !(matrix.dimensions || []).length)
      return;
    if (!byGroup[matrix.group_id]) {
      byGroup[matrix.group_id] = { matrix: matrix, cells: {}, jobs: [] };
      groups.push(byGroup[matrix.group_id]);
    }
    const group = byGroup[matrix.group_id];
    group.jobs.push(job);
    const values = matrix.values || [];
    if (
      matrix.dimensions.some(
        (dimension) => !values.some((value) => value.name === dimension.name),
      )
    )
      return;
    const key = matrixKey(matrix.dimensions, values);
    (group.cells[key] = group.cells[key] || []).push(job);
  });
  return groups;
}

function currentMatrixRun() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return null;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  return (
    (queue &&
      queue.runs.find(
        (item) => item.run_id === decodeURIComponent(parts[3]),
      )) ||
    null
  );
}

function applyMatrixExpanded(section, groupID) {
  const expanded = !!expandedMatrixGroups[groupID];
  const button = section.querySelector(".matrix-toggle");
  button.textContent = expanded ? "-" : "+";
  button.setAttribute("aria-expanded", String(expanded));
  section.querySelector(".matrix-content").hidden = !expanded;
}

// addMatrixPanels runs after each render. It adds one section per matrix
// group, collapsed unless opened, just above the job table controls, then
// restores an open action box.
function addMatrixPanels() {
  const run = currentMatrixRun();
  const app = document.getElementById("app");
  if (!run || !app) return;
  app.querySelectorAll(".matrix-panel").forEach((panel) => panel.remove());
  const anchor =
    app.querySelector(".web-copy-controls") || app.querySelector("table.runs");
  matrixGroups(run.jobs || []).forEach((group) => {
    const groupID = group.matrix.group_id;
    const states = group.jobs.map((job) => matrixJobState(job, !!run.running));
    const count = (value) => states.filter((item) => item === value).length;
    const section = document.createElement("section");
    section.className = "matrix-panel";
    section.dataset.groupId = groupID;
    section.innerHTML =
      '<div class="matrix-heading"><button type="button" class="matrix-toggle"></button><h2>Matrix: ' +
      esc(group.matrix.base_name || groupID) +
      '</h2><span class="meta">' +
      esc(
        group.matrix.dimensions.map((dimension) => dimension.name).join(" × "),
      ) +
      '</span><span class="meta matrix-summary">' +
      esc(
        count("success") +
          "/" +
          states.length +
          " success" +
          (count("failed") ? ", " + count("failed") + " failed" : ""),
      ) +
      '</span></div><div class="matrix-content">' +
      renderMatrixGroupContent(group, !!run.running) +
      "</div>";
    section.querySelector(".matrix-toggle").onclick = () => {
      expandedMatrixGroups[groupID] = !expandedMatrixGroups[groupID];
      applyMatrixExpanded(section, groupID);
      restoreMatrixActions();
    };
    applyMatrixExpanded(section, groupID);
    if (anchor) anchor.before(section);
    else app.append(section);
  });
  restoreMatrixActions();
}

// setMatrixAxis sets a group's row or column dimension; choosing the one the
// other axis uses swaps them.
function setMatrixAxis(select) {
  const groupID = select.dataset.groupId;
  const run = currentMatrixRun();
  const group =
    run &&
    matrixGroups(run.jobs || []).find(
      (item) => item.matrix.group_id === groupID,
    );
  if (!group) return;
  const axes = matrixGroupAxes(group);
  const next = { rows: axes.rows.name, columns: axes.columns.name };
  const other = select.dataset.axis === "rows" ? "columns" : "rows";
  if (next[other] === select.value) next[other] = next[select.dataset.axis];
  next[select.dataset.axis] = select.value;
  matrixAxes[groupID] = next;
  const section = Array.from(document.querySelectorAll(".matrix-panel")).find(
    (panel) => panel.dataset.groupId === groupID,
  );
  if (section) {
    section.querySelector(".matrix-content").innerHTML =
      renderMatrixGroupContent(group, !!run.running);
  }
  closeMatrixActions();
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
    heading.innerHTML =
      "<strong>" +
      esc(job.name) +
      "</strong>" +
      copyIconForValue(job.name, "job name") +
      ' <span class="matrix-' +
      esc(job.state) +
      '">' +
      esc(job.state) +
      "</span>";
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
      buttons.append(copy);
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

// restoreMatrixActions moves the open box to the re-rendered cell, rebuilds
// it when the cell's jobs changed, and closes it when the cell is gone or
// hidden.
function restoreMatrixActions() {
  if (!openMatrixCellKey) return;
  const cell = Array.from(document.querySelectorAll("td.matrix-cell")).find(
    (candidate) =>
      candidate.dataset.cellKey === openMatrixCellKey &&
      !candidate.closest(".matrix-content[hidden]"),
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
