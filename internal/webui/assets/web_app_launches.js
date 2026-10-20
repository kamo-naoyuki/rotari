// Launching the commands registered in launchers.toml against an artifact,
// and the server-wide list of launches. The server decides what may run,
// resolves every path, and words every explanation; this file only lays
// them out. Nothing runs without the Launch button in the confirmation.
let launcherAvailability = null;
// selectedLaunchID is the launch whose output the Launches view shows.
let selectedLaunchID = "";
let launchListTimer = null;

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

function artifactLaunchButton(index, child, type) {
  return (
    '<button type="button" class="artifact-launch-button" onclick="toggleArtifactLaunch(' +
    index +
    "," +
    jsArg(child) +
    "," +
    jsArg(type) +
    ')">Launch with…</button>'
  );
}

function artifactLaunchPanel() {
  return document.getElementById("artifact-launch-panel");
}

function closeArtifactLaunch() {
  artifactLaunchPanel()?.remove();
}

// toggleArtifactLaunch lists the registered commands that accept the open
// artifact, or explains why there are none.
async function toggleArtifactLaunch(index, child, type) {
  if (artifactLaunchPanel()) return closeArtifactLaunch();
  const panel = document.createElement("div");
  panel.id = "artifact-launch-panel";
  panel.className = "artifact-launch-panel";
  panel.innerHTML = '<p class="meta">Loading...</p>';
  artifactPreview().querySelector(".artifact-preview-title").after(panel);
  const availability = await loadLauncherAvailability();
  if (!availability.available) {
    panel.innerHTML = launchExplanation(availability.reason, availability.hint);
    return;
  }
  const kind = type === "directory" ? "directory" : "file";
  const commands = (availability.commands || []).filter((command) =>
    (command.targets || []).includes(kind),
  );
  if (!commands.length) {
    panel.innerHTML = launchExplanation((availability.unmatched || {})[kind]);
    return;
  }
  panel.innerHTML =
    '<p class="meta">Launch with</p><div class="launch-choices">' +
    commands
      .map(
        (command) =>
          '<button type="button" onclick="previewArtifactLaunch(' +
          index +
          "," +
          jsArg(child) +
          "," +
          jsArg(command.name) +
          ')">' +
          esc(command.name) +
          "</button>",
      )
      .join("") +
    "</div>";
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
    index +
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
      show(body.id) +
      '<button type="button" onclick="startArtifactLaunch(' +
      index +
      "," +
      jsArg(child) +
      "," +
      jsArg(name) +
      ',true)">Start another instance</button></div>';
    return;
  }
  panel.innerHTML =
    '<p class="launch-reason">' +
    esc("Started " + name + ".") +
    '</p><div class="launch-choices">' +
    show(body.id) +
    "</div>";
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
      const live = launch.state === "running" || launch.state === "stopping";
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
        '</td><td><button type="button" onclick="showLaunchLog(' +
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
          new URLSearchParams({ id: selectedLaunchID, limit: String(1 << 20) }),
      );
      const page = await response.json();
      log = response.ok
        ? (page.dropped
            ? '<p class="meta">Earlier output was dropped; the most recent output follows.</p>'
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
