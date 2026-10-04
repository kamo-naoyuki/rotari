// Artifact candidates of a job attempt, and previews of their contents on
// the live server. The server decides which entries may be previewed and
// computes every field; this file only lays them out.
let selectedArtifacts = null;
const artifactImageTypes = ["png", "jpg", "jpeg", "gif", "svg"];
const artifactAudioTypes = ["wav", "mp3", "flac", "ogg", "oga", "opus", "m4a"];
const artifactVideoTypes = ["mp4", "webm", "mov"];
// Text shown from the end, like a job log; other text starts at the top.
const artifactTailTypes = ["log", "txt"];
const artifactTextTypes = [
  "json",
  "jsonl",
  "yaml",
  "yml",
  "toml",
  "py",
  "sh",
  "md",
  "cfg",
  "ini",
  "conf",
  "out",
  "err",
];
async function showArtifacts(queue, run, job, attemptID) {
  const modal = document.getElementById("output-modal");
  modal.dataset.view = "artifacts";
  modal.querySelector("strong").textContent = "Artifacts";
  if (followTimer) clearInterval(followTimer);
  followTimer = null;
  selectedLog = null;
  selectedArtifacts = { queue, run, job, attemptID, listing: null };
  selectedOutput = "Loading...";
  artifactView().textContent = selectedOutput;
  openOutputModal(false);
  try {
    const response = await fetch(
      "/api/artifacts?" + artifactParams(selectedArtifacts),
    );
    if (!response.ok) throw new Error(await response.text());
    selectedArtifacts.listing = await response.json();
    selectedOutput = formatArtifactListing(selectedArtifacts.listing);
    if ((selectedArtifacts.listing.entries || []).length) {
      renderArtifactListing();
    } else {
      showArtifactText();
    }
  } catch (error) {
    selectedOutput = "Failed to load artifacts: " + error.message;
    showArtifactText();
  }
}
// showArtifactText shows a listing without entries, or a failure, as plain
// text in the log box, where the copy button sits as it does for logs.
function showArtifactText() {
  document.getElementById("output-modal").dataset.view = "artifacts-text";
  ensureModalOutput().textContent = selectedOutput;
  openOutputModal(isCompactOutput(selectedOutput));
}
function artifactView() {
  return document.getElementById("artifact-view");
}
function artifactParams(context, extra) {
  const params = new URLSearchParams({
    project_name: context.queue,
    run_id: context.run,
    job_id: context.job,
  });
  if (context.attemptID) params.set("attempt_id", context.attemptID);
  Object.entries(extra || {}).forEach(([key, value]) => {
    if (value !== undefined && value !== "") params.set(key, value);
  });
  return params;
}
// formatArtifactListing lays out a listing from /api/artifacts line for
// line as `rotari show -j JOB --artifacts` prints it; the copy button copies
// this text.
function formatArtifactListing(listing) {
  if (!listing.recorded) return "Artifacts: (not recorded)";
  const entries = listing.entries || [];
  const lines = [];
  if (!entries.length) {
    lines.push("Artifacts: none found");
  } else {
    lines.push(
      listing.working_directory
        ? "Artifacts: relative to " + listing.working_directory
        : "Artifacts:",
    );
    entries.forEach((entry) =>
      lines.push(
        "  " +
          entry.type.padEnd(9) +
          "  " +
          entry.display_path +
          "  (" +
          entry.origin +
          ")",
      ),
    );
  }
  const notes = listing.diagnostics || [];
  if (notes.length) {
    lines.push("Discovery notes:");
    notes.forEach((note) =>
      lines.push("  " + (note.source ? note.source + ": " : "") + note.message),
    );
  }
  return lines.join("\n") + "\n";
}
function renderArtifactListing() {
  const listing = selectedArtifacts.listing;
  const entries = listing.entries;
  const previewable = listing.previewable || [];
  const rows = entries
    .map((entry, index) => {
      const path = previewable[index]
        ? '<button type="button" class="artifact-open" onclick="openArtifact(' +
          index +
          "," +
          jsArg("") +
          "," +
          jsArg(entry.type) +
          ')">' +
          esc(entry.display_path) +
          "</button>"
        : esc(entry.display_path);
      return (
        "<tr><td>" +
        esc(entry.type) +
        "</td><td>" +
        path +
        "</td><td>" +
        esc(entry.origin) +
        "</td></tr>"
      );
    })
    .join("");
  const notes = (listing.diagnostics || [])
    .map(
      (note) =>
        "<li>" +
        esc((note.source ? note.source + ": " : "") + note.message) +
        "</li>",
    )
    .join("");
  const hint = listing.previewable
    ? ""
    : '<p class="meta">Previews need the live Web UI, or a static export made with --static-artifact-contents.</p>';
  artifactView().innerHTML =
    (listing.working_directory
      ? '<p class="meta">Relative to ' + esc(listing.working_directory) + "</p>"
      : "") +
    hint +
    '<table class="runs artifact-table"><thead><tr><th>Type</th><th>Path</th><th>Found in</th></tr></thead><tbody>' +
    rows +
    "</tbody></table>" +
    (notes
      ? '<p class="meta">Discovery notes</p><ul class="artifact-notes">' +
        notes +
        "</ul>"
      : "") +
    '<div id="artifact-preview" class="artifact-preview"></div>';
}
function artifactPreview() {
  return document.getElementById("artifact-preview");
}
// jsArg writes a value as a JavaScript literal inside an HTML attribute,
// such as an onclick handler: JSON.stringify quotes it for JavaScript, and
// esc for HTML, which the browser undoes before running the handler.
function jsArg(value) {
  return esc(JSON.stringify(value));
}
// artifactPath is a content route with its query, for fetch, which adds
// the basedir prefix itself; artifactURL adds it for img and a elements.
function artifactPath(path, index, child, extra) {
  return (
    path +
    "?" +
    artifactParams(
      selectedArtifacts,
      Object.assign({ entry: String(index), child }, extra || {}),
    )
  );
}
function artifactURL(path, index, child, extra) {
  // A static export made with --static-artifact-contents serves its copy.
  if (typeof staticArtifactFileURL === "function") {
    const copy = staticArtifactFileURL(
      artifactParams(selectedArtifacts, { entry: String(index), child }),
    );
    if (copy) return copy;
  }
  return appURL(artifactPath(path, index, child, extra));
}
function artifactExtension(index, child) {
  const name = child || selectedArtifacts.listing.entries[index].path;
  const dot = name.lastIndexOf(".");
  return dot > name.lastIndexOf("/") ? name.slice(dot + 1).toLowerCase() : "";
}
function artifactTitle(index, child) {
  const entry = selectedArtifacts.listing.entries[index];
  return entry.display_path + (child ? "/" + child : "");
}
function artifactDownloadLink(index, child) {
  return (
    '<a class="artifact-download" href="' +
    esc(artifactURL("/api/artifact-file", index, child, { download: "1" })) +
    '" download>Download</a>'
  );
}
// openArtifact previews a listed entry, or a child of a listed directory:
// a directory's children, an image, text a page at a time, a table, or
// otherwise the file's size and time with a download.
async function openArtifact(index, child, type) {
  const extension = artifactExtension(index, child);
  const header =
    '<p class="artifact-preview-title"><strong>' +
    esc(artifactTitle(index, child)) +
    "</strong> " +
    (type === "directory" ? "" : artifactDownloadLink(index, child)) +
    "</p>";
  const preview = artifactPreview();
  preview.innerHTML = header + '<p class="meta">Loading...</p>';
  preview.dataset.index = String(index);
  preview.dataset.child = child;
  if (type === "directory") return loadArtifactDirectory(index, child, 0);
  if (artifactImageTypes.includes(extension)) {
    preview.innerHTML =
      header +
      '<img class="artifact-image" alt="' +
      esc(artifactTitle(index, child)) +
      '" src="' +
      esc(artifactURL("/api/artifact-file", index, child)) +
      '">';
    return;
  }
  if (
    artifactAudioTypes.includes(extension) ||
    artifactVideoTypes.includes(extension)
  ) {
    const tag = artifactAudioTypes.includes(extension) ? "audio" : "video";
    preview.innerHTML =
      header +
      "<" +
      tag +
      ' class="artifact-media" controls preload="metadata" src="' +
      esc(artifactURL("/api/artifact-file", index, child)) +
      '"></' +
      tag +
      ">";
    return;
  }
  if (extension === "npy" || extension === "npz") {
    return loadArtifactArrays(index, child, header);
  }
  if (extension === "csv" || extension === "tsv") {
    return loadArtifactTable(index, child, extension === "tsv" ? "\t" : ",");
  }
  if (artifactTailTypes.includes(extension)) {
    return loadArtifactText(index, child, "end");
  }
  if (artifactTextTypes.includes(extension)) {
    return loadArtifactText(index, child, "start");
  }
  const response = await fetch(
    artifactPath("/api/artifact-file", index, child),
    {
      method: "HEAD",
    },
  );
  preview.innerHTML =
    header +
    (response.ok
      ? '<p class="meta">No preview for this file type. ' +
        esc(formatBytes(Number(response.headers.get("Content-Length") || 0))) +
        ", modified " +
        esc(response.headers.get("Last-Modified") || "-") +
        ".</p>"
      : '<p class="meta">' +
        esc(
          "Failed to open: " +
            (response.statusText || "HTTP " + response.status),
        ) +
        "</p>");
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
async function fetchArtifactJSON(path, index, child, extra) {
  const response = await fetch(artifactPath(path, index, child, extra));
  const body = await response.text();
  if (!response.ok) throw new Error(body.trim() || "HTTP " + response.status);
  return JSON.parse(body);
}
function artifactFailure(index, child, error) {
  artifactPreview().innerHTML =
    '<p class="artifact-preview-title"><strong>' +
    esc(artifactTitle(index, child)) +
    '</strong></p><p class="meta">' +
    esc("Failed to open: " + error.message) +
    "</p>";
}
// loadArtifactText shows a text file a page at a time: from the end with
// earlier pages on demand, or from the start with later pages on demand.
async function loadArtifactText(index, child, from) {
  const state = { index, child, from, text: "", start: 0, end: 0, size: 0 };
  selectedArtifacts.text = state;
  await moreArtifactText();
}
async function moreArtifactText() {
  const state = selectedArtifacts.text;
  const offset =
    state.text === ""
      ? ""
      : String(state.from === "end" ? state.start : state.end);
  try {
    const page = await fetchArtifactJSON(
      "/api/artifact-text",
      state.index,
      state.child,
      { from: state.from, offset },
    );
    if (state.text === "") {
      state.start = page.start;
      state.end = page.end;
      state.text = page.text;
    } else if (state.from === "end") {
      state.start = page.start;
      state.text = page.text + state.text;
    } else {
      state.end = page.end;
      state.text += page.text;
    }
    state.size = page.size;
  } catch (error) {
    return artifactFailure(state.index, state.child, error);
  }
  const more =
    state.from === "end"
      ? state.start > 0
        ? '<button type="button" onclick="moreArtifactText()">Load earlier</button>'
        : ""
      : state.end < state.size
        ? '<button type="button" onclick="moreArtifactText()">Load more</button>'
        : "";
  const pre = '<pre class="log artifact-text">' + esc(state.text) + "</pre>";
  artifactPreview().innerHTML =
    '<p class="artifact-preview-title"><strong>' +
    esc(artifactTitle(state.index, state.child)) +
    "</strong> " +
    artifactDownloadLink(state.index, state.child) +
    "</p>" +
    (state.from === "end" ? more + pre : pre + more);
}
// loadArtifactTable shows a CSV or TSV file as a table of the rows loaded so
// far, loading more on demand.
async function loadArtifactTable(index, child, delimiter) {
  selectedArtifacts.text = {
    index,
    child,
    from: "start",
    text: "",
    start: 0,
    end: 0,
    size: 0,
    delimiter,
  };
  await moreArtifactTable();
}
async function moreArtifactTable() {
  const state = selectedArtifacts.text;
  try {
    const page = await fetchArtifactJSON(
      "/api/artifact-text",
      state.index,
      state.child,
      { from: "start", offset: state.text === "" ? "" : String(state.end) },
    );
    state.text += page.text;
    state.end = page.end;
    state.size = page.size;
  } catch (error) {
    return artifactFailure(state.index, state.child, error);
  }
  const rows = parseDelimited(state.text, state.delimiter);
  const [head, ...body] = rows;
  const cells = (row, tag) =>
    row.map((cell) => "<" + tag + ">" + esc(cell) + "</" + tag + ">").join("");
  artifactPreview().innerHTML =
    '<p class="artifact-preview-title"><strong>' +
    esc(artifactTitle(state.index, state.child)) +
    "</strong> " +
    artifactDownloadLink(state.index, state.child) +
    "</p>" +
    '<div class="artifact-table-scroll"><table class="runs"><thead><tr>' +
    cells(head || [], "th") +
    "</tr></thead><tbody>" +
    body.map((row) => "<tr>" + cells(row, "td") + "</tr>").join("") +
    "</tbody></table></div>" +
    (state.end < state.size
      ? '<button type="button" onclick="moreArtifactTable()">Load more rows</button>'
      : "");
}
// parseDelimited splits CSV or TSV text into rows of cells, honoring double
// quotes.
function parseDelimited(text, delimiter) {
  const rows = [];
  let row = [];
  let cell = "";
  let quoted = false;
  for (let index = 0; index < text.length; index++) {
    const character = text[index];
    if (quoted) {
      if (character === '"' && text[index + 1] === '"') {
        cell += '"';
        index++;
      } else if (character === '"') {
        quoted = false;
      } else {
        cell += character;
      }
    } else if (character === '"' && cell === "") {
      quoted = true;
    } else if (character === delimiter) {
      row.push(cell);
      cell = "";
    } else if (character === "\n") {
      row.push(cell.replace(/\r$/, ""));
      rows.push(row);
      row = [];
      cell = "";
    } else {
      cell += character;
    }
  }
  if (cell !== "" || row.length) {
    row.push(cell);
    rows.push(row);
  }
  return rows;
}
// loadArtifactDirectory lists a page of a directory's immediate children;
// each opens under the same rules, and later pages load on demand.
async function loadArtifactDirectory(index, child, offset) {
  let page;
  try {
    page = await fetchArtifactJSON("/api/artifact-directory", index, child, {
      offset: String(offset),
    });
  } catch (error) {
    return artifactFailure(index, child, error);
  }
  const rows = page.entries
    .map((entry) => {
      const path = child ? child + "/" + entry.name : entry.name;
      const type = entry.type === "symlink" ? "file" : entry.type;
      const open =
        entry.type === "other"
          ? esc(entry.name)
          : '<button type="button" class="artifact-open" onclick="openArtifact(' +
            index +
            "," +
            jsArg(path) +
            "," +
            jsArg(type) +
            ')">' +
            esc(entry.name) +
            "</button>";
      return (
        "<tr><td>" +
        esc(entry.type) +
        "</td><td>" +
        open +
        "</td><td>" +
        esc(entry.type === "file" ? formatBytes(entry.size || 0) : "") +
        "</td><td>" +
        esc(entry.modified || "") +
        "</td></tr>"
      );
    })
    .join("");
  const end = page.offset + page.entries.length;
  artifactPreview().innerHTML =
    '<p class="artifact-preview-title"><strong>' +
    esc(artifactTitle(index, child)) +
    "</strong></p>" +
    '<p class="meta">' +
    esc(
      (page.total
        ? page.offset + 1 + "–" + end + " of " + page.total
        : "Empty") +
        (page.truncated
          ? "; only the first " + page.total + " entries are listed"
          : ""),
    ) +
    "</p>" +
    '<table class="runs"><thead><tr><th>Type</th><th>Name</th><th>Size</th><th>Modified</th></tr></thead><tbody>' +
    rows +
    "</tbody></table>" +
    (end < page.total
      ? '<button type="button" onclick="loadArtifactDirectory(' +
        index +
        "," +
        jsArg(child) +
        "," +
        end +
        ')">More</button>'
      : "");
}
// loadArtifactArrays describes a .npy file, or each array of a .npz file:
// dtype, shape, and its first values, which the server reads from the
// file's headers without numpy.
async function loadArtifactArrays(index, child, header) {
  let result;
  try {
    result = await fetchArtifactJSON("/api/artifact-array", index, child);
  } catch (error) {
    return artifactFailure(index, child, error);
  }
  const arrays = result.arrays
    .map(
      (array) =>
        '<div class="artifact-array"><p>' +
        (array.name ? "<strong>" + esc(array.name) + "</strong> " : "") +
        esc(
          (array.dtype || "?") +
            " shape (" +
            (array.shape || []).join(", ") +
            (array.shape && array.shape.length === 1 ? "," : "") +
            ")" +
            (array.fortran_order ? " Fortran order" : "") +
            (array.dtype ? ", " + array.size + " values" : ""),
        ) +
        "</p>" +
        (array.values && array.values.length
          ? '<pre class="log artifact-values">[' +
            esc(array.values.join(", ")) +
            (array.values.length < array.size ? ", …" : "") +
            "]</pre>"
          : "") +
        (array.note ? '<p class="meta">' + esc(array.note) + "</p>" : "") +
        "</div>",
    )
    .join("");
  artifactPreview().innerHTML =
    header +
    (arrays || '<p class="meta">No arrays.</p>') +
    (result.truncated
      ? '<p class="meta">Only the first arrays are listed.</p>'
      : "");
}
