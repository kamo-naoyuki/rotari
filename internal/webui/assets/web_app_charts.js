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
// table; each starts collapsed and remembers, while the page is open,
// whether it was opened (expandedRunGraphics).
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
  const apply = (expanded) => {
    section.classList.toggle("collapsed", !expanded);
    toggle.setAttribute("aria-expanded", String(expanded));
    toggle.textContent = expanded ? "-" : "+";
    expandedRunGraphics[kind] = expanded;
  };
  toggle.onclick = () => apply(!expandedRunGraphics[kind]);
  apply(!!expandedRunGraphics[kind]);
  return section;
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
/** @param {WebRun} run */
function jobTimelineSection(run) {
  const rawPoints = run.timeline || [];
  const maxPoints = 10;
  const points =
    rawPoints.length > maxPoints
      ? Array.from(
          { length: maxPoints },
          (_, index) =>
            rawPoints[
              Math.round((index * (rawPoints.length - 1)) / (maxPoints - 1))
            ],
        )
      : rawPoints;
  const clockLabels = points.map((point) =>
    new Date(point.at).toLocaleTimeString(),
  );
  const hasRepeatedClockLabels =
    new Set(clockLabels).size !== clockLabels.length;
  const timeLabels = points.map((point, index) => {
    const date = new Date(point.at);
    const label = hasRepeatedClockLabels
      ? date.toLocaleString(undefined, {
          year: "numeric",
          month: "short",
          day: "numeric",
          hour: "2-digit",
          minute: "2-digit",
          second: "2-digit",
        })
      : clockLabels[index];
    return index === 0 ? "start · " + label : label;
  });
  const keys = ["pending", "running", "success", "failed"];
  const colors = statusColorMap(keys);
  const total = Math.max(1, (run.jobs || []).length);
  const width = Math.max(560, points.length * 100 + 70),
    height = 260,
    left = 42,
    top = 18,
    right = 14,
    bottom = 58,
    plotWidth = width - left - right,
    plotHeight = height - top - bottom;
  const section = runGraphicSection(
    "job-timeline",
    "Job timeline",
    graphicNote(
      (rawPoints.length > maxPoints
        ? "sampled to " + maxPoints + " points / "
        : "") + "time → / share ↑",
      "meta",
    ),
  );
  const chart = document.createElement("div");
  chart.className = "job-timeline-chart";
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 " + width + " " + height);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Job timeline chart");
  svg.style.width = width + "px";
  svg.style.height = height + "px";
  const line = (x1, y1, x2, y2, color = "var(--line)", dash = "") => {
    const element = document.createElementNS(
      "http://www.w3.org/2000/svg",
      "line",
    );
    Object.entries({
      x1,
      y1,
      x2,
      y2,
      stroke: color,
      "stroke-width": "1",
    }).forEach(([key, value]) => element.setAttribute(key, value));
    if (dash) element.setAttribute("stroke-dasharray", dash);
    svg.append(element);
  };
  const text = (x, y, value, anchor = "end") => {
    const element = document.createElementNS(
      "http://www.w3.org/2000/svg",
      "text",
    );
    element.setAttribute("x", x);
    element.setAttribute("y", y);
    element.setAttribute("fill", "var(--muted)");
    element.setAttribute("font-size", "11");
    element.setAttribute("text-anchor", anchor);
    element.textContent = value;
    svg.append(element);
  };
  [0, 50, 100].forEach((percent) => {
    const y = top + plotHeight - (percent / 100) * plotHeight;
    line(left, y, width - right, y, "var(--line)", percent ? "4 4" : "");
    text(left - 7, y + 4, percent + "%");
  });
  line(left, top, left, top + plotHeight, "var(--muted)");
  line(left, top + plotHeight, width - right, top + plotHeight, "var(--muted)");
  points.forEach((point, index) => {
    const x = left + ((index + 0.5) / Math.max(points.length, 1)) * plotWidth;
    let y = top + plotHeight;
    keys.forEach((key) => {
      const count = point[key] || 0;
      if (!count) return;
      const segmentHeight = (count / total) * plotHeight;
      y -= segmentHeight;
      const rect = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "rect",
      );
      rect.setAttribute("x", String(x - 14));
      rect.setAttribute("y", String(y));
      rect.setAttribute("width", "28");
      rect.setAttribute("height", String(segmentHeight));
      rect.setAttribute("fill", colors[key]);
      rect.setAttribute("rx", "2");
      rect.setAttribute("title", key + ": " + count);
      svg.append(rect);
    });
    line(x, top + plotHeight, x, top + plotHeight + 4, "var(--muted)");
    text(x, height - 24, point.at ? timeLabels[index] : "-", "middle");
  });
  text(width / 2, height - 4, "time", "middle");
  const yLabel = document.createElementNS("http://www.w3.org/2000/svg", "text");
  yLabel.setAttribute("x", "12");
  yLabel.setAttribute("y", String(height / 2));
  yLabel.setAttribute("fill", "var(--muted)");
  yLabel.setAttribute("font-size", "11");
  yLabel.setAttribute("text-anchor", "middle");
  yLabel.setAttribute("transform", "rotate(-90 12 " + height / 2 + ")");
  yLabel.textContent = "share";
  svg.append(yLabel);
  chart.append(svg);
  const legend = document.createElement("div");
  legend.className = "graphic-legend";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.color = colors[key];
    item.style.borderLeftColor = colors[key];
    legend.append(item);
  });
  section.append(chart, legend);
  return section;
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
  const table = app.querySelector("table.runs");
  if (table) sections.forEach((section) => app.insertBefore(section, table));
  else app.prepend(...sections);
}

function addProjectRuntime() {
  const parts = pageParts();
  if (parts.length !== 2 || parts[0] !== "project") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const app = document.getElementById("app");
  if (!queue || !app || app.querySelector(".project-runtime")) return;
  const active = !!queue.running_run_id;
  const runner = active
    ? "Active runner: " +
      queue.running_run_id +
      (queue.runner_host ? " on " + queue.runner_host : "") +
      (queue.runner_pid ? " (PID " + queue.runner_pid + ")" : "")
    : "No runner lock";
  const server = queue.server || {};
  const coordinator = server.pid_file_exists
    ? server.pid
      ? "PID " + server.pid
      : "PID record unreadable"
    : "no PID record";
  const section = document.createElement("section");
  section.className = "project-runtime";
  section.innerHTML =
    '<h2>Project runtime</h2><div class="summary"><span class="' +
    (active ? statusPillClass("running") : "meta") +
    '">' +
    esc(runner) +
    "</span></div><details" +
    (projectRuntimeDetailsOpen ? " open" : "") +
    '><summary>Internal state</summary><div class="meta runtime-details">Runner lock: ' +
    (active ? "present" : "absent") +
    (queue.runner_started_at
      ? " | Started: " + esc(queue.runner_started_at)
      : "") +
    "<br>Coordinator record: " +
    esc(coordinator) +
    "<br>State lock: advisory and intentionally not probed</div></details>";
  const controls = app.querySelector(".toolbar");
  if (controls) controls.after(section);
  else app.prepend(section);
}
