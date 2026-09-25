// Matrix panels summarize each matrix group of a run as a grid above the job
// table: the first dimension forms the rows, the second the columns, and any
// further dimensions split the group into one grid per remaining combination.
// A cell's color is the worst state of its jobs (several for array tasks), and
// clicking it scrolls to the job's table row and opens its output.

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

function renderMatrixCell(cellJobs, running) {
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
  return (
    '<td class="matrix-cell matrix-' +
    state +
    '" data-job-ids="' +
    esc(ordered.join(" ")) +
    '" title="' +
    esc(title) +
    '" onclick="focusMatrixCell(this)">' +
    esc(label) +
    "</td>"
  );
}

function renderMatrixGrid(dimensions, fixed, cells, running) {
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
          return renderMatrixCell(
            cells[matrixKey(values.concat(fixed))],
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
        .map((fixed) => renderMatrixGrid(shown, fixed, group.cells, running))
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

// focusMatrixCell scrolls to the first listed job's table row, highlights it,
// and opens its output through the row's own Output button.
function focusMatrixCell(cell) {
  const jobID = (cell.dataset.jobIds || "").split(" ")[0];
  const row = Array.from(document.querySelectorAll("tr[data-job-id]")).find(
    (candidate) => candidate.dataset.jobId === jobID,
  );
  if (!row) return;
  if (row.scrollIntoView) row.scrollIntoView({ block: "center" });
  row.classList.add("matrix-focus");
  setTimeout(() => row.classList.remove("matrix-focus"), 2000);
  const output = row.querySelector('button[onclick^="log("]');
  if (output) output.click();
}
