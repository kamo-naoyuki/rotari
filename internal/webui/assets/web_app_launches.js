// Launching the commands registered in launchers.toml against an artifact,
// and the server-wide list of launches. The server decides what may run,
// resolves every path, and words every explanation; this file only lays
// them out. Nothing runs without the Launch button in the confirmation.
let launcherAvailability = null;
// selectedLaunchID is the launch whose output the Launches view shows.
let selectedLaunchID = "";
let launchListTimer = null;

// loadLauncherAvailability asks whether this server can launch, which
// shows the Launches and job Launch buttons.
async function loadLauncherAvailability() {
  try {
    const response = await fetch("/api/launchers");
    launcherAvailability = response.ok
      ? await response.json()
      : {
          available: false,
          reason: "Cannot check launching",
          hint: await response.text(),
        };
  } catch (error) {
    launcherAvailability = {
      available: false,
      reason: "Cannot check launching: " + error.message,
    };
  }
  const button = document.getElementById("launches-button");
  if (button) button.hidden = !launcherAvailability.available;
  return launcherAvailability;
}

// launchExplanation shows the server's reason and how to fix it.
function launchExplanation(reason, hint) {
  return (
    '<p class="launch-reason">' +
    esc(reason || "") +
    "</p>" +
    (hint ? '<p class="meta launch-hint">' + esc(hint) + "</p>" : "")
  );
}

// artifactLaunchButtons is a placeholder for a row's, or a preview title
// line's, buttons: one per viewer and registered command that accepts the
// artifact. fillArtifactLaunchButtons asks the server which those are.
function artifactLaunchButtons(index, child, type) {
  const target = child || artifactEntry(index).path;
  return (
    '<span class="artifact-launchers" data-index="' +
    esc(String(index)) +
    '" data-child="' +
    esc(child || "") +
    '" data-kind="' +
    (type === "directory" ? "directory" : "file") +
    '" data-name="' +
    esc(target.slice(target.lastIndexOf("/") + 1)) +
    '"></span>'
  );
}

// fillArtifactLaunchButtons fills the placeholders under container with
// one /api/launchers request. A command whose program is missing is shown
// disabled with the server's reason. When this server cannot launch, the
// rows keep only the viewers and the server's reason tops the view.
async function fillArtifactLaunchButtons(container) {
  const holders = [...container.querySelectorAll(".artifact-launchers")];
  if (!holders.length) return;
  const params = new URLSearchParams();
  holders.forEach((holder) =>
    params.append("target", holder.dataset.kind + ":" + holder.dataset.name),
  );
  let availability;
  try {
    const response = await fetch("/api/launchers?" + params);
    availability = response.ok
      ? await response.json()
      : {
          available: false,
          reason: "Cannot check launching",
          hint: await response.text(),
        };
  } catch (error) {
    availability = {
      available: false,
      reason: "Cannot check launching: " + error.message,
    };
  }
  const commands = new Map(
    (availability.commands || []).map((command) => [command.name, command]),
  );
  const targets = availability.targets || [];
  holders.forEach((holder, position) => {
    const index = holder.dataset.index;
    const entry = /^\d+$/.test(index) ? Number(index) : index;
    const child = holder.dataset.child;
    const match = targets[position] || { commands: [] };
    holder.innerHTML =
      (match.viewers || [])
        .map(
          (viewer) =>
            '<a class="launch-viewer" target="_blank" rel="noopener" href="' +
            esc(
              appURL(viewer.path) +
                "?" +
                artifactParams(selectedArtifacts, { entry: index, child }),
            ) +
            '">' +
            esc(viewer.name) +
            "</a>",
        )
        .join("") +
      match.commands
        .map((name) => {
          const unavailable = (commands.get(name) || {}).unavailable;
          return unavailable
            ? '<button type="button" class="artifact-launch-command" disabled title="' +
                esc(name + ": " + unavailable) +
                '">' +
                esc(name) +
                "</button>"
            : '<button type="button" class="artifact-launch-command" title="' +
                esc("Launch " + name + " on " + artifactTitle(entry, child)) +
                '" onclick="chooseArtifactLaunch(' +
                jsArg(entry) +
                "," +
                jsArg(child) +
                "," +
                jsArg(name) +
                ',this)">' +
                esc(name) +
                "</button>";
        })
        .join("");
  });
  const view = artifactView();
  view.querySelector(".launch-unavailable-note")?.remove();
  if (!availability.available && availability.reason) {
    const note = document.createElement("div");
    note.className = "launch-unavailable-note";
    note.innerHTML = launchExplanation(availability.reason, availability.hint);
    view.prepend(note);
  }
}

// launchOnWorkingDirectory opens a job's Artifacts and points at the
// commands for its working directory, the `.` row; when that row cannot be
// launched on, the view says why instead.
async function launchOnWorkingDirectory(queue, run, job, attemptID) {
  await showArtifacts(queue, run, job, attemptID);
  const row = artifactView().querySelector(
    'tr[data-entry="' + WORKING_DIRECTORY_ENTRY + '"]',
  );
  if (row && row.querySelector(".artifact-launchers")) {
    row.classList.add("artifact-row-focus");
    row.scrollIntoView({ block: "nearest" });
    return;
  }
  const listing = selectedArtifacts.listing;
  if (!listing) return;
  const note = document.createElement("p");
  note.className = "launch-reason";
  note.textContent = !listing.working_directory_type
    ? "This attempt recorded no working directory to launch on."
    : listing.working_directory_type !== "directory"
      ? "The working directory " +
        listing.working_directory +
        " is " +
        listing.working_directory_type +
        " on the Web server's host, so nothing can be launched on it."
      : "The working directory " +
        listing.working_directory +
        " is not served here, so nothing can be launched on it.";
  artifactView().prepend(note);
}

function basedirParent() {
  const base = state.base_dir.replace(/\/+$/, "");
  return base.slice(0, base.lastIndexOf("/")) || "/";
}

// runFilesButton is the run toolbar's Files button, which browses the
// directory holding the basedir, where a run's jobs often share one
// tracker store or TensorBoard log directory. A static export holds no
// such files.
function runFilesButton(project, runID) {
  if (typeof window.__ROTARI_STATIC_STATE__ !== "undefined") return "";
  return (
    '<button type="button" class="run-files" title="' +
    esc("Browse " + basedirParent() + " and launch commands on its files") +
    '" onclick="showRunFiles(' +
    jsArg(project) +
    "," +
    jsArg(runID) +
    ')">Files</button>'
  );
}

// showRunFiles opens the Files view: the directory holding the basedir,
// browsed and launched on like a job's listed directory. The server
// resolves the directory from the run and decides whether it is served.
async function showRunFiles(queue, run) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "artifacts";
  modal.querySelector("strong").textContent = "Files";
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  selectedArtifacts = {
    queue,
    run,
    job: "",
    attemptID: "",
    listing: null,
    basedirParent: basedirParent(),
  };
  selectedOutput = selectedArtifacts.basedirParent;
  artifactView().innerHTML =
    '<div id="artifact-preview" class="artifact-preview"></div>';
  openOutputModal(false);
  await openArtifact(BASEDIR_PARENT_ENTRY, "", "directory");
}

function artifactLaunchPanel() {
  return document.getElementById("artifact-launch-panel");
}

// closeArtifactLaunch removes the panel, and the table row that holds it
// when it was opened from a row.
function closeArtifactLaunch() {
  const panel = artifactLaunchPanel();
  if (!panel) return;
  (panel.closest(".artifact-launch-row") || panel).remove();
}

// chooseArtifactLaunch opens the confirmation of launching a command on
// an artifact: from a table row, in a row below it; otherwise after the
// preview's title line. The same button closes it; another one moves it.
async function chooseArtifactLaunch(index, child, name, button) {
  const key = index + "/" + (child || "") + "/" + name;
  const row = button && button.closest("tr");
  const open = artifactLaunchPanel();
  closeArtifactLaunch();
  if (open && open.dataset.key === key && open.dataset.row === String(!!row))
    return;
  const panel = document.createElement("div");
  panel.id = "artifact-launch-panel";
  panel.className = "artifact-launch-panel";
  panel.dataset.key = key;
  panel.dataset.row = String(!!row);
  if (row) {
    const holder = document.createElement("tr");
    holder.className = "artifact-launch-row";
    const cell = document.createElement("td");
    cell.colSpan = row.children.length;
    cell.append(panel);
    holder.append(cell);
    row.after(holder);
  } else {
    artifactPreview().querySelector(".artifact-preview-title").after(panel);
  }
  await previewArtifactLaunch(index, child, name);
}

function launchRequestBody(index, child, name, newInstance) {
  return {
    project_name: selectedArtifacts.queue,
    run_id: selectedArtifacts.run,
    job_id: selectedArtifacts.job,
    attempt_id: selectedArtifacts.attemptID || "",
    entry: index,
    child: child || "",
    name,
    new_instance: !!newInstance,
  };
}

// previewArtifactLaunch asks the server what a start would run and shows it
// for confirmation.
async function previewArtifactLaunch(index, child, name) {
  const panel = artifactLaunchPanel();
  panel.innerHTML = '<p class="meta">Loading...</p>';
  const response = await fetch(
    "/api/launch-preview?" +
      artifactParams(selectedArtifacts, { entry: String(index), child, name }),
  );
  const body = await response
    .json()
    .catch(() => ({ error: "HTTP " + response.status }));
  if (!response.ok) {
    panel.innerHTML = launchExplanation(body.error, body.hint);
    return;
  }
  const origin = ["project", "run", "job", "attempt"]
    .filter((key) => body.labels && body.labels[key])
    .map((key) => key + " " + body.labels[key])
    .join(", ");
  const rows = [
    ["Command", body.name],
    ["Target", body.target + " (" + body.kind + ")"],
    ["Working directory", body.cwd],
    ["Mode", body.mode],
    ["Runs as", body.user + "@" + body.host],
    ["From", origin],
  ];
  panel.innerHTML =
    '<dl class="launch-details">' +
    rows
      .map(
        ([label, value]) =>
          "<dt>" + esc(label) + "</dt><dd>" + esc(value) + "</dd>",
      )
      .join("") +
    '<dt>Runs</dt><dd><pre class="launch-command">' +
    esc(body.shell + " -c " + body.command) +
    "</pre></dd></dl>" +
    '<div class="launch-choices"><button type="button" class="launch-confirm" onclick="startArtifactLaunch(' +
    jsArg(index) +
    "," +
    jsArg(child) +
    "," +
    jsArg(name) +
    ',false)">Launch</button><button type="button" onclick="closeArtifactLaunch()">Cancel</button></div>';
}

async function startArtifactLaunch(index, child, name, newInstance) {
  const panel = artifactLaunchPanel();
  panel.innerHTML = '<p class="meta">Starting...</p>';
  let response;
  try {
    response = await fetch("/api/launch", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(launchRequestBody(index, child, name, newInstance)),
    });
  } catch (error) {
    panel.innerHTML = launchExplanation(
      "Cannot reach the server: " + error.message,
    );
    return;
  }
  const body = await response
    .json()
    .catch(() => ({ error: "HTTP " + response.status }));
  const show = (id) =>
    '<button type="button" onclick="showLaunches(' +
    jsArg(id) +
    ')">Show in Launches</button>';
  if (!response.ok) {
    panel.innerHTML =
      launchExplanation(body.error, body.hint) +
      (body.launch
        ? '<div class="launch-choices">' + show(body.launch.id) + "</div>"
        : "");
    return;
  }
  if (body.reused) {
    panel.innerHTML =
      '<p class="launch-reason">' +
      esc(
        name +
          " is already running here, since " +
          formatDateTime(body.started_at) +
          ".",
      ) +
      '</p><div class="launch-choices">' +
      launchOpenLink(body) +
      show(body.id) +
      '<button type="button" onclick="startArtifactLaunch(' +
      jsArg(index) +
      "," +
      jsArg(child) +
      "," +
      jsArg(name) +
      ',true)">Start another instance</button></div>';
    return;
  }
  if (body.mode === "service") return followServiceStart(body.id, name);
  panel.innerHTML =
    '<p class="launch-reason">' +
    esc("Started " + name + ".") +
    '</p><div class="launch-choices">' +
    show(body.id) +
    "</div>";
}

// launchOpenLink opens a ready service through the server's proxy, in a new
// tab. A service that is starting or has ended has no link.
function launchOpenLink(launch) {
  if (launch.mode !== "service" || launch.state !== "running") return "";
  return (
    '<a class="launch-open" href="' +
    esc(launch.base_path + "/") +
    '" target="_blank" rel="noopener">Open</a>'
  );
}

// followServiceStart shows a starting service until it is ready, with its
// Open link, or until it fails, with the server's reason.
async function followServiceStart(id, name) {
  const show =
    '<button type="button" onclick="showLaunches(' +
    jsArg(id) +
    ')">Show in Launches</button>';
  for (;;) {
    const panel = artifactLaunchPanel();
    if (!panel) return;
    let launch;
    try {
      const response = await fetch("/api/launches");
      launch = ((await response.json()).launches || []).find(
        (item) => item.id === id,
      );
    } catch (error) {
      panel.innerHTML = launchExplanation(
        "Cannot follow " + name + ": " + error.message,
      );
      return;
    }
    if (!launch) return;
    if (launch.state === "starting") {
      panel.innerHTML =
        '<p class="launch-reason">' +
        esc("Starting " + name + "; waiting for it to answer...") +
        '</p><div class="launch-choices">' +
        show +
        "</div>";
      await new Promise((resolve) => setTimeout(resolve, 500));
      continue;
    }
    panel.innerHTML =
      (launch.state === "running"
        ? '<p class="launch-reason">' + esc(name + " is ready.") + "</p>"
        : launchExplanation(name + " " + launchOutcome(launch))) +
      '<div class="launch-choices">' +
      launchOpenLink(launch) +
      show +
      "</div>";
    return;
  }
}

// showLaunches opens the server-wide list of launches, optionally with one
// launch's output.
async function showLaunches(launchID) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "launches";
  modal.querySelector("strong").textContent = "Launches";
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  selectedOutput = "";
  selectedLaunchID = launchID || "";
  openOutputModal(false);
  await renderLaunches();
  if (launchListTimer) clearInterval(launchListTimer);
  launchListTimer = setInterval(() => {
    if (document.getElementById("output-modal").dataset.view !== "launches") {
      clearInterval(launchListTimer);
      launchListTimer = null;
      return;
    }
    renderLaunches();
  }, 2000);
}

function launchOutcome(launch) {
  if (launch.state === "failed") return "failed: " + (launch.error || "");
  if (launch.signal) return launch.state + " (" + launch.signal + ")";
  if (launch.exit_code !== undefined && launch.exit_code !== null)
    return launch.state + " (exit " + launch.exit_code + ")";
  return launch.state;
}

async function renderLaunches() {
  const view = document.getElementById("launch-view");
  let launches;
  try {
    const response = await fetch("/api/launches");
    const body = await response.json();
    if (!response.ok) {
      view.innerHTML = launchExplanation(body.error, body.hint);
      return;
    }
    launches = (body.launches || []).slice().reverse();
  } catch (error) {
    view.innerHTML = launchExplanation(
      "Cannot list launches: " + error.message,
    );
    return;
  }
  const rows = launches
    .map((launch) => {
      const live =
        launch.state === "starting" ||
        launch.state === "running" ||
        launch.state === "stopping";
      return (
        '<tr data-launch="' +
        esc(launch.id) +
        '"' +
        (launch.id === selectedLaunchID ? ' class="selected"' : "") +
        "><td>" +
        esc(launch.name) +
        "</td><td>" +
        esc(launch.target) +
        "</td><td>" +
        esc(launchOutcome(launch)) +
        "</td><td>" +
        esc(formatDateTime(launch.started_at)) +
        "</td><td>" +
        launchOpenLink(launch) +
        '<button type="button" onclick="showLaunchLog(' +
        jsArg(launch.id) +
        ')">Logs</button>' +
        (live
          ? '<button type="button" onclick="stopLaunch(' +
            jsArg(launch.id) +
            ')"' +
            (launch.state === "stopping" ? " disabled" : "") +
            ">Stop</button>"
          : "") +
        "</td></tr>"
      );
    })
    .join("");
  let log = "";
  if (selectedLaunchID) {
    try {
      const response = await fetch(
        "/api/launch-log?" +
          new URLSearchParams({
            id: selectedLaunchID,
            from: "end",
            limit: String(1 << 20),
          }),
      );
      const page = await response.json();
      log = response.ok
        ? (page.offset > 0
            ? '<p class="meta">Earlier output is not shown; the most recent output follows.</p>'
            : "") +
          '<pre class="log launch-log">' +
          esc(page.text || "(no output)") +
          "</pre>"
        : launchExplanation(page.error, page.hint);
    } catch (error) {
      log = launchExplanation("Cannot read the output: " + error.message);
    }
  }
  view.innerHTML =
    (launches.length
      ? '<table class="runs launch-table"><thead><tr><th>Command</th><th>Target</th><th>State</th><th>Started</th><th></th></tr></thead><tbody>' +
        rows +
        "</tbody></table>"
      : '<p class="meta">Nothing has been launched from this server yet. Open a job\'s Artifacts and choose Launch with.</p>') +
    log;
}

function showLaunchLog(id) {
  selectedLaunchID = id;
  renderLaunches();
}

async function stopLaunch(id) {
  const response = await fetch("/api/launch-stop", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ id }),
  });
  if (!response.ok) {
    const body = await response
      .json()
      .catch(() => ({ error: "HTTP " + response.status }));
    document
      .getElementById("launch-view")
      .insertAdjacentHTML(
        "afterbegin",
        launchExplanation(body.error, body.hint),
      );
    return;
  }
  renderLaunches();
}

// addLauncherConfigButtons adds the server-wide launcher config controls to
// Settings on every page; launcher config is not part of a run snapshot.
function addLauncherConfigButtons(toolbar) {
  if (typeof rewriteStaticLinks === "function") return;
  const view = document.createElement("button");
  view.className = "launcher-config-button";
  view.textContent = "Launcher config";
  view.title = "View launchers.toml and the commands it registers";
  view.onclick = () => showLauncherConfig();
  const generate = document.createElement("button");
  generate.className = "launcher-generate-config-button";
  generate.textContent = "Generate launcher config";
  generate.title = "Write a launchers.toml template if there is none";
  generate.onclick = () => generateLauncherConfig();
  toolbar.append(view, generate);
}

// showLauncherConfig shows the launcher file as it is on disk, why rotari
// web --allow-exec would refuse it, and the commands it registers.
async function showLauncherConfig(message) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "launcher-config";
  modal.querySelector("strong").textContent = "Launcher config";
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  selectedOutput = "";
  openOutputModal(false);
  const view = document.getElementById("launch-view");
  view.innerHTML = '<p class="meta">Loading...</p>';
  let config;
  try {
    const response = await fetch("/api/launcher-config", { cache: "no-store" });
    if (!response.ok) throw new Error(await response.text());
    config = await response.json();
  } catch (error) {
    view.innerHTML = launchExplanation(
      "Cannot read the launcher config: " + error.message,
    );
    return;
  }
  const commands = (config.commands || [])
    .map(
      (command) =>
        "<tr><td>" +
        esc(command.name) +
        "</td><td>" +
        esc((command.targets || []).join(", ")) +
        "</td><td>" +
        esc(command.mode) +
        "</td><td>" +
        esc(
          [
            command.builtin ? "built in" : "",
            command.unavailable ? "unavailable: " + command.unavailable : "",
          ]
            .filter(Boolean)
            .join("; "),
        ) +
        '</td><td><pre class="launch-command">' +
        esc(command.command) +
        "</pre></td></tr>",
    )
    .join("");
  view.innerHTML =
    (message ? '<p class="launch-reason">' + esc(message) + "</p>" : "") +
    '<p class="meta">' +
    esc(config.path) +
    (config.exists ? "" : " (does not exist)") +
    "</p>" +
    (config.error ? launchExplanation(config.error) : "") +
    '<p class="meta">' +
    esc(
      config.enabled
        ? "This server launches commands; it read this file at startup, so restart it to apply edits."
        : "This server does not launch commands; start rotari web --allow-exec to use them.",
    ) +
    "</p>" +
    (commands
      ? '<table class="runs launch-table"><thead><tr><th>Name</th><th>Targets</th><th>Mode</th><th>Notes</th><th>Command</th></tr></thead><tbody>' +
        commands +
        "</tbody></table>"
      : "") +
    (config.exists
      ? config.editable
        ? '<div class="launcher-config-editor"></div>'
        : '<pre class="log launcher-config-text">' +
          esc(config.content) +
          "</pre>"
      : '<div class="launch-choices"><button type="button" onclick="generateLauncherConfig()">Generate launcher config</button></div>');
  if (config.exists && config.editable) {
    const editor = document.createElement("div");
    editor.className = "launcher-config-editor";
    const textarea = document.createElement("textarea");
    textarea.setAttribute("aria-label", "Launcher config contents");
    textarea.value = config.content || "";
    textarea.dataset.initial = textarea.value;
    const save = document.createElement("button");
    save.type = "button";
    save.textContent = "Save launcher config";
    save.disabled = true;
    textarea.oninput = () => {
      save.disabled = textarea.value === textarea.dataset.initial;
      textarea.classList.toggle("dirty", !save.disabled);
    };
    save.onclick = async () => {
      save.disabled = true;
      try {
        const response = await fetch("/api/save-launcher-config", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: JSON.stringify({
            expected: textarea.dataset.initial,
            content: textarea.value,
          }),
        });
        const text = await response.text();
        if (!response.ok)
          throw new Error(text.trim() || "HTTP " + response.status);
        const result = JSON.parse(text);
        await showLauncherConfig(result.message);
      } catch (error) {
        save.disabled = textarea.value === textarea.dataset.initial;
        view.insertAdjacentHTML(
          "afterbegin",
          launchExplanation(
            "Cannot save the launcher config: " + error.message,
          ),
        );
      }
    };
    editor.append(textarea, save);
    view.querySelector(".launcher-config-editor").replaceWith(editor);
  }
}

// generateLauncherConfig writes the template where the server reads the
// launcher file, never replacing one, and shows the result.
async function generateLauncherConfig() {
  const response = await fetch("/api/generate-launcher-config", {
    method: "POST",
  });
  const text = await response.text();
  let message = text.trim();
  if (response.ok) {
    try {
      message = JSON.parse(text).message;
    } catch (_) {}
  }
  await showLauncherConfig(message);
}
