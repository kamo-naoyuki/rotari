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
  ["mean", (stats, column) => formatValue(stats.mean, column)],
  ["std", (stats, column) => formatValue(stats.std, column)],
  ["min", (stats, column) => formatValue(stats.min, column)],
  ["max", (stats, column) => formatValue(stats.max, column)],
  ["sum", (stats, column) => formatValue(stats.sum, column)],
];
// How a directory is split into a stems view's groups.
const groupChoices = [
  { value: "", label: "automatic" },
  { value: "top", label: "first-level directory" },
  { value: "parent", label: "parent directory" },
  { value: "extension", label: "file extension" },
  { value: "suffix", label: "suffix after a separator" },
];

let info = null;
let rows = null;
let description = null;
// view holds the settings the fragment keeps. A filter or sort names a
// column by its position, counted from 0.
const view = {
  base: "",
  sep: "",
  // A directory's listing.
  listing: "files",
  pattern: "**/*",
  group: "",
  suffixSep: "_",
  incomplete: false,
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

// formatValue writes a number in its column's units.
function formatValue(value, column) {
  if (value === undefined || value === null) return "–";
  if (column && column.type === "duration") return formatDuration(value);
  if (column && column.type === "bytes") return formatBytes(Math.round(value));
  return formatNumber(value);
}

// formatCell writes a cell of a typed column for reading: durations,
// sizes, and times; other cells are shown as they are.
function formatCell(text, column) {
  if (text === "" || !column || !column.type) return text;
  if (column.type === "time") return text.replace("T", " ").replace("Z", "");
  const value = Number(text);
  if (!Number.isFinite(value)) return text;
  if (column.type === "duration") return formatDuration(value);
  if (column.type === "bytes") return formatBytes(value);
  if (column.type === "number") return formatCount(value);
  return text;
}

// formatDuration writes seconds as m:ss.cc, or h:mm:ss past an hour.
function formatDuration(seconds) {
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const rest = seconds % 60;
  if (hours) {
    return (
      hours +
      ":" +
      String(minutes).padStart(2, "0") +
      ":" +
      String(Math.floor(rest)).padStart(2, "0")
    );
  }
  return minutes + ":" + rest.toFixed(2).padStart(5, "0");
}

function formatRate(rate) {
  return rate % 1000 ? (rate / 1000).toFixed(1) + " kHz" : rate / 1000 + " kHz";
}

// formatProperties is one line for a media file, such as
// "WAV PCM 16-bit · 0:03.12 · 16 kHz · 1 ch · 256 kbps · 97.5 KiB".
function formatProperties(properties) {
  const parts = [
    properties.format +
      (properties.bit_depth ? " " + properties.bit_depth + "-bit" : ""),
  ];
  if (properties.duration) parts.push(formatDuration(properties.duration));
  if (properties.width && properties.height) {
    parts.push(properties.width + "×" + properties.height);
  }
  if (properties.sample_rate) parts.push(formatRate(properties.sample_rate));
  if (properties.channels) parts.push(properties.channels + " ch");
  if (properties.bitrate) {
    parts.push(Math.round(properties.bitrate / 1000) + " kbps");
  }
  parts.push(formatBytes(properties.size));
  return parts.join(" · ");
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
  if (fragment.get("listing") === "stems") view.listing = "stems";
  if (fragment.has("pattern")) view.pattern = fragment.get("pattern");
  if (fragment.has("group")) view.group = fragment.get("group");
  if (fragment.has("suffix_sep")) view.suffixSep = fragment.get("suffix_sep");
  view.incomplete = fragment.get("incomplete") === "1";
  if (fragment.has("base")) view.base = fragment.get("base");
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
  if (info.kind === "directory") {
    fragment.set("listing", view.listing);
    fragment.set("pattern", view.pattern);
    if (view.listing === "stems") {
      if (view.group) fragment.set("group", view.group);
      if (view.group === "suffix") fragment.set("suffix_sep", view.suffixSep);
      if (view.incomplete) fragment.set("incomplete", "1");
    }
  } else {
    fragment.set("sep", view.sep);
    if (view.base) fragment.set("base", view.base);
    fragment.set("header", view.header ? "1" : "0");
  }
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
    const noun =
      info.kind === "file"
        ? " rows"
        : view.listing === "stems"
          ? " stems"
          : " files";
    parts.push(formatCount(rows.total) + noun);
    parts.push(formatCount(rows.columns.length) + " columns");
  }
  document.getElementById("tv-summary").textContent = parts.join(" · ");
}

// describeToggle is the button that shows or hides Describe.
function describeToggle() {
  return el(
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
}

// changeListing applies a change to how a directory is listed, which makes
// other columns: filters and sort name them by position.
function changeListing(change) {
  Object.assign(view, change);
  view.page = 1;
  view.filters = [];
  view.sort = null;
  renderToolbar();
  loadRows();
}

// renderDirectoryToolbar lays out a directory's choices: files or stems,
// the pattern, and how stems are grouped.
function renderDirectoryToolbar(toolbar) {
  const listingButton = (value, label) =>
    el(
      "button",
      {
        type: "button",
        "aria-pressed": String(view.listing === value),
        onclick: () => changeListing({ listing: value }),
      },
      label,
    );
  const pattern = el("input", {
    type: "text",
    className: "tv-pattern",
    "aria-label": "Pattern",
    value: view.pattern,
    title: "* matches within a name, ** any directories, {a,b} either",
    onchange: (event) =>
      changeListing({ pattern: event.target.value || "**/*" }),
  });
  const stems = view.listing === "stems";
  const group = el(
    "select",
    {
      "aria-label": "Groups from",
      onchange: (event) => changeListing({ group: event.target.value }),
    },
    groupChoices.map((choice) =>
      el(
        "option",
        { value: choice.value, selected: choice.value === view.group },
        choice.label,
      ),
    ),
  );
  const suffix = el("input", {
    type: "text",
    className: "tv-separator-other",
    "aria-label": "Suffix separator",
    value: view.suffixSep,
    onchange: (event) =>
      changeListing({ suffixSep: event.target.value || "_" }),
  });
  const incomplete = el("input", {
    type: "checkbox",
    checked: view.incomplete,
    onchange: (event) => changeListing({ incomplete: event.target.checked }),
  });
  setChildren(
    toolbar,
    el(
      "span",
      { className: "tv-segmented", role: "group", "aria-label": "View" },
      listingButton("files", "Files"),
      listingButton("stems", "Group by stem"),
    ),
    el("label", {}, el("span", {}, "Pattern"), pattern),
    stems
      ? el(
          "label",
          {},
          el("span", {}, "Groups from"),
          group,
          view.group === "suffix" ? suffix : null,
        )
      : null,
    stems
      ? el(
          "label",
          {},
          incomplete,
          el("span", {}, "Only stems missing in some group"),
        )
      : null,
    el("span", { className: "tv-spacer" }),
    describeToggle(),
  );
}

function renderToolbar() {
  const toolbar = document.getElementById("tv-toolbar");
  if (info.kind === "directory") {
    renderDirectoryToolbar(toolbar);
    return;
  }
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
  const bases = info.bases || [];
  const base = el(
    "select",
    {
      "aria-label": "Resolve media paths from",
      onchange: (event) => {
        view.base = event.target.value;
        loadRows();
      },
    },
    bases.map((base, index) =>
      el(
        "option",
        {
          value: base.id,
          selected: view.base ? base.id === view.base : index === 0,
        },
        base.label,
      ),
    ),
  );
  setChildren(
    toolbar,
    el("label", {}, el("span", {}, "Separator"), select, other),
    el("label", {}, header, el("span", {}, "First row is a header")),
    bases.length > 1
      ? el("label", {}, el("span", {}, "Resolve media paths from"), base)
      : null,
    el("span", { className: "tv-spacer" }),
    describeToggle(),
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
  const filters = view.filters.length
    ? JSON.stringify(view.filters)
    : undefined;
  if (info.kind === "directory") {
    const stems = view.listing === "stems";
    return {
      view: view.listing,
      pattern: view.pattern,
      group: stems && view.group ? view.group : undefined,
      suffix_sep: stems && view.group === "suffix" ? view.suffixSep : undefined,
      incomplete: stems && view.incomplete ? "1" : undefined,
      filters,
    };
  }
  return {
    base: view.base || undefined,
    sep: view.sep,
    header: view.header ? "1" : "0",
    filters,
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
  // A stems view's rows open all their files side by side.
  const rowActions = info.kind === "directory" && view.listing === "stems";
  const head = el(
    "thead",
    {},
    el(
      "tr",
      {},
      el("th", { className: "tv-number", scope: "col" }, "#"),
      rowActions ? el("th", { scope: "col" }, "") : null,
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
    describeHead(rowActions),
  );
  const body = el(
    "tbody",
    {},
    rows.rows.map((row) =>
      el(
        "tr",
        {},
        el("td", { className: "tv-number" }, formatCount(row.number)),
        rowActions ? el("td", {}, openRowButton(row)) : null,
        rows.columns.map((_, index) =>
          el(
            "td",
            {},
            row.media && row.media[index]
              ? mediaCell(row.media[index])
              : cellContent(row.cells[index] ?? "", rows.columns[index]),
          ),
        ),
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
          {
            className: "tv-empty",
            colSpan: rows.columns.length + (rowActions ? 2 : 1),
          },
          "No rows.",
        ),
      ),
    );
  }
}

// openRowButton opens a row's files in panes side by side, to compare them.
function openRowButton(row) {
  const media = Object.keys(row.media || {})
    .map(Number)
    .sort((a, b) => a - b)
    .map((index) => row.media[index])
    .filter((media) => !media.error);
  return media.length
    ? el(
        "button",
        {
          type: "button",
          className: "tv-open-row",
          title: "Open this row's files in panes side by side",
          onclick: () => openMediaRow(media),
        },
        "Open row",
      )
    : null;
}

// cellContent is a cell that names no media: a path of a stems view's
// group that has no file for the stem says so.
function cellContent(text, column) {
  if (column.type === "path") {
    return text
      ? el("span", { className: "tv-media-path" }, text)
      : el("span", { className: "tv-missing" }, "— missing");
  }
  return formatCell(text, column);
}

// mediaURL requests the file a cell names, as the server resolved it.
function mediaURL(media) {
  return apiURL("api/media", { ref: media.ref, base: media.base || undefined });
}

// mediaCell shows a path that names an image, audio, or video file: the
// path, which opens the file, and its properties, or why it is not shown.
function mediaCell(media) {
  return el(
    "span",
    { className: "tv-media" },
    media.error
      ? el("span", { className: "tv-media-path" }, media.ref)
      : el(
          "a",
          {
            className: "tv-media-path",
            href: mediaURL(media),
            target: "_blank",
            rel: "noopener",
            title: "Open in the active pane",
            // A plain click opens the file in a pane; with a modifier the
            // browser opens the link as usual.
            onclick: (event) => {
              if (
                event.ctrlKey ||
                event.metaKey ||
                event.shiftKey ||
                event.button
              )
                return;
              event.preventDefault();
              openMedia(media);
            },
          },
          media.ref,
        ),
    el(
      "span",
      { className: media.error ? "tv-media-error" : "tv-media-properties" },
      media.error || formatProperties(media.properties),
    ),
  );
}

// describeHead is the Describe rows, under the column names.
function describeHead(rowActions) {
  if (!view.describe || !description) return null;
  return describeRows.map(([name, show]) =>
    el(
      "tr",
      { className: "tv-describe" },
      el("th", { className: "tv-number", scope: "row" }, name),
      rowActions ? el("td", {}) : null,
      rows.columns.map((_, index) => {
        const stats = description.columns[index];
        return el("td", {}, stats ? show(stats, rows.columns[index]) : "–");
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
    (rows.notes || []).map((note) =>
      el("span", { className: "tv-muted" }, " · " + note),
    ),
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
  startPanesHandle();
  renderHeader();
  renderToolbar();
  await loadRows();
}

start();
