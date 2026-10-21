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

let info = null;
let rows = null;
// view holds the settings the fragment keeps.
const view = { sep: "", header: true, page: 1, limit: 100 };

// el builds an element: attributes set as properties (or with setAttribute
// for names with a dash), children appended as nodes or text.
function el(tag, attributes, ...children) {
  const node = document.createElement(tag);
  Object.entries(attributes || {}).forEach(([name, value]) => {
    if (value === undefined || value === null || value === false) return;
    if (name.includes("-")) node.setAttribute(name, String(value));
    else node[name] = value;
  });
  children.flat().forEach((child) => {
    if (child === null || child === undefined || child === false) return;
    node.append(child instanceof Node ? child : String(child));
  });
  return node;
}

function formatCount(value) {
  return Number(value).toLocaleString("en-US");
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
}

function writeView() {
  const fragment = new URLSearchParams();
  fragment.set("sep", view.sep);
  fragment.set("header", view.header ? "1" : "0");
  fragment.set("page", String(view.page));
  fragment.set("limit", String(view.limit));
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
  toolbar.replaceChildren(
    el("label", {}, el("span", {}, "Separator"), select, other),
    el("label", {}, header, el("span", {}, "First row is a header")),
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
  renderToolbar();
  loadRows();
}

function pageCount() {
  return Math.max(1, Math.ceil(rows.matched / view.limit));
}

async function loadRows() {
  writeView();
  try {
    rows = await fetchJSON(
      apiURL("api/rows", {
        sep: view.sep,
        header: view.header ? "1" : "0",
        offset: (view.page - 1) * view.limit,
        limit: view.limit,
      }),
    );
    showError("");
  } catch (error) {
    rows = null;
    showError(error.message);
  }
  if (rows && view.page > pageCount()) {
    view.page = pageCount();
    return loadRows();
  }
  renderSummary();
  renderTable();
  renderPager();
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
      rows.columns.map((column) => el("th", { scope: "col" }, column.name)),
    ),
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
