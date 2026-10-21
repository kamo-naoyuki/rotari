// The Table viewer: a delimited file as a paged table. The server parses,
// filters, and pages the file (internal/tableview); this script keeps the
// view's settings in the URL fragment, so a reload or a shared link shows
// the same view, and lays out what the server returns.
"use strict";

// The page's query names the source; API requests pass it through and add
// the view's own parameters.
const sourceQuery = new URLSearchParams(location.search);

const separatorChoices = [
  { label: "Comma ,", value: "," },
  { label: "Tab", value: "\t" },
  { label: "Semicolon ;", value: ";" },
  { label: "Pipe |", value: "|" },
  { label: "Space", value: " " },
];
const pageSizes = [50, 100, 200, 500];
// The filter operators, as the server names them (internal/tableview).
const filterOperators = [
  { op: "=", label: "=" },
  { op: "!=", label: "≠" },
  { op: "<", label: "<" },
  { op: "<=", label: "≤" },
  { op: ">", label: ">" },
  { op: ">=", label: "≥" },
  { op: "contains", label: "contains" },
  { op: "not_contains", label: "does not contain" },
  { op: "empty", label: "is empty", noValue: true },
  { op: "not_empty", label: "is not empty", noValue: true },
];
// The statistics Describe shows, one row each.
const describeRows = [
  ["count", (stats) => formatCount(stats.count)],
  [
    "unique",
    (stats) => formatCount(stats.unique) + (stats.unique_over ? "+" : ""),
  ],
  ["mean", (stats) => formatNumber(stats.mean)],
  ["std", (stats) => formatNumber(stats.std)],
  ["min", (stats) => formatNumber(stats.min)],
  ["max", (stats) => formatNumber(stats.max)],
  ["sum", (stats) => formatNumber(stats.sum)],
];

let info = null;
let rows = null;
let description = null;
// view holds the settings the fragment keeps. A filter or sort names a
// column by its position, counted from 0.
const view = {
  sep: "",
  header: true,
  page: 1,
  limit: 100,
  filters: [],
  sort: null,
  describe: false,
};
// draft is the condition being written in the filter bar.
const draft = { column: 0, op: "contains", value: "" };

// el builds an element: attributes set as properties (or with setAttribute
// for names with a dash), children appended as nodes or text.
function el(tag, attributes, ...children) {
  const node = document.createElement(tag);
  Object.entries(attributes || {}).forEach(([name, value]) => {
    if (value === undefined || value === null || value === false) return;
    if (name.includes("-")) node.setAttribute(name, String(value));
    else node[name] = value;
  });
  setChildren(node, ...children);
  return node;
}

// setChildren replaces a node's children as el adds them: arrays are
// flattened and null, undefined, and false are left out.
function setChildren(node, ...children) {
  node.replaceChildren(
    ...children
      .flat(Infinity)
      .filter(
        (child) => child !== null && child !== undefined && child !== false,
      )
      .map((child) => (child instanceof Node ? child : String(child))),
  );
}

function formatCount(value) {
  return Number(value).toLocaleString("en-US");
}

function formatNumber(value) {
  if (value === undefined || value === null) return "–";
  return Number(value).toLocaleString("en-US", { maximumFractionDigits: 4 });
}

function formatBytes(size) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let value = size;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return (unit ? value.toFixed(1) : String(value)) + " " + units[unit];
}

function readView() {
  const fragment = new URLSearchParams(location.hash.slice(1));
  if (fragment.has("sep")) view.sep = fragment.get("sep");
  if (fragment.has("header")) view.header = fragment.get("header") !== "0";
  const pageNumber = Number(fragment.get("page"));
  if (Number.isInteger(pageNumber) && pageNumber > 0) view.page = pageNumber;
  const limit = Number(fragment.get("limit"));
  if (pageSizes.includes(limit)) view.limit = limit;
  try {
    const filters = JSON.parse(fragment.get("filters") || "[]");
    if (Array.isArray(filters)) view.filters = filters;
  } catch {
    view.filters = [];
  }
  const sort = Number(fragment.get("sort"));
  if (fragment.has("sort") && Number.isInteger(sort) && sort >= 0) {
    view.sort = { column: sort, desc: fragment.get("desc") === "1" };
  }
  view.describe = fragment.get("describe") === "1";
}

function writeView() {
  const fragment = new URLSearchParams();
  fragment.set("sep", view.sep);
  fragment.set("header", view.header ? "1" : "0");
  fragment.set("page", String(view.page));
  fragment.set("limit", String(view.limit));
  if (view.filters.length)
    fragment.set("filters", JSON.stringify(view.filters));
  if (view.sort) {
    fragment.set("sort", String(view.sort.column));
    if (view.sort.desc) fragment.set("desc", "1");
  }
  if (view.describe) fragment.set("describe", "1");
  history.replaceState(null, "", "#" + fragment);
}

// apiURL is an API route relative to the page, with the source's query and
// the given parameters.
function apiURL(route, extra) {
  const query = new URLSearchParams(sourceQuery);
  Object.entries(extra || {}).forEach(([name, value]) => {
    if (value !== undefined && value !== null) query.set(name, String(value));
  });
  return route + "?" + query;
}

async function fetchJSON(url) {
  const response = await fetch(url);
  const body = await response.text();
  if (!response.ok) throw new Error(body.trim() || "HTTP " + response.status);
  return JSON.parse(body);
}

function showError(message) {
  const error = document.getElementById("tv-error");
  error.hidden = !message;
  error.textContent = message || "";
}

function renderHeader() {
  document.title = info.name + " · Table viewer · rotari";
  document.getElementById("tv-trail").textContent = (info.trail || [])
    .filter(Boolean)
    .concat("Table viewer")
    .join(" › ");
  document.getElementById("tv-name").textContent = info.name;
  const kind = document.getElementById("tv-kind");
  kind.hidden = info.kind !== "directory";
  kind.textContent = info.kind;
  const download = document.getElementById("tv-download");
  download.hidden = !info.download;
  if (info.download) download.href = info.download;
}

function renderSummary() {
  const parts = [];
  if (info.kind === "file") parts.push(formatBytes(info.size));
  if (rows) {
    parts.push(formatCount(rows.total) + " rows");
    parts.push(formatCount(rows.columns.length) + " columns");
  }
  document.getElementById("tv-summary").textContent = parts.join(" · ");
}

function renderToolbar() {
  const toolbar = document.getElementById("tv-toolbar");
  const known = separatorChoices.some((choice) => choice.value === view.sep);
  const select = el(
    "select",
    { "aria-label": "Separator", onchange: changeSeparatorChoice },
    separatorChoices.map((choice) =>
      el(
        "option",
        { value: choice.value, selected: choice.value === view.sep },
        choice.label,
      ),
    ),
    el("option", { value: "other", selected: !known }, "Other…"),
  );
  const other = el("input", {
    type: "text",
    className: "tv-separator-other",
    "aria-label": "Separator character",
    value: known ? "" : view.sep,
    hidden: known,
    maxLength: 2,
    onchange: (event) => changeSeparator(event.target.value),
  });
  const header = el("input", {
    type: "checkbox",
    checked: view.header,
    onchange: (event) => {
      view.header = event.target.checked;
      view.page = 1;
      loadRows();
    },
  });
  const describe = el(
    "button",
    {
      type: "button",
      className: "tv-describe-toggle",
      "aria-pressed": String(view.describe),
      title: "Summarize each column over the matching rows",
      onclick: () => {
        view.describe = !view.describe;
        renderToolbar();
        loadRows();
      },
    },
    "Describe",
  );
  toolbar.replaceChildren(
    el("label", {}, el("span", {}, "Separator"), select, other),
    el("label", {}, header, el("span", {}, "First row is a header")),
    el("span", { className: "tv-spacer" }),
    describe,
  );
}

function changeSeparatorChoice(event) {
  if (event.target.value === "other") {
    const other = document.querySelector(".tv-separator-other");
    other.hidden = false;
    other.focus();
    return;
  }
  changeSeparator(event.target.value);
}

function changeSeparator(value) {
  if ([...value].length !== 1) {
    showError("The separator must be one character.");
    return;
  }
  view.sep = value;
  view.page = 1;
  // Another separator makes other columns: filters and sort name them by
  // position.
  view.filters = [];
  view.sort = null;
  renderToolbar();
  loadRows();
}

function pageCount() {
  return Math.max(1, Math.ceil(rows.matched / view.limit));
}

// tableParams are the parameters that choose the rows, for both the rows
// and their description.
function tableParams() {
  return {
    sep: view.sep,
    header: view.header ? "1" : "0",
    filters: view.filters.length ? JSON.stringify(view.filters) : undefined,
  };
}

// loadSequence numbers loads, so a slow answer to an earlier one never
// replaces a later one.
let loadSequence = 0;

async function loadRows() {
  const sequence = ++loadSequence;
  writeView();
  const params = tableParams();
  const sort = view.sort
    ? { sort: view.sort.column, desc: view.sort.desc ? "1" : undefined }
    : {};
  const [rowsResult, describeResult] = await Promise.allSettled([
    fetchJSON(
      apiURL(
        "api/rows",
        Object.assign({}, params, sort, {
          offset: (view.page - 1) * view.limit,
          limit: view.limit,
        }),
      ),
    ),
    view.describe ? fetchJSON(apiURL("api/describe", params)) : null,
  ]);
  if (sequence !== loadSequence) return;
  rows = rowsResult.status === "fulfilled" ? rowsResult.value : null;
  description =
    describeResult.status === "fulfilled" ? describeResult.value : null;
  const failure = [rowsResult, describeResult].find(
    (result) => result.status === "rejected",
  );
  showError(failure ? failure.reason.message : "");
  if (rows && view.page > pageCount()) {
    view.page = pageCount();
    return loadRows();
  }
  renderSummary();
  renderFilterBar();
  renderTable();
  renderPager();
}

function changeFilters(filters) {
  view.filters = filters;
  view.page = 1;
  loadRows();
}

function operatorOf(op) {
  return filterOperators.find((operator) => operator.op === op);
}

function renderFilterBar() {
  const bar = document.getElementById("tv-filterbar");
  if (!rows) {
    bar.replaceChildren();
    return;
  }
  const columnName = (index) =>
    rows.columns[index] ? rows.columns[index].name : "column " + (index + 1);
  const chips = view.filters.map((filter, index) =>
    el(
      "span",
      { className: "tv-chip" },
      el("strong", {}, columnName(filter.column)),
      " " + (operatorOf(filter.op) || { label: filter.op }).label,
      filter.value !== undefined && filter.value !== ""
        ? el("code", {}, filter.value)
        : null,
      el(
        "button",
        {
          type: "button",
          "aria-label": "Remove condition",
          onclick: () =>
            changeFilters(view.filters.filter((_, other) => other !== index)),
        },
        "×",
      ),
    ),
  );
  draft.column = Math.min(draft.column, rows.columns.length - 1);
  const value = el("input", {
    type: "text",
    className: "tv-filter-value",
    "aria-label": "Value",
    value: draft.value,
    hidden: operatorOf(draft.op).noValue,
    oninput: (event) => (draft.value = event.target.value),
    onkeydown: (event) => {
      if (event.key === "Enter") addFilter();
    },
  });
  const column = el(
    "select",
    {
      "aria-label": "Column",
      onchange: (event) => (draft.column = Number(event.target.value)),
    },
    rows.columns.map((column, index) =>
      el(
        "option",
        { value: index, selected: index === draft.column },
        column.name,
      ),
    ),
  );
  const operator = el(
    "select",
    {
      "aria-label": "Operator",
      onchange: (event) => {
        draft.op = event.target.value;
        value.hidden = !!operatorOf(draft.op).noValue;
      },
    },
    filterOperators.map((operator) =>
      el(
        "option",
        { value: operator.op, selected: operator.op === draft.op },
        operator.label,
      ),
    ),
  );
  setChildren(
    bar,
    el("span", { className: "tv-filter-label" }, "Filter"),
    chips,
    view.filters.length > 1
      ? el("span", { className: "tv-muted" }, "all must match")
      : null,
    el(
      "span",
      { className: "tv-filter-form" },
      column,
      operator,
      value,
      el("button", { type: "button", onclick: addFilter }, "+ Add"),
    ),
    view.filters.length
      ? el(
          "button",
          {
            type: "button",
            className: "tv-link-button",
            onclick: () => changeFilters([]),
          },
          "Clear all",
        )
      : null,
  );
}

function addFilter() {
  const filter = { column: draft.column, op: draft.op };
  if (!operatorOf(draft.op).noValue) filter.value = draft.value;
  draft.value = "";
  changeFilters(view.filters.concat(filter));
}

// toggleSort orders by a column, ascending, then descending, then not.
function toggleSort(column) {
  if (!view.sort || view.sort.column !== column) {
    view.sort = { column, desc: false };
  } else if (!view.sort.desc) {
    view.sort = { column, desc: true };
  } else {
    view.sort = null;
  }
  view.page = 1;
  loadRows();
}

function renderTable() {
  const table = document.getElementById("tv-table");
  if (!rows) {
    table.replaceChildren();
    return;
  }
  const head = el(
    "thead",
    {},
    el(
      "tr",
      {},
      el("th", { className: "tv-number", scope: "col" }, "#"),
      rows.columns.map((column, index) => {
        const sorted = view.sort && view.sort.column === index;
        const order = sorted
          ? view.sort.desc
            ? "descending"
            : "ascending"
          : "none";
        return el(
          "th",
          { scope: "col", "aria-sort": order },
          el(
            "button",
            {
              type: "button",
              className: "tv-sort",
              title: "Sort by " + column.name,
              onclick: () => toggleSort(index),
            },
            column.name,
            sorted ? (view.sort.desc ? " ▼" : " ▲") : "",
          ),
        );
      }),
    ),
    describeHead(),
  );
  const body = el(
    "tbody",
    {},
    rows.rows.map((row) =>
      el(
        "tr",
        {},
        el("td", { className: "tv-number" }, formatCount(row.number)),
        rows.columns.map((_, index) => el("td", {}, row.cells[index] ?? "")),
      ),
    ),
  );
  table.replaceChildren(head, body);
  if (!rows.rows.length) {
    body.append(
      el(
        "tr",
        {},
        el(
          "td",
          { className: "tv-empty", colSpan: rows.columns.length + 1 },
          "No rows.",
        ),
      ),
    );
  }
}

// describeHead is the Describe rows, under the column names.
function describeHead() {
  if (!view.describe || !description) return null;
  return describeRows.map(([name, show]) =>
    el(
      "tr",
      { className: "tv-describe" },
      el("th", { className: "tv-number", scope: "row" }, name),
      rows.columns.map((_, index) => {
        const stats = description.columns[index];
        return el("td", {}, stats ? show(stats) : "–");
      }),
    ),
  );
}

function goToPage(number) {
  view.page = Math.min(Math.max(1, number), pageCount());
  loadRows();
}

function renderPager() {
  const pager = document.getElementById("tv-pager");
  if (!rows) {
    pager.replaceChildren();
    return;
  }
  const first = rows.offset + 1;
  const last = rows.offset + rows.rows.length;
  const count = el(
    "span",
    { className: "tv-pager-count" },
    rows.rows.length
      ? "Rows " + formatCount(first) + "–" + formatCount(last) + " of "
      : "No rows of ",
    formatCount(rows.matched),
    rows.matched !== rows.total
      ? el(
          "span",
          { className: "tv-muted" },
          " matching (" + formatCount(rows.total) + " in all)",
        )
      : null,
    rows.cut
      ? el("span", { className: "tv-muted" }, " · long cells are shortened")
      : null,
  );
  const pages = pageCount();
  const button = (label, target, disabled) =>
    el(
      "button",
      { type: "button", disabled, onclick: () => goToPage(target) },
      label,
    );
  const input = el("input", {
    type: "number",
    className: "tv-page-input",
    min: 1,
    max: pages,
    value: view.page,
    "aria-label": "Page",
    onchange: (event) => goToPage(Number(event.target.value) || 1),
  });
  const buttons = el(
    "div",
    { className: "tv-pager-buttons" },
    button("« First", 1, view.page <= 1),
    button("‹ Previous", view.page - 1, view.page <= 1),
    el("label", {}, "Page", input, "of " + formatCount(pages)),
    button("Next ›", view.page + 1, view.page >= pages),
    button("Last »", pages, view.page >= pages),
  );
  const size = el(
    "label",
    { className: "tv-pager-size" },
    "Rows per page",
    el(
      "select",
      {
        onchange: (event) => {
          view.limit = Number(event.target.value);
          view.page = Math.floor(rows.offset / view.limit) + 1;
          loadRows();
        },
      },
      pageSizes.map((size) =>
        el(
          "option",
          { value: size, selected: size === view.limit },
          String(size),
        ),
      ),
    ),
  );
  pager.replaceChildren(count, buttons, size);
}

async function start() {
  readView();
  try {
    info = await fetchJSON(apiURL("api/info"));
  } catch (error) {
    showError(error.message);
    return;
  }
  if (!view.sep) view.sep = info.separator || ",";
  renderHeader();
  renderToolbar();
  await loadRows();
}

start();
