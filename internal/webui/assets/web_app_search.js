const historySearchFieldOptions = {
  project: [
    { target: "project", value: "project_name", label: "Project · Name" },
  ],
  run: [
    { target: "project", value: "project_name", label: "Project · Name" },
    { target: "run", value: "run_id", label: "Run · ID" },
    { target: "run", value: "run_name", label: "Run · Name" },
    { target: "run", value: "status", label: "Run · Status" },
    { target: "run", value: "exit_code", label: "Run · Exit code" },
  ],
  job: [
    { target: "project", value: "project_name", label: "Project · Name" },
    { target: "run", value: "run_id", label: "Run · ID" },
    { target: "run", value: "run_name", label: "Run · Name" },
    { target: "run", value: "status", label: "Run · Status" },
    { target: "run", value: "exit_code", label: "Run · Exit code" },
    { target: "job", value: "command", label: "Job · Command" },
    { target: "job", value: "status", label: "Job · Status" },
    { target: "job", value: "job_id", label: "Job · ID" },
    { target: "job", value: "job_name", label: "Job · Name" },
    { target: "job", value: "stage", label: "Job · Stage" },
    { target: "job", value: "executor", label: "Job · Executor" },
    { target: "job", value: "attempt_id", label: "Job · Attempt ID" },
    { target: "job", value: "exit_code", label: "Job · Exit code" },
  ],
};
let historySearchLastRequest = null;
let historySearchOffset = 0;
let historySearchFocusedJobURL = "";

const historySearchStatusOptions = {
  run: ["running", "finished", "failed", "unreadable"],
  job: [
    "pending",
    "running",
    "success",
    "success (accepted)",
    "failed",
    "cancelled",
    "blocked",
  ],
};

function historySearchConditionHTML(join = "and", target = "job") {
  return `<div class="history-search-condition">
    <select class="history-search-field" aria-label="Search field" onchange="historySearchUpdateValueControl(this)">${historySearchOptionsHTML(target)}</select>
    ${historySearchValueControl("job", "command")}
    <select class="history-search-join" aria-label="Combine condition" ${join === "first" ? "hidden" : ""}>
      <option value="and" ${join === "and" ? "selected" : ""}>AND</option>
      <option value="or" ${join === "or" ? "selected" : ""}>OR</option>
    </select>
    <button type="button" class="history-search-remove" aria-label="Remove condition" onclick="historySearchRemoveCondition(this)" hidden>−</button>
  </div>`;
}

function historySearchOptionsHTML(target, selected = "") {
  return (historySearchFieldOptions[target] || [])
    .map((field) => {
      const value = `${field.target}:${field.value}`;
      return `<option value="${value}" ${value === selected ? "selected" : ""}>${field.label}</option>`;
    })
    .join("");
}

function historySearchValueControl(target, field, selectedValue = "") {
  let options = [];
  if (field === "status") options = historySearchStatusOptions[target] || [];
  if (field === "executor") options = executorNames || [];
  if (!options.length)
    return `<input class="history-search-word" type="search" maxlength="256" placeholder="Search word" aria-label="Search word" value="${esc(selectedValue)}" required />`;
  const placeholder = field === "status" ? "Choose status" : "Choose executor";
  const optionHTML = [""]
    .concat(options)
    .map((value) => {
      const label = value || placeholder;
      return `<option value="${esc(value)}" ${value === selectedValue ? "selected" : ""}>${esc(label)}</option>`;
    })
    .join("");
  return `<select class="history-search-word" aria-label="Search value" required>${optionHTML}</select>`;
}

function historySearchUpdateValueControl(select) {
  const condition = select.closest(".history-search-condition");
  const current = condition.querySelector(".history-search-word");
  const [target, field] = select.value.split(":", 2);
  current.outerHTML = historySearchValueControl(target, field);
}

function historySearchUpdateTarget(select) {
  const conditions = document.getElementById("history-search-conditions");
  [...conditions.querySelectorAll(".history-search-condition")].forEach(
    (condition) => {
      const field = condition.querySelector(".history-search-field");
      const selected = field.value;
      field.innerHTML = historySearchOptionsHTML(select.value, selected);
      if (field.value !== selected && field.options.length)
        field.selectedIndex = 0;
      historySearchUpdateValueControl(field);
    },
  );
}

function historySearchAddCondition() {
  const conditions = document.getElementById("history-search-conditions");
  if (!conditions || conditions.children.length >= 20) return;
  const target = document.getElementById("history-search-target").value;
  conditions.insertAdjacentHTML(
    "beforeend",
    historySearchConditionHTML("and", target),
  );
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

function historySearchScopeHTML() {
  const currentBasedirID =
    mountedBasedirID ||
    registeredBasedirs.find((entry) => entry.current)?.id ||
    registeredBasedirs[0]?.id ||
    "";
  const basedirOptions = registeredBasedirs
    .map(
      (entry) =>
        `<option value="${esc(entry.id)}" ${entry.id === currentBasedirID ? "selected" : ""}>${esc(entry.path)}</option>`,
    )
    .join("");
  return `<div class="history-search-scope-row">
    <select class="history-search-scope-basedir" aria-label="Basedir" onchange="historySearchBasedirChanged(this)">
      <option value="">Choose basedir</option>${basedirOptions}
    </select>
    <select class="history-search-scope-project" aria-label="Project" onchange="historySearchProjectChanged(this)" disabled>
      <option value="">All projects</option>
    </select>
    <select class="history-search-scope-run" aria-label="Run" disabled>
      <option value="">All runs</option>
    </select>
    <button type="button" class="history-search-scope-remove" aria-label="Remove search range" onclick="historySearchRemoveScope(this)" hidden>−</button>
  </div>`;
}

function historySearchPageHTML() {
  return `<div class="history-search-page">
    <form id="history-search-form" class="history-search-form">
      <fieldset class="history-search-scope">
        <legend>Search scope</legend>
        <p class="history-search-scope-help">Choose a basedir, then optionally narrow the range to a project and run. Added ranges are searched together.</p>
        <div id="history-search-scopes">${historySearchScopeHTML()}</div>
        <button type="button" class="history-search-add-scope" onclick="historySearchAddScope()">＋ Add search range</button>
      </fieldset>
      <fieldset class="history-search-query">
        <legend>Conditions</legend>
        <div class="history-search-controls-row">
          <label class="history-search-result-target">Search for
            <select id="history-search-target" onchange="historySearchUpdateTarget(this)">
              <option value="project">Projects</option>
              <option value="run">Runs</option>
              <option value="job" selected>Jobs</option>
            </select>
          </label>
          <div class="history-search-match-options">
            <label><input id="history-search-ignore-case" type="checkbox" checked /> Ignore case</label>
            <label><input id="history-search-fuzzy" type="checkbox" /> Fuzzy search</label>
          </div>
        </div>
        <div id="history-search-conditions">${historySearchConditionHTML("first", "job")}</div>
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
  document.title = "History search · rotari";
  document.querySelector(".header-title-text").textContent =
    "rotari History search";
  document.getElementById("location").textContent = "History search";
  if (typeof rewriteStaticLinks === "function") {
    document.getElementById("page-title").textContent = "History search";
    document.getElementById("summary").textContent = "";
    app.className = "empty";
    app.textContent =
      "Cross-basedir history search is available in the live Web UI only.";
    return;
  }
  if (document.getElementById("history-search-form")) return;
  document.getElementById("page-title").textContent = "History search";
  document.getElementById("summary").textContent =
    "Search project, run, and job history in the ranges you choose.";
  app.className = "";
  app.innerHTML = historySearchPageHTML();
  document
    .getElementById("history-search-form")
    .addEventListener("submit", historySearchSubmit);
}

function historySearchAddScope() {
  const scopes = document.getElementById("history-search-scopes");
  if (!scopes) return;
  scopes.insertAdjacentHTML("beforeend", historySearchScopeHTML());
  historySearchUpdateScopeControls(scopes);
}

function historySearchRemoveScope(button) {
  const scopes = document.getElementById("history-search-scopes");
  button.closest(".history-search-scope-row").remove();
  historySearchUpdateScopeControls(scopes);
}

function historySearchUpdateScopeControls(scopes) {
  [...scopes.children].forEach((scope) => {
    scope.querySelector(".history-search-scope-remove").hidden =
      scopes.children.length === 1;
  });
}

async function historySearchBasedirChanged(select) {
  const row = select.closest(".history-search-scope-row");
  const project = row.querySelector(".history-search-scope-project");
  const run = row.querySelector(".history-search-scope-run");
  project.innerHTML = '<option value="">All projects</option>';
  run.innerHTML = '<option value="">All runs</option>';
  project.disabled = !select.value;
  run.disabled = true;
  if (!select.value) return;
  const selectedID = select.value;
  try {
    const response = await fetch(
      basedirURL(
        selectedID,
        "/api/history-search-options?basedir_id=" +
          encodeURIComponent(selectedID),
      ),
    );
    if (!response.ok || select.value !== selectedID) return;
    const options = await response.json();
    project.insertAdjacentHTML(
      "beforeend",
      (options.projects || [])
        .map((name) => `<option value="${esc(name)}">${esc(name)}</option>`)
        .join(""),
    );
  } catch (error) {
    // Keep the project selector empty if the selected basedir is unavailable.
  }
}

async function historySearchProjectChanged(select) {
  const row = select.closest(".history-search-scope-row");
  const basedir = row.querySelector(".history-search-scope-basedir");
  const run = row.querySelector(".history-search-scope-run");
  run.innerHTML = '<option value="">All runs</option>';
  run.disabled = !select.value;
  if (!select.value) return;
  const selectedProject = select.value;
  const selectedID = basedir.value;
  const query = new URLSearchParams({
    basedir_id: selectedID,
    project_name: selectedProject,
  });
  try {
    const response = await fetch(
      basedirURL(selectedID, "/api/history-search-options?" + query),
    );
    if (
      !response.ok ||
      basedir.value !== selectedID ||
      select.value !== selectedProject
    )
      return;
    const options = await response.json();
    run.insertAdjacentHTML(
      "beforeend",
      (options.runs || [])
        .map((item) => {
          const label = item.name
            ? `${item.name} · ${item.id}`
            : `${item.id}${item.status ? ` · ${item.status}` : ""}`;
          return `<option value="${esc(item.id)}">${esc(label)}</option>`;
        })
        .join(""),
    );
  } catch (error) {
    // Keep the run selector empty if the selected project is unavailable.
  }
}

function historySearchRequestFromForm() {
  const scopes = [...document.querySelectorAll(".history-search-scope-row")]
    .map((row) => ({
      basedir_id: row.querySelector(".history-search-scope-basedir").value,
      project_name: row.querySelector(".history-search-scope-project").value,
      run_id: row.querySelector(".history-search-scope-run").value,
    }))
    .filter((scope) => scope.basedir_id)
    .map((scope) => ({
      basedir_id: scope.basedir_id,
      ...(scope.project_name ? { project_name: scope.project_name } : {}),
      ...(scope.run_id ? { run_id: scope.run_id } : {}),
    }));
  const filters = [
    ...document.querySelectorAll(".history-search-condition"),
  ].map((condition, index) => {
    const [target, field] = condition
      .querySelector(".history-search-field")
      .value.split(":", 2);
    return {
      target,
      field,
      word: condition.querySelector(".history-search-word").value.trim(),
      ...(index > 0
        ? { join: condition.querySelector(".history-search-join").value }
        : {}),
    };
  });
  const request = {
    scopes,
    target: document.getElementById("history-search-target").value,
    case_sensitive: !document.getElementById("history-search-ignore-case")
      .checked,
    fuzzy: document.getElementById("history-search-fuzzy").checked,
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
  if (!historySearchLastRequest.scopes.length) {
    message.textContent = "Choose at least one search range.";
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
      const runPath =
        "/project/" +
        encodeURIComponent(row.project_name) +
        "/run/" +
        encodeURIComponent(row.run_id);
      const targetURL =
        row.target === "project"
          ? basedirURL(
              row.basedir_id,
              "/project/" + encodeURIComponent(row.project_name),
            )
          : basedirURL(
              row.basedir_id,
              row.target === "job"
                ? runPath + "?job_id=" + encodeURIComponent(row.job_id)
                : runPath,
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

function focusHistorySearchJob() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const jobID = new URLSearchParams(location.search).get("job_id");
  if (!jobID) return;
  const focusKey = location.pathname + location.search;
  if (historySearchFocusedJobURL === focusKey) return;
  const table = [...document.querySelectorAll("#app table.runs")].find(
    (item) => !item.closest(".web-queue-commands"),
  );
  if (!table) return;
  const rows = [...table.querySelectorAll("tbody tr[data-job-id]")];
  const index = rows.findIndex((row) => row.dataset.jobId === jobID);
  if (index < 0) return;
  paginationState.job.page = Math.floor(index / paginationPageSize);
  paginateTable(table, "job");
  const row = rows[index];
  row.classList.add("history-search-job-match");
  row.scrollIntoView({ behavior: "smooth", block: "center" });
  historySearchFocusedJobURL = focusKey;
}
