const historySearchFieldOptions = {
  project: [{ value: "project_name", label: "Project name" }],
  run: [
    { value: "run_id", label: "Run ID" },
    { value: "run_name", label: "Run name" },
    { value: "status", label: "Run status" },
    { value: "exit_code", label: "Run exit code" },
  ],
  job: [
    { value: "command", label: "Command" },
    { value: "status", label: "Job status" },
    { value: "job_id", label: "Job ID" },
    { value: "job_name", label: "Job name" },
    { value: "stage", label: "Stage" },
    { value: "executor", label: "Executor" },
    { value: "attempt_id", label: "Attempt ID" },
    { value: "exit_code", label: "Job exit code" },
  ],
};
const historySearchBasedirsKey =
  "rotari-history-search-basedirs:" +
  registeredBasedirs
    .map((entry) => entry.id)
    .sort()
    .join(",");
let historySearchLastRequest = null;
let historySearchOffset = 0;

function historySearchSelectedBasedirIDs() {
  const all = registeredBasedirs.map((entry) => entry.id);
  try {
    const stored = localStorage.getItem(historySearchBasedirsKey);
    if (stored === null) return new Set(all);
    const registered = new Set(all);
    return new Set(JSON.parse(stored).filter((id) => registered.has(id)));
  } catch (error) {
    // If local storage is unavailable or malformed, default to all registered basedirs.
    return new Set(all);
  }
}

function historySearchConditionHTML(join = "and") {
  const targets = [
    ["project", "Project"],
    ["run", "Run"],
    ["job", "Job"],
  ];
  const targetOptions = targets
    .map(
      ([value, label]) =>
        `<option value="${value}" ${value === "job" ? "selected" : ""}>${label}</option>`,
    )
    .join("");
  return `<div class="history-search-condition">
    <select class="history-search-join" aria-label="Combine condition" ${join === "first" ? "hidden" : ""}>
      <option value="and" ${join === "and" ? "selected" : ""}>AND</option>
      <option value="or" ${join === "or" ? "selected" : ""}>OR</option>
    </select>
    <select class="history-search-target" aria-label="Search target" onchange="historySearchUpdateFields(this)">${targetOptions}</select>
    <select class="history-search-field" aria-label="Search field">${historySearchOptionsHTML("job")}</select>
    <input class="history-search-word" type="search" maxlength="256" placeholder="Search word" aria-label="Search word" required />
    <button type="button" class="history-search-remove" aria-label="Remove condition" onclick="historySearchRemoveCondition(this)" hidden>−</button>
  </div>`;
}

function historySearchOptionsHTML(target, selected = "") {
  return (historySearchFieldOptions[target] || [])
    .map(
      (field) =>
        `<option value="${field.value}" ${field.value === selected ? "selected" : ""}>${field.label}</option>`,
    )
    .join("");
}

function historySearchUpdateFields(select) {
  const condition = select.closest(".history-search-condition");
  condition.querySelector(".history-search-field").innerHTML =
    historySearchOptionsHTML(select.value);
}

function historySearchAddCondition() {
  const conditions = document.getElementById("history-search-conditions");
  if (!conditions || conditions.children.length >= 20) return;
  conditions.insertAdjacentHTML("beforeend", historySearchConditionHTML("and"));
  historySearchUpdateConditionControls(conditions);
}

function historySearchRemoveCondition(button) {
  const conditions = document.getElementById("history-search-conditions");
  button.closest(".history-search-condition").remove();
  historySearchUpdateConditionControls(conditions);
}

function historySearchUpdateConditionControls(conditions) {
  [...conditions.children].forEach((condition, index) => {
    condition.querySelector(".history-search-join").hidden = index === 0;
    condition.querySelector(".history-search-remove").hidden =
      conditions.children.length === 1;
  });
}

function historySearchToggleCustomRange(select) {
  const custom = document.getElementById("history-search-custom-range");
  if (custom) custom.hidden = select.value !== "custom";
}

function historySearchBasedirsHTML() {
  const selected = historySearchSelectedBasedirIDs();
  return registeredBasedirs
    .map(
      (entry) => `<label class="history-search-basedir">
        <input type="checkbox" name="basedir" value="${esc(entry.id)}" ${selected.has(entry.id) ? "checked" : ""} />
        <span title="${esc(entry.path)}">${esc(entry.path)}</span>
      </label>`,
    )
    .join("");
}

function historySearchPageHTML() {
  return `<div class="history-search-page">
    <p class="history-search-intro">Search project, run, and job history across registered basedirs.</p>
    <form id="history-search-form" class="history-search-form">
      <fieldset class="history-search-scope">
        <legend>Search scope</legend>
        <div class="history-search-basedirs">${historySearchBasedirsHTML()}</div>
        <div class="history-search-scope-actions">
          <button type="button" onclick="historySearchSetAllBasedirs(true)">Select all</button>
          <button type="button" onclick="historySearchSetAllBasedirs(false)">Clear</button>
        </div>
      </fieldset>
      <fieldset class="history-search-query">
        <legend>Conditions</legend>
        <div id="history-search-conditions">${historySearchConditionHTML("first")}</div>
        <button type="button" class="history-search-add" onclick="historySearchAddCondition()">＋ Add condition</button>
      </fieldset>
      <div class="history-search-time-row">
        <label>Time range
          <select id="history-search-time-range" onchange="historySearchToggleCustomRange(this)">
            <option value="24h" selected>Last 24 hours</option>
            <option value="7d">Last 7 days</option>
            <option value="30d">Last 30 days</option>
            <option value="all">All history</option>
            <option value="custom">Custom range</option>
          </select>
        </label>
        <div id="history-search-custom-range" class="history-search-custom-range" hidden>
          <label>From <input id="history-search-from" type="datetime-local" /></label>
          <label>To <input id="history-search-to" type="datetime-local" /></label>
        </div>
        <button class="primary" type="submit">Search</button>
      </div>
    </form>
    <div id="history-search-message" class="history-search-message" role="status"></div>
    <div id="history-search-results"></div>
    <div id="history-search-pagination" class="history-search-pagination" hidden>
      <button type="button" onclick="historySearchPage(-1)">Previous</button>
      <span id="history-search-page-label"></span>
      <button type="button" onclick="historySearchPage(1)">Next</button>
    </div>
  </div>`;
}

function renderHistorySearchPage() {
  const app = document.getElementById("app");
  if (typeof rewriteStaticLinks === "function") {
    document.getElementById("page-title").textContent = "History search";
    document.getElementById("summary").textContent = "";
    app.className = "empty";
    app.textContent =
      "Cross-basedir history search is available in the live Web UI only.";
    return;
  }
  if (document.getElementById("history-search-form")) return;
  document.title = "History search · rotari";
  document.getElementById("page-title").textContent = "History search";
  document.getElementById("summary").textContent =
    "Across projects and registered basedirs";
  app.className = "";
  app.innerHTML = historySearchPageHTML();
  document
    .getElementById("history-search-form")
    .addEventListener("submit", historySearchSubmit);
  document
    .querySelectorAll('.history-search-basedirs input[name="basedir"]')
    .forEach((checkbox) =>
      checkbox.addEventListener("change", historySearchSaveBasedirs),
    );
}

function historySearchSetAllBasedirs(checked) {
  document
    .querySelectorAll('.history-search-basedirs input[name="basedir"]')
    .forEach((checkbox) => (checkbox.checked = checked));
  historySearchSaveBasedirs();
}

function historySearchSaveBasedirs() {
  const selected = [
    ...document.querySelectorAll(
      '.history-search-basedirs input[name="basedir"]:checked',
    ),
  ].map((checkbox) => checkbox.value);
  localStorage.setItem(historySearchBasedirsKey, JSON.stringify(selected));
}

function historySearchRequestFromForm() {
  const filters = [
    ...document.querySelectorAll(".history-search-condition"),
  ].map((condition, index) => ({
    target: condition.querySelector(".history-search-target").value,
    field: condition.querySelector(".history-search-field").value,
    word: condition.querySelector(".history-search-word").value.trim(),
    ...(index > 0
      ? { join: condition.querySelector(".history-search-join").value }
      : {}),
  }));
  const request = {
    basedir_ids: [
      ...document.querySelectorAll(
        '.history-search-basedirs input[name="basedir"]:checked',
      ),
    ].map((checkbox) => checkbox.value),
    filters,
    limit: 50,
    offset: 0,
  };
  const range = document.getElementById("history-search-time-range").value;
  if (range !== "all" && range !== "custom") {
    const duration = {
      "24h": 24 * 60 * 60 * 1000,
      "7d": 7 * 86400000,
      "30d": 30 * 86400000,
    }[range];
    request.from = new Date(Date.now() - duration).toISOString();
    request.to = new Date().toISOString();
  } else if (range === "custom") {
    const from = document.getElementById("history-search-from").value;
    const to = document.getElementById("history-search-to").value;
    if (from) request.from = new Date(from).toISOString();
    if (to) {
      const end = new Date(to);
      end.setSeconds(59, 999);
      request.to = end.toISOString();
    }
  }
  return request;
}

async function historySearchSubmit(event) {
  event.preventDefault();
  historySearchLastRequest = historySearchRequestFromForm();
  historySearchOffset = 0;
  await historySearchExecute();
}

async function historySearchPage(direction) {
  if (!historySearchLastRequest) return;
  historySearchOffset = Math.max(0, historySearchOffset + direction * 50);
  await historySearchExecute();
}

async function historySearchExecute() {
  const message = document.getElementById("history-search-message");
  const results = document.getElementById("history-search-results");
  const pagination = document.getElementById("history-search-pagination");
  message.textContent = "Searching…";
  pagination.hidden = true;
  results.replaceChildren();
  if (!historySearchLastRequest.basedir_ids.length) {
    message.textContent = "Select at least one basedir to search.";
    return;
  }
  const request = {
    ...historySearchLastRequest,
    offset: historySearchOffset,
  };
  try {
    const response = await fetch("/api/history-search", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(request),
    });
    if (!response.ok) {
      message.textContent = await response.text();
      return;
    }
    const data = await response.json();
    message.textContent = `${data.total} result${data.total === 1 ? "" : "s"}`;
    historySearchRenderResults(data.rows || []);
    pagination.hidden = data.total <= data.limit;
    document.getElementById("history-search-page-label").textContent =
      `${data.offset + 1}–${Math.min(data.offset + data.limit, data.total)} of ${data.total}`;
    pagination.querySelector("button").disabled = data.offset === 0;
    pagination.querySelectorAll("button")[1].disabled =
      data.offset + data.limit >= data.total;
  } catch (error) {
    // Report transport and response errors inline so the user can retry.
    message.textContent =
      "History search failed. Check the Web server connection and try again.";
  }
}

function historySearchRenderResults(rows) {
  const container = document.getElementById("history-search-results");
  if (!rows.length) {
    container.innerHTML =
      '<p class="history-search-empty">No matching history found.</p>';
    return;
  }
  const body = rows
    .map((row) => {
      const targetURL =
        row.target === "project"
          ? basedirURL(
              row.basedir_id,
              "/project/" + encodeURIComponent(row.project_name),
            )
          : basedirURL(
              row.basedir_id,
              "/project/" +
                encodeURIComponent(row.project_name) +
                "/run/" +
                encodeURIComponent(row.run_id),
            );
      let title;
      let details;
      if (row.target === "project") {
        title = row.project_name;
        details = row.basedir_path;
      } else if (row.target === "run") {
        title = row.run_name || row.run_id;
        details = row.project_name;
      } else {
        title = row.job_name || row.job_id;
        details = `${row.project_name} / ${row.run_name || row.run_id} · ${row.job_id} · ${row.command || ""}`;
      }
      const status = row.job_status || row.run_status || "";
      const timestamp = row.timestamp
        ? new Date(row.timestamp).toLocaleString()
        : "";
      return `<tr>
        <td><span class="history-search-kind">${esc(row.target)}</span></td>
        <td><a href="${esc(targetURL)}">${esc(title)}</a><div class="history-search-detail">${esc(details)}</div></td>
        <td>${esc(status)}</td>
        <td>${esc(timestamp)}</td>
      </tr>`;
    })
    .join("");
  container.innerHTML = `<div class="table-scroll"><table class="history-search-table">
    <thead><tr><th>Type</th><th>Match</th><th>Status</th><th>Time</th></tr></thead>
    <tbody>${body}</tbody>
  </table></div>`;
}
