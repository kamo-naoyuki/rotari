// @ts-check
function runLoadSummary(run) {
  const context = run && run.context;
  const samples = (context && context.load_samples) || [];
  const load =
    samples.length > 0
      ? samples[samples.length - 1]
      : context && context.started_load;
  if (!load) return "";
  const format = (value) => Number(value).toFixed(2);
  return (
    "load " +
    format(load.one) +
    " / " +
    format(load.five) +
    " / " +
    format(load.fifteen)
  );
}

// The run page's sections: statistics, job timeline, load average, and
// output word cloud. renderRunGraphics draws them whole, above the jobs
// table. The job timeline starts open and the others closed; a section the
// viewer opens or closes stays so in this browser (runSectionOpen).
/**
 * @param {string} kind the section's class, which also keys its open state
 * @param {string} title
 * @param {HTMLElement} [note] shown at the heading's end
 */
function runGraphicSection(kind, title, note) {
  const section = document.createElement("section");
  section.className = "run-graphic " + kind;
  const heading = document.createElement("div");
  heading.className = "graphic-heading";
  const toggle = document.createElement("button");
  toggle.type = "button";
  toggle.className = "graphic-toggle";
  const h2 = document.createElement("h2");
  h2.textContent = title;
  heading.append(toggle, h2);
  if (note) heading.append(note);
  section.append(heading);
  /** @param {boolean} expanded */
  const apply = (expanded) => {
    section.classList.toggle("collapsed", !expanded);
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.textContent = expanded ? "-" : "+";
  };
  toggle.onclick = () => {
    const expanded = !runSectionOpen(kind);
    setRunSectionOpen(kind, expanded);
    apply(expanded);
  };
  apply(runSectionOpen(kind));
  return section;
}
const runSectionsKey = "rotari-run-sections";
/** @returns {Record<string, boolean>} */
function storedRunSections() {
  try {
    const stored = JSON.parse(localStorage.getItem(runSectionsKey) || "{}");
    return stored && typeof stored === "object" ? stored : {};
  } catch {
    return {};
  }
}
/** @param {string} kind */
function runSectionOpen(kind) {
  if (kind in expandedRunGraphics) return expandedRunGraphics[kind];
  const stored = storedRunSections()[kind];
  return typeof stored === "boolean" ? stored : kind === "job-timeline";
}
/**
 * @param {string} kind
 * @param {boolean} open
 */
function setRunSectionOpen(kind, open) {
  expandedRunGraphics[kind] = open;
  const stored = storedRunSections();
  stored[kind] = open;
  try {
    localStorage.setItem(runSectionsKey, JSON.stringify(stored));
  } catch {
    // The choice then lasts while the page is open.
  }
}
/**
 * @param {string} text
 * @param {string} [className]
 */
function graphicNote(text, className) {
  const note = document.createElement("span");
  note.className = "graphic-note" + (className ? " " + className : "");
  note.textContent = text;
  return note;
}
// The job timeline draws one lane per job, in the jobs' order, against the
// run's time: each executed job's span (run.lanes, from the server) coloured
// by its status, a running job's span up to now, an open-ended span for a
// job that recorded no finish, and a note for a job not yet started or
// carried from an earlier run. The axis starts at the run timeline's initial
// point and marks the rest as offsets from it. A lane's label shows the
// job's row in the table.
const timelineLaneLimit = 50;
/**
 * @param {number} origin the axis start, milliseconds since the epoch
 * @param {number} span the axis length in milliseconds
 * @param {number[]} ticks fractions of the span
 * @returns {string[]} the start's clock time, with its date when the span
 *   crosses a day, then each tick as an offset from the start
 */
function timelineAxisLabels(origin, span, ticks) {
  const start = new Date(origin);
  const crossesDay =
    start.toDateString() !== new Date(origin + span).toDateString();
  return ticks.map((tick) => {
    if (tick === 0)
      return (
        "start · " +
        (crossesDay
          ? start.toLocaleString(undefined, {
              month: "short",
              day: "numeric",
              hour: "2-digit",
              minute: "2-digit",
              second: "2-digit",
            })
          : start.toLocaleTimeString())
      );
    const offset = tick * span;
    return (
      "+" +
      (offset < 10000 ? (offset / 1000).toFixed(1) + "s" : formatSpan(offset))
    );
  });
}
/** @param {WebRun} run */
function jobTimelineSection(run) {
  const jobs = run.jobs || [];
  const lanes = run.lanes || [];
  /** @param {string | undefined} value */
  const time = (value) => (value ? Date.parse(value) : NaN);
  const finite = (/** @type {number[]} */ values) =>
    values.filter((value) => Number.isFinite(value));
  const starts = finite(lanes.map((lane) => time(lane.start)));
  const ends = finite(lanes.map((lane) => time(lane.end)));
  const points = run.timeline || [];
  const first = time(points[0]?.at);
  const origin = Number.isFinite(first)
    ? first
    : starts.length
      ? Math.min(...starts)
      : Date.now();
  let end = Math.max(
    origin,
    ...starts,
    ...ends,
    ...finite([time(points[points.length - 1]?.at)]),
  );
  if (run.running) end = Math.max(end, Date.now());
  const span = Math.max(end - origin, 1000);
  /** @param {number} value */
  const percent = (value) =>
    Math.min(100, Math.max(0, ((value - origin) / span) * 100));
  const byID = new Map(jobs.map((job) => [job.id, job]));
  const shown = lanes.slice(0, timelineLaneLimit);
  const tones = new Set();
  const rows = shown
    .map((lane) => {
      const job = byID.get(lane.job_id) || { id: lane.job_id };
      const name = job.name || job.id;
      const status = jobDisplayStatus(job, run);
      const tone = statusTone(status);
      const start = time(lane.start);
      let track;
      if (lane.carried) {
        track = '<span class="lane-note">carried from an earlier run</span>';
      } else if (!Number.isFinite(start)) {
        track = '<span class="lane-note">' + esc(status) + "</span>";
      } else {
        tones.add(tone);
        const finish = time(lane.end);
        const open = !Number.isFinite(finish);
        const left = percent(start);
        const right = open
          ? run.running
            ? percent(Date.now())
            : 100
          : percent(finish);
        track =
          '<span class="lane-bar tone-' +
          tone +
          (open && !run.running ? " open" : "") +
          '" style="left:' +
          left.toFixed(2) +
          "%;width:" +
          Math.max(right - left, 0.6).toFixed(2) +
          '%" title="' +
          esc(
            name +
              " · " +
              status +
              (job.elapsed ? " · " + job.elapsed : "") +
              (open && !run.running ? " · no finish recorded" : ""),
          ) +
          '"></span>';
      }
      return (
        '<div class="lane-row"><button type="button" class="lane-label" title="Show ' +
        esc(name) +
        ' in the table" onclick="focusMatrixJob(' +
        esc(JSON.stringify(lane.job_id)) +
        ')">' +
        esc(name) +
        '</button><div class="lane-track">' +
        track +
        "</div></div>"
      );
    })
    .join("");
  const ticks = [0, 0.25, 0.5, 0.75, 1];
  const labels = timelineAxisLabels(origin, span, ticks);
  const axis =
    '<div class="lane-row lane-axis-row"><span></span><div class="lane-axis">' +
    ticks
      .map(
        (tick, index) =>
          '<span style="left:' +
          tick * 100 +
          '%">' +
          esc(labels[index]) +
          "</span>",
      )
      .join("") +
    "</div></div>";
  const toneLabels = {
    ok: "success",
    bad: "failed",
    run: "running",
    warn: "blocked / interrupted",
    off: "other",
  };
  const legend =
    '<div class="lane-legend">' +
    ["ok", "bad", "run", "warn", "off"]
      .filter((tone) => tones.has(tone))
      .map(
        (tone) =>
          '<span><i class="lane-swatch tone-' +
          tone +
          '"></i>' +
          toneLabels[tone] +
          "</span>",
      )
      .join("") +
    "</div>";
  const section = runGraphicSection(
    "job-timeline",
    "Job timeline",
    graphicNote(
      (lanes.length > timelineLaneLimit
        ? "first " + timelineLaneLimit + " of " + lanes.length + " jobs"
        : lanes.length + (lanes.length === 1 ? " job" : " jobs")) +
        " · " +
        formatSpan(span),
      "meta",
    ),
  );
  const chart = document.createElement("div");
  chart.className = "lane-chart";
  chart.innerHTML = lanes.length
    ? rows + axis + (tones.size > 1 ? legend : "")
    : '<div class="meta">No jobs.</div>';
  section.append(chart);
  return section;
}
/**
 * @param {number} milliseconds
 * @returns {string} a duration as rotari prints one ("5s", "3m 05s", "2h 07m")
 */
function formatSpan(milliseconds) {
  const seconds = Math.round(milliseconds / 1000);
  if (seconds < 60) return seconds + "s";
  const minutes = Math.floor(seconds / 60);
  if (minutes < 60)
    return minutes + "m " + String(seconds % 60).padStart(2, "0") + "s";
  return (
    Math.floor(minutes / 60) +
    "h " +
    String(minutes % 60).padStart(2, "0") +
    "m"
  );
}
/** @param {WebRun} run */
function runStatisticsSection(run) {
  const keys = ["success", "failed", "blocked", "running", "pending"];
  const counts = Object.fromEntries(keys.map((key) => [key, 0]));
  (run.jobs || []).forEach((job) => {
    const result = job.result;
    const status = !result
      ? run.running
        ? "running"
        : "pending"
      : result.error === "blocked by failed dependency"
        ? "blocked"
        : result.exit_code === 0
          ? "success"
          : "failed";
    counts[status]++;
  });
  const total = (run.jobs || []).length;
  const completed = counts.success + counts.failed;
  const successRate = completed
    ? Math.round((counts.success / completed) * 100)
    : 0;
  const tints = statusTintMap(keys);
  const section = runGraphicSection(
    "run-statistics",
    "Run statistics",
    graphicNote(total + " jobs"),
  );
  const metrics = document.createElement("div");
  metrics.className = "stat-metrics";
  [
    ["Success rate", successRate + "%", ""],
    ["Succeeded", counts.success, "succeeded"],
    ["Failed", counts.failed, "failed"],
    ["In progress", counts.running, "in progress"],
    ["Pending", counts.pending, "pending"],
  ].forEach(([label, value, status]) => {
    const metric = document.createElement("div");
    metric.className = "stat-metric";
    const valueElement = document.createElement("strong");
    valueElement.textContent = String(value);
    if (status) valueElement.style.color = statusColor(status);
    const labelElement = document.createElement("span");
    labelElement.textContent = String(label);
    metric.append(valueElement, labelElement);
    metrics.append(metric);
  });
  // The bar and legend share the counts; each part of the bar is a status's
  // share of the run's jobs.
  const bar = document.createElement("div");
  bar.className = "status-bar";
  bar.title = "Job status distribution";
  const legend = document.createElement("div");
  legend.className = "graphic-legend";
  keys.forEach((status) => {
    if (!counts[status]) return;
    const segment = document.createElement("span");
    segment.style.width = (counts[status] / Math.max(total, 1)) * 100 + "%";
    segment.style.background = tints[status][0];
    segment.title = status + ": " + counts[status];
    bar.append(segment);
    const item = document.createElement("span");
    item.textContent = status + " " + counts[status];
    item.style.borderLeftColor = statusColor(status);
    legend.append(item);
  });
  section.append(metrics, bar, legend);
  return section;
}
/** @param {WebRun} run */
function loadAverageSection(run) {
  const section = runGraphicSection(
    "run-environment",
    "Load average",
    graphicNote(runLoadSummary(run), "meta"),
  );
  const samples = ((run.context && run.context.load_samples) || []).filter(
    (sample) => sample.at,
  );
  if (samples.length < 2) return section;
  const width = 640,
    height = 230,
    left = 42,
    top = 16,
    right = 16,
    bottom = 42,
    plotWidth = width - left - right,
    plotHeight = height - top - bottom;
  const times = samples.map((sample) => Date.parse(sample.at));
  const start = Math.min(...times),
    end = Math.max(...times),
    span = Math.max(1, end - start);
  const maximum = Math.max(
    1,
    ...samples.flatMap((sample) => [
      sample.one || 0,
      sample.five || 0,
      sample.fifteen || 0,
    ]),
  );
  const x = (index) => left + ((times[index] - start) / span) * plotWidth;
  const y = (value) => top + plotHeight - (value / maximum) * plotHeight;
  const colors = { one: "var(--c1)", five: "var(--c2)", fifteen: "var(--c3)" };
  const labels = { one: "1 min", five: "5 min", fifteen: "15 min" };
  const wrap = document.createElement("div");
  wrap.className = "load-timeline";
  const legend = document.createElement("div");
  legend.className = "load-legend";
  Object.keys(colors).forEach((key) => {
    const item = document.createElement("span");
    item.className = "meta";
    item.style.color = colors[key];
    item.textContent = labels[key];
    legend.append(item);
  });
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 " + width + " " + height);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Host load average over time");
  const add = (name, attrs, text) => {
    const node = document.createElementNS("http://www.w3.org/2000/svg", name);
    Object.entries(attrs).forEach(([key, value]) =>
      node.setAttribute(key, value),
    );
    if (text !== undefined) node.textContent = text;
    svg.append(node);
  };
  [0, 0.5, 1].forEach((ratio) => {
    const value = maximum * ratio,
      py = y(value);
    add("line", {
      x1: left,
      y1: py,
      x2: width - right,
      y2: py,
      stroke: "var(--line)",
      "stroke-width": 1,
    });
    add(
      "text",
      { x: 4, y: py + 4, fill: "var(--muted)", "font-size": 11 },
      value.toFixed(1),
    );
  });
  Object.keys(colors).forEach((key) =>
    add("polyline", {
      points: samples
        .map(
          (sample, index) =>
            x(index).toFixed(1) + "," + y(sample[key] || 0).toFixed(1),
        )
        .join(" "),
      fill: "none",
      stroke: colors[key],
      "stroke-width": 2,
      "stroke-linejoin": "round",
      "stroke-linecap": "round",
    }),
  );
  const format = (value) => new Date(value).toLocaleTimeString();
  add(
    "text",
    { x: left, y: height - 12, fill: "var(--muted)", "font-size": 11 },
    format(start),
  );
  add(
    "text",
    {
      x: width - right,
      y: height - 12,
      fill: "var(--muted)",
      "font-size": 11,
      "text-anchor": "end",
    },
    format(end),
  );
  wrap.append(legend, svg);
  section.append(wrap);
  return section;
}

let outputWordCloudData = null;
let outputWordCloudKey = "";

function renderOutputWordCloud(details, cloud) {
  const content = details.querySelector(".output-word-cloud-content");
  const status = details.querySelector(".output-word-cloud-status");
  const terms = cloud.terms || [];
  content.textContent = "";
  if (!terms.length) {
    content.textContent = "No output words found.";
  } else {
    const maxCount = Math.max(...terms.map((term) => term.count), 1);
    const colors = [
      "var(--c1)",
      "var(--c2)",
      "var(--c3)",
      "var(--c4)",
      "var(--c5)",
    ];
    terms.forEach((term, index) => {
      const word = document.createElement("span");
      word.textContent = term.word;
      word.title = term.count + " occurrences in " + term.jobs + " jobs";
      word.style.fontSize =
        14 + Math.round((term.count / maxCount) * 28) + "px";
      word.style.color = colors[index % colors.length];
      content.append(word);
    });
  }
  status.textContent =
    cloud.total_jobs +
    " jobs / " +
    cloud.total_bytes.toLocaleString() +
    " bytes / generated " +
    new Date(cloud.generated_at).toLocaleString();
  const button = details.querySelector(".output-word-cloud-regenerate");
  if (button) button.textContent = "Regenerate";
}

async function loadOutputWordCloud(details, projectName, runID, refresh) {
  const status = details.querySelector(".output-word-cloud-status");
  const content = details.querySelector(".output-word-cloud-content");
  const button = details.querySelector(".output-word-cloud-regenerate");
  status.textContent = refresh ? "Regenerating..." : "Loading...";
  button.disabled = true;
  try {
    const query =
      "/api/output-word-cloud?project_name=" +
      encodeURIComponent(projectName) +
      "&run_id=" +
      encodeURIComponent(runID) +
      (refresh ? "&refresh=1" : "");
    const response = await fetch(query);
    if (!response.ok) throw new Error(await response.text());
    outputWordCloudData = await response.json();
    outputWordCloudKey = projectName + "/" + runID;
    renderOutputWordCloud(details, outputWordCloudData);
  } catch (error) {
    content.textContent = "Unable to generate output word cloud.";
    status.textContent = String(error);
  } finally {
    button.disabled = false;
  }
}

/**
 * @param {WebProject} queue
 * @param {string} runID
 */
function outputWordCloudSection(queue, runID) {
  const isStatic = typeof window.__ROTARI_STATIC_STATE__ !== "undefined";
  const wordCloudKey = queue.project_name + "/" + runID;
  const status = document.createElement("div");
  status.className = "graphic-note output-word-cloud-status meta";
  status.textContent = "Not generated";
  const section = runGraphicSection(
    "output-word-cloud",
    "Output word cloud",
    status,
  );
  section.dataset.wordCloudProject = queue.project_name;
  section.dataset.wordCloudRun = runID;
  const body = document.createElement("div");
  const regenerate = document.createElement("button");
  regenerate.className = "output-word-cloud-regenerate";
  regenerate.textContent = "Generate";
  regenerate.type = "button";
  regenerate.onclick = () =>
    loadOutputWordCloud(section, queue.project_name, runID, true);
  const content = document.createElement("div");
  content.className = "output-word-cloud-content";
  body.append(regenerate, content);
  section.append(body);
  if (isStatic) {
    regenerate.hidden = true;
    const cloud = window.__ROTARI_STATIC_WORD_CLOUDS__[wordCloudKey];
    if (cloud) {
      section.dataset.wordCloudLoaded = "true";
      renderOutputWordCloud(section, cloud);
    } else {
      status.textContent = "No output word cloud available";
    }
  }
  if (outputWordCloudData && outputWordCloudKey === wordCloudKey) {
    section.dataset.wordCloudLoaded = "true";
    renderOutputWordCloud(section, outputWordCloudData);
  }
  return section;
}
function renderRunGraphics() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const runID = decodeURIComponent(parts[3]);
  const run = queue && queue.runs.find((item) => item.run_id === runID);
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".run-graphic")) return;
  const sections = [
    runStatisticsSection(run),
    jobTimelineSection(run),
    loadAverageSection(run),
    outputWordCloudSection(queue, runID),
  ];
  const table =
    app.querySelector(".run-jobs-bar") || app.querySelector("table.runs");
  if (table) sections.forEach((section) => app.insertBefore(section, table));
  else app.prepend(...sections);
}
