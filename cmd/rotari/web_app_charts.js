function addRunHeatmap() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".run-heatmap")) return;
  const section = document.createElement("section");
  section.className = "run-heatmap";
  section.style.background =
    "linear-gradient(135deg,rgba(30,48,58,.95),rgba(24,33,43,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.justifyContent = "space-between";
  heading.style.alignItems = "baseline";
  heading.style.gap = "12px";
  const title = document.createElement("h2");
  title.textContent = "Run heatmap";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.textContent = "Click a tile to inspect the job";
  note.style.color = "var(--muted)";
  note.style.fontSize = "12px";
  heading.append(title, note);
  section.append(heading);
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "10px";
  legend.style.margin = "10px 0 14px";
  const grid = document.createElement("div");
  grid.style.display = "grid";
  grid.style.gridTemplateColumns = "repeat(auto-fit,minmax(120px,1fr))";
  grid.style.gap = "8px";
  const colors = {
    success: ["#1d6b52", "#b4f0c8"],
    failed: ["#8f3b47", "#ffd2d2"],
    blocked: ["#87502d", "#ffe1b0"],
    running: ["#80651e", "#fff0ae"],
    pending: ["#3a4a57", "#cbd9e4"],
  };
  ["success", "failed", "blocked", "running", "pending"].forEach((status) => {
    const item = document.createElement("span");
    item.style.color = colors[status][1];
    item.style.fontSize = "12px";
    const swatch = document.createElement("i");
    swatch.style.display = "inline-block";
    swatch.style.width = "10px";
    swatch.style.height = "10px";
    swatch.style.marginRight = "5px";
    swatch.style.background = colors[status][0];
    swatch.style.border = "1px solid " + colors[status][1];
    item.append(swatch, status);
    legend.append(item);
  });
  section.append(legend, grid);
  (run.jobs || []).forEach((job, index) => {
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
    const tile = document.createElement("button");
    tile.type = "button";
    tile.title = (job.name || job.id) + " - " + status;
    tile.style.display = "flex";
    tile.style.flexDirection = "column";
    tile.style.alignItems = "flex-start";
    tile.style.gap = "2px";
    tile.style.minHeight = "68px";
    tile.style.padding = "10px";
    tile.style.border = "1px solid " + colors[status][1];
    tile.style.borderRadius = "4px";
    tile.style.background = colors[status][0];
    tile.style.color = colors[status][1];
    tile.style.textAlign = "left";
    tile.style.overflow = "hidden";
    const name = document.createElement("strong");
    name.textContent = job.name || job.id;
    name.style.maxWidth = "100%";
    name.style.overflow = "hidden";
    name.style.textOverflow = "ellipsis";
    name.style.whiteSpace = "nowrap";
    const detail = document.createElement("span");
    detail.textContent =
      status +
      (result && result.exit_code !== undefined
        ? " / exit " + result.exit_code
        : "");
    detail.style.fontSize = "12px";
    detail.style.opacity = ".9";
    tile.append(name, detail);
    tile.onclick = () => {
      const row = document.querySelectorAll("#app table.runs tbody tr")[index];
      if (row) {
        row.scrollIntoView({ behavior: "smooth", block: "center" });
        row.style.outline = "2px solid " + colors[status][1];
        setTimeout(() => (row.style.outline = ""), 1200);
      }
    };
    grid.append(tile);
  });
  const table = app.querySelector("table.runs");
  if (table) app.insertBefore(section, table);
  else app.prepend(section);
}
function addRunStatistics() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".run-statistics")) return;
  const counts = { success: 0, failed: 0, blocked: 0, running: 0, pending: 0 };
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
  const colors = {
    success: ["#1d6b52", "#b4f0c8"],
    failed: ["#8f3b47", "#ffd2d2"],
    blocked: ["#87502d", "#ffe1b0"],
    running: ["#80651e", "#fff0ae"],
    pending: ["#3a4a57", "#cbd9e4"],
  };
  const section = document.createElement("section");
  section.className = "run-statistics";
  section.style.background =
    "linear-gradient(135deg,rgba(30,48,58,.95),rgba(24,33,43,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.justifyContent = "space-between";
  heading.style.alignItems = "baseline";
  heading.style.gap = "12px";
  const title = document.createElement("h2");
  title.textContent = "Run statistics";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.textContent = total + " jobs";
  note.style.color = "var(--muted)";
  note.style.fontSize = "12px";
  heading.append(title, note);
  const metrics = document.createElement("div");
  metrics.style.display = "grid";
  metrics.style.gridTemplateColumns = "repeat(auto-fit,minmax(140px,1fr))";
  metrics.style.gap = "12px";
  metrics.style.margin = "16px 0";
  [
    ["Success rate", successRate + "%"],
    ["Succeeded", counts.success],
    ["Failed", counts.failed],
    ["In progress", counts.running],
    ["Pending", counts.pending],
  ].forEach(([label, value]) => {
    const metric = document.createElement("div");
    metric.style.borderLeft = "3px solid #385160";
    metric.style.paddingLeft = "10px";
    const valueElement = document.createElement("strong");
    valueElement.textContent = value;
    valueElement.style.display = "block";
    valueElement.style.fontSize = "22px";
    const labelElement = document.createElement("span");
    labelElement.textContent = label;
    labelElement.style.color = "var(--muted)";
    labelElement.style.fontSize = "12px";
    metric.append(valueElement, labelElement);
    metrics.append(metric);
  });
  const bar = document.createElement("div");
  bar.style.display = "flex";
  bar.style.height = "14px";
  bar.style.overflow = "hidden";
  bar.style.borderRadius = "3px";
  bar.title = "Job status distribution";
  ["success", "failed", "blocked", "running", "pending"].forEach((status) => {
    if (!counts[status]) return;
    const segment = document.createElement("span");
    segment.style.width = (counts[status] / Math.max(total, 1)) * 100 + "%";
    segment.style.background = colors[status][0];
    segment.title = status + ": " + counts[status];
    bar.append(segment);
  });
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "12px";
  legend.style.marginTop = "10px";
  ["success", "failed", "blocked", "running", "pending"].forEach((status) => {
    if (!counts[status]) return;
    const item = document.createElement("span");
    item.textContent = status + " " + counts[status];
    item.style.color = colors[status][1];
    item.style.fontSize = "12px";
    legend.append(item);
  });
  section.append(heading, metrics, bar, legend);
  const table = app.querySelector("table.runs");
  if (table) app.insertBefore(section, table);
  else app.prepend(section);
}
function addRunEnvironment() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".run-environment")) return;
  const section = document.createElement("section");
  section.className = "run-environment";
  section.style.background =
    "linear-gradient(135deg,rgba(25,45,49,.95),rgba(24,33,43,.92))";
  section.style.border = "1px solid #3d5f62";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  section.innerHTML =
    '<div style="display:flex;align-items:baseline;gap:12px"><h2 style="margin:0">Load average</h2></div>';
  const stats = app.querySelector(".run-statistics");
  if (stats) stats.after(section);
  else app.prepend(section);
}
function addJobTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".job-timeline")) return;
  const points = (run.timeline || []).filter((point) => point.at);
  if (points.length < 2) return;
  const times = points
    .map((point) => Date.parse(point.at))
    .filter(Number.isFinite);
  if (!times.length) return;
  const start = Math.min(...times),
    end = Math.max(...times),
    span = Math.max(1, end - start);
  const max = Math.max(
    1,
    ...points.flatMap((point) => [
      point.pending || 0,
      point.running || 0,
      point.success || 0,
      point.failed || 0,
    ]),
  );
  const x = (point) => 40 + ((Date.parse(point.at) - start) / span) * 500;
  const y = (value) => 170 - (value / max) * 130;
  const line = (key) =>
    points
      .map((point) => x(point).toFixed(1) + "," + y(point[key] || 0).toFixed(1))
      .join(" ");
  const label = (value) => new Date(value).toLocaleTimeString();
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  section.innerHTML =
    '<div style="display:flex;justify-content:space-between;align-items:baseline;gap:12px"><h2 style="margin:0">Job timeline</h2><span class="meta">' +
    esc(label(start)) +
    " - " +
    esc(label(end)) +
    '</span></div><svg viewBox="0 0 580 210" role="img" aria-label="job count timeline" style="width:100%;height:auto;margin-top:10px;display:block"><g stroke="#2d3a47" stroke-width="1"><line x1="40" y1="170" x2="540" y2="170"/><line x1="40" y1="40" x2="40" y2="170"/></g><g fill="#94a3b3" font-size="11"><text x="8" y="44">' +
    max +
    '</text><text x="16" y="174">0</text><text x="40" y="194">' +
    esc(label(start)) +
    '</text><text x="460" y="194">' +
    esc(label(end)) +
    '</text></g><polyline fill="none" stroke="#94a3b3" stroke-width="3" points="' +
    line("pending") +
    '"/><polyline fill="none" stroke="#f3c969" stroke-width="3" points="' +
    line("running") +
    '"/><polyline fill="none" stroke="#63d297" stroke-width="3" points="' +
    line("success") +
    '"/><polyline fill="none" stroke="#ff7c7c" stroke-width="3" points="' +
    line("failed") +
    '"/></svg><div class="summary" style="gap:16px"><span style="color:#94a3b3">pending</span><span style="color:#f3c969">running</span><span style="color:#63d297">success</span><span style="color:#ff7c7c">failed</span></div>';
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function addJobTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".job-timeline")) return;
  const counts = { success: 0, failed: 0, blocked: 0, running: 0, pending: 0 };
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
  const total = Math.max(1, (run.jobs || []).length);
  const colors = {
    success: "#63d297",
    failed: "#ff7c7c",
    blocked: "#ff9f68",
    running: "#f3c969",
    pending: "#94a3b3",
  };
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.justifyContent = "space-between";
  heading.style.alignItems = "baseline";
  heading.style.gap = "12px";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.textContent = (run.jobs || []).length + " jobs";
  note.className = "meta";
  heading.append(title, note);
  const bar = document.createElement("div");
  bar.style.display = "flex";
  bar.style.height = "22px";
  bar.style.margin = "16px 0 12px";
  bar.style.overflow = "hidden";
  bar.style.borderRadius = "3px";
  bar.title = "Job status distribution";
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "12px";
  ["success", "failed", "blocked", "running", "pending"].forEach((status) => {
    if (!counts[status]) return;
    const segment = document.createElement("span");
    segment.style.width = (counts[status] / total) * 100 + "%";
    segment.style.background = colors[status];
    segment.title = status + ": " + counts[status];
    bar.append(segment);
    const item = document.createElement("span");
    item.textContent =
      status +
      " " +
      counts[status] +
      " (" +
      Math.round((counts[status] / total) * 100) +
      "%)";
    item.style.color = colors[status];
    item.style.fontSize = "12px";
    legend.append(item);
  });
  section.append(heading, bar, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function addJobTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".job-timeline")) return;
  const points = (run.timeline || []).filter((point) => point.at);
  if (!points.length) return;
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  const keys = ["pending", "running", "success", "failed"];
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.justifyContent = "space-between";
  heading.style.alignItems = "baseline";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.className = "meta";
  note.textContent = points.length + " time points";
  heading.append(title, note);
  const chart = document.createElement("div");
  chart.style.display = "grid";
  chart.style.gap = "7px";
  chart.style.marginTop = "14px";
  points.forEach((point) => {
    const row = document.createElement("div");
    row.style.display = "grid";
    row.style.gridTemplateColumns = "92px 1fr";
    row.style.alignItems = "center";
    row.style.gap = "10px";
    const label = document.createElement("span");
    label.className = "meta";
    label.textContent = new Date(point.at).toLocaleTimeString();
    const bar = document.createElement("div");
    bar.style.display = "flex";
    bar.style.height = "16px";
    bar.style.overflow = "hidden";
    bar.style.borderRadius = "3px";
    bar.title = keys.map((key) => key + ": " + (point[key] || 0)).join(" | ");
    const total = keys.reduce((sum, key) => sum + (point[key] || 0), 0) || 1;
    keys.forEach((key) => {
      const count = point[key] || 0;
      if (!count) return;
      const segment = document.createElement("span");
      segment.style.width = (count / total) * 100 + "%";
      segment.style.background = colors[key];
      bar.append(segment);
    });
    row.append(label, bar);
    chart.append(row);
  });
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "18px";
  legend.style.marginTop = "12px";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.color = colors[key];
    item.style.borderLeft = "3px solid " + colors[key];
    item.style.paddingLeft = "8px";
    legend.append(item);
  });
  section.append(heading, chart, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function addJobTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  const points = ((run && run.timeline) || []).filter((point) => point.at);
  if (!run || !app || app.querySelector(".job-timeline") || !points.length)
    return;
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  const keys = ["pending", "running", "success", "failed"];
  const total = Math.max(1, (run.jobs || []).length);
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.alignItems = "baseline";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.className = "meta";
  note.style.marginLeft = "auto";
  note.textContent = "time → / share ↑";
  heading.append(title, note);
  const plot = document.createElement("div");
  plot.style.display = "flex";
  plot.style.alignItems = "flex-end";
  plot.style.gap = "8px";
  plot.style.height = "190px";
  plot.style.marginTop = "14px";
  plot.style.padding = "8px 8px 0 34px";
  plot.style.borderLeft = "1px solid var(--line)";
  plot.style.borderBottom = "1px solid var(--line)";
  points.forEach((point) => {
    const column = document.createElement("div");
    column.style.flex = "1 1 0";
    column.style.minWidth = "18px";
    column.style.height = "100%";
    column.style.display = "flex";
    column.style.flexDirection = "column";
    column.style.justifyContent = "flex-end";
    const bar = document.createElement("div");
    bar.style.display = "flex";
    bar.style.flexDirection = "column-reverse";
    bar.style.height = "100%";
    bar.style.justifyContent = "flex-start";
    bar.title = keys.map((key) => key + ": " + (point[key] || 0)).join(" | ");
    keys.forEach((key) => {
      const count = point[key] || 0;
      if (!count) return;
      const segment = document.createElement("span");
      segment.style.height = (count / total) * 100 + "%";
      segment.style.background = colors[key];
      segment.style.minHeight = "2px";
      bar.append(segment);
    });
    const label = document.createElement("span");
    label.className = "meta";
    label.style.fontSize = "10px";
    label.style.textAlign = "center";
    label.style.marginTop = "5px";
    label.textContent = new Date(point.at).toLocaleTimeString();
    column.append(bar, label);
    plot.append(column);
  });
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "18px";
  legend.style.marginTop = "12px";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.color = colors[key];
    item.style.borderLeft = "3px solid " + colors[key];
    item.style.paddingLeft = "8px";
    legend.append(item);
  });
  section.append(heading, plot, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function collapseRunGraphics() {
  document.querySelectorAll(".run-statistics").forEach((section) => {
    const content = section.children[1];
    if (content) {
      content.style.display = "grid";
      content.style.gridTemplateColumns = "repeat(5,minmax(0,1fr))";
      content.style.gap = "12px";
    }
  });
  document
    .querySelectorAll(".run-statistics,.run-environment,.job-timeline")
    .forEach((section) => {
      if (section.dataset.collapsible) return;
      section.dataset.collapsible = "true";
      const heading = section.firstElementChild;
      if (!heading) return;
      const key = section.className;
      const button = document.createElement("button");
      button.type = "button";
      button.style.marginRight = "8px";
      button.setAttribute("aria-expanded", String(!!expandedRunGraphics[key]));
      const apply = (expanded) => {
        [...section.children].slice(1).forEach((child, index) => {
          const isTimelinePlot =
            section.classList.contains("job-timeline") && index === 0;
          child.style.display = expanded
            ? isTimelinePlot
              ? "flex"
              : index === 0 && section.classList.contains("run-statistics")
                ? "grid"
                : ""
            : "none";
        });
        button.setAttribute("aria-expanded", String(expanded));
        button.textContent = expanded ? "-" : "+";
        expandedRunGraphics[key] = expanded;
      };
      button.onclick = () => apply(!expandedRunGraphics[key]);
      heading.style.display = "flex";
      heading.style.alignItems = "center";
      heading.insertBefore(button, heading.firstChild);
      apply(!!expandedRunGraphics[key]);
    });
}
function addJobTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  const points = ((run && run.timeline) || []).filter((point) => point.at);
  if (!run || !app || app.querySelector(".job-timeline") || !points.length)
    return;
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  const keys = ["pending", "running", "success", "failed"];
  const total = Math.max(1, (run.jobs || []).length);
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.alignItems = "baseline";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.className = "meta";
  note.style.marginLeft = "auto";
  note.textContent = "time → / share ↑";
  heading.append(title, note);
  const plot = document.createElement("div");
  plot.className = "timeline-plot";
  plot.style.display = "flex";
  plot.style.alignItems = "flex-end";
  plot.style.gap = "10px";
  plot.style.height = "190px";
  plot.style.marginTop = "14px";
  plot.style.padding = "8px 8px 0 34px";
  plot.style.borderLeft = "1px solid var(--line)";
  plot.style.borderBottom = "1px solid var(--line)";
  points.forEach((point) => {
    const column = document.createElement("div");
    column.style.flex = "1 1 0";
    column.style.minWidth = "24px";
    column.style.height = "100%";
    column.style.display = "flex";
    column.style.flexDirection = "column";
    column.style.justifyContent = "flex-end";
    const bar = document.createElement("div");
    bar.className = "timeline-bar";
    bar.style.display = "flex";
    bar.style.flexDirection = "column-reverse";
    bar.style.height = "100%";
    bar.style.justifyContent = "flex-start";
    bar.title = keys.map((key) => key + ": " + (point[key] || 0)).join(" | ");
    keys.forEach((key) => {
      const count = point[key] || 0;
      if (!count) return;
      const segment = document.createElement("span");
      segment.style.height = (count / total) * 100 + "%";
      segment.style.background = colors[key];
      segment.style.minHeight = "2px";
      bar.append(segment);
    });
    const label = document.createElement("span");
    label.className = "meta";
    label.style.fontSize = "10px";
    label.style.textAlign = "center";
    label.style.marginTop = "5px";
    label.textContent = new Date(point.at).toLocaleTimeString();
    column.append(bar, label);
    plot.append(column);
  });
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "18px";
  legend.style.marginTop = "12px";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.display = "inline-flex";
    item.style.color = colors[key];
    item.style.borderLeft = "3px solid " + colors[key];
    item.style.paddingLeft = "8px";
    legend.append(item);
  });
  section.append(heading, plot, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function moveActionColumnsLeft() {
  document.querySelectorAll("#app table.runs").forEach((table) => {
    const headerRow = table.querySelector("thead tr");
    if (!headerRow) return;
    const actionHeader = [...headerRow.children].find(
      (header) => header.textContent.trim() === "Actions",
    );
    if (!actionHeader) return;
    const actionIndex = [...headerRow.children].indexOf(actionHeader);
    headerRow.insertBefore(actionHeader, headerRow.firstChild);
    table.querySelectorAll("tbody tr").forEach((row) => {
      const actionCell = row.children[actionIndex];
      if (actionCell) row.insertBefore(actionCell, row.firstChild);
    });
  });
}
function spaceGraphicLegends() {
  document
    .querySelectorAll(
      ".run-statistics > div:last-child,.job-timeline > div:last-child",
    )
    .forEach((legend) => {
      legend.style.display = "flex";
      legend.style.flexWrap = "wrap";
      legend.style.columnGap = "24px";
      legend.style.rowGap = "8px";
      legend.querySelectorAll("span").forEach((item) => {
        const failed = item.textContent.trim().startsWith("failed");
        if (failed) {
          item.style.color = "#ff7c7c";
        }
        item.style.display = "inline-flex";
        item.style.whiteSpace = "nowrap";
        item.style.borderLeft = "3px solid " + item.style.color;
        item.style.paddingLeft = "10px";
        item.style.paddingRight = "8px";
        item.style.marginRight = "4px";
      });
    });
}
function showTimelineBar() {
  document.querySelectorAll(".job-timeline").forEach((section) => {
    const bar = section.children[1];
    if (bar) bar.style.display = "flex";
  });
}
function renderJobTimelineScratch() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".job-timeline")) return;
  const points = run.timeline || [];
  const keys = ["pending", "running", "success", "failed"];
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  const total = Math.max(1, (run.jobs || []).length);
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.alignItems = "baseline";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.className = "meta";
  note.style.marginLeft = "auto";
  note.textContent = "time → / share ↑";
  heading.append(title, note);
  const plot = document.createElement("div");
  plot.className = "timeline-plot";
  plot.style.display = "flex";
  plot.style.alignItems = "flex-end";
  plot.style.gap = "10px";
  plot.style.height = "190px";
  plot.style.overflowX = "auto";
  plot.style.marginTop = "14px";
  plot.style.padding = "8px 12px 0 34px";
  plot.style.borderLeft = "1px solid var(--line)";
  plot.style.borderBottom = "1px solid var(--line)";
  points.forEach((point) => {
    const column = document.createElement("div");
    column.style.flex = "0 0 28px";
    column.style.width = "28px";
    column.style.height = "100%";
    column.style.display = "flex";
    column.style.flexDirection = "column";
    column.style.justifyContent = "flex-end";
    const bar = document.createElement("div");
    bar.className = "timeline-bar";
    bar.style.display = "flex";
    bar.style.flexDirection = "column-reverse";
    bar.style.height = "100%";
    bar.title = keys.map((key) => key + ": " + (point[key] || 0)).join(" | ");
    keys.forEach((key) => {
      const count = point[key] || 0;
      if (!count) return;
      const segment = document.createElement("span");
      segment.style.height = (count / total) * 100 + "%";
      segment.style.background = colors[key];
      segment.style.minHeight = "2px";
      bar.append(segment);
    });
    const label = document.createElement("span");
    label.className = "meta";
    label.style.fontSize = "10px";
    label.style.textAlign = "center";
    label.style.marginTop = "5px";
    label.textContent = point.at
      ? new Date(point.at).toLocaleTimeString()
      : "-";
    column.append(bar, label);
    plot.append(column);
  });
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "18px";
  legend.style.marginTop = "12px";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.display = "inline-flex";
    item.style.color = colors[key];
    item.style.borderLeft = "3px solid " + colors[key];
    item.style.paddingLeft = "8px";
    legend.append(item);
  });
  section.append(heading, plot, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function renderJobTimelineScratch() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const app = document.getElementById("app");
  if (!run || !app || app.querySelector(".job-timeline")) return;
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
  const keys = ["pending", "running", "success", "failed"];
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  const total = Math.max(1, (run.jobs || []).length);
  const width = Math.max(560, points.length * 100 + 70),
    height = 260,
    left = 42,
    top = 18,
    right = 14,
    bottom = 58,
    plotWidth = width - left - right,
    plotHeight = height - top - bottom;
  const section = document.createElement("section");
  section.className = "job-timeline";
  section.style.background =
    "linear-gradient(135deg,rgba(29,39,49,.95),rgba(20,29,38,.92))";
  section.style.border = "1px solid #385160";
  section.style.padding = "18px";
  section.style.margin = "16px 0 20px";
  const heading = document.createElement("div");
  heading.style.display = "flex";
  heading.style.alignItems = "baseline";
  heading.style.gap = "12px";
  const title = document.createElement("h2");
  title.textContent = "Job timeline";
  title.style.margin = "0";
  const note = document.createElement("span");
  note.className = "meta";
  note.style.marginLeft = "auto";
  note.textContent =
    (rawPoints.length > maxPoints
      ? "sampled to " + maxPoints + " points / "
      : "") + "time → / share ↑";
  heading.append(title, note);
  const chart = document.createElement("div");
  chart.style.overflowX = "auto";
  chart.style.marginTop = "14px";
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 " + width + " " + height);
  svg.setAttribute("role", "img");
  svg.setAttribute("aria-label", "Job timeline chart");
  svg.style.display = "block";
  svg.style.width = width + "px";
  svg.style.height = height + "px";
  const line = (x1, y1, x2, y2, color = "#2d3a47", dash = "") => {
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
    element.setAttribute("fill", "#94a3b3");
    element.setAttribute("font-size", "11");
    element.setAttribute("text-anchor", anchor);
    element.textContent = value;
    svg.append(element);
  };
  [0, 50, 100].forEach((percent) => {
    const y = top + plotHeight - (percent / 100) * plotHeight;
    line(left, y, width - right, y, "#2d3a47", percent ? "4 4" : "");
    text(left - 7, y + 4, percent + "%");
  });
  line(left, top, left, top + plotHeight, "#94a3b3");
  line(left, top + plotHeight, width - right, top + plotHeight, "#94a3b3");
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
      rect.setAttribute("x", x - 14);
      rect.setAttribute("y", y);
      rect.setAttribute("width", 28);
      rect.setAttribute("height", segmentHeight);
      rect.setAttribute("fill", colors[key]);
      rect.setAttribute("rx", "2");
      rect.setAttribute("title", key + ": " + count);
      svg.append(rect);
    });
    line(x, top + plotHeight, x, top + plotHeight + 4, "#94a3b3");
    text(
      x,
      height - 24,
      point.at ? new Date(point.at).toLocaleTimeString() : "-",
      "middle",
    );
  });
  text(width / 2, height - 4, "time", "middle");
  const yLabel = document.createElementNS("http://www.w3.org/2000/svg", "text");
  yLabel.setAttribute("x", "12");
  yLabel.setAttribute("y", height / 2);
  yLabel.setAttribute("fill", "#94a3b3");
  yLabel.setAttribute("font-size", "11");
  yLabel.setAttribute("text-anchor", "middle");
  yLabel.setAttribute("transform", "rotate(-90 12 " + height / 2 + ")");
  yLabel.textContent = "share";
  svg.append(yLabel);
  chart.append(svg);
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.flexWrap = "wrap";
  legend.style.gap = "18px";
  legend.style.marginTop = "10px";
  keys.forEach((key) => {
    const item = document.createElement("span");
    item.textContent = key;
    item.style.display = "inline-flex";
    item.style.color = colors[key];
    item.style.borderLeft = "3px solid " + colors[key];
    item.style.paddingLeft = "8px";
    legend.append(item);
  });
  section.append(heading, chart, legend);
  const env = app.querySelector(".run-environment");
  if (env) env.after(section);
  else app.prepend(section);
}
function syncTimelineBar() {}
function fixTimelineBarWidths() {
  document.querySelectorAll(".timeline-plot").forEach((plot) => {
    plot.style.display = "flex";
    plot.style.flexDirection = "row";
    plot.style.flexWrap = "nowrap";
    plot.style.alignItems = "flex-end";
    plot.style.overflowX = "auto";
    plot.style.height = "230px";
    plot.style.paddingBottom = "46px";
  });
  document.querySelectorAll(".timeline-plot>div").forEach((column) => {
    column.style.flex = "0 0 92px";
    column.style.width = "92px";
    column.style.minWidth = "92px";
    column.style.height = "180px";
    const bar = column.querySelector(".timeline-bar");
    if (bar) {
      bar.style.width = "28px";
      bar.style.height = "160px";
      bar.style.flex = "0 0 160px";
      bar.style.marginLeft = "auto";
      bar.style.marginRight = "auto";
    }
    const label = column.querySelector(".timeline-bar+span");
    if (label) {
      label.style.display = "block";
      label.style.width = "92px";
      label.style.whiteSpace = "nowrap";
      label.style.textAlign = "center";
      label.style.transform = "none";
      label.style.position = "static";
      label.style.fontSize = "10px";
    }
  });
}
function alignTimelineHeading() {
  document
    .querySelectorAll(".job-timeline>div:first-child")
    .forEach((heading) => {
      heading.style.paddingLeft = "0";
    });
}
function alignGraphicHeadings() {
  document
    .querySelectorAll(
      ".run-statistics>div:first-child,.run-environment>div:first-child,.job-timeline>div:first-child",
    )
    .forEach((heading) => {
      heading.style.display = "flex";
      heading.style.justifyContent = "flex-start";
      heading.style.alignItems = "center";
      const title = heading.querySelector("h2");
      const note =
        heading.querySelector(".meta") || heading.querySelector("span");
      if (title) title.style.margin = "0";
      if (note) note.style.marginLeft = "auto";
    });
}
function fixTimelineLegendColors() {
  const colors = {
    pending: "#94a3b3",
    running: "#f3c969",
    success: "#63d297",
    failed: "#ff7c7c",
  };
  document.querySelectorAll(".job-timeline span").forEach((item) => {
    const key = item.textContent.trim();
    if (colors[key]) {
      item.style.color = colors[key];
      item.style.borderLeftColor = colors[key];
    }
  });
}
function fixRunStatisticsColors() {
  const colors = {
    succeeded: "#63d297",
    failed: "#ff7c7c",
    "in progress": "#f3c969",
    pending: "#94a3b3",
  };
  document.querySelectorAll(".run-statistics strong").forEach((value) => {
    const metric = value.parentElement;
    const label = metric ? metric.textContent.toLowerCase() : "";
    const key = Object.keys(colors).find((name) => label.includes(name));
    if (key) value.style.color = colors[key];
  });
  document
    .querySelectorAll(".run-statistics .meta,.run-statistics div span")
    .forEach((label) => {
      label.style.color = "var(--muted)";
    });
}
function addLoadTimeline() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (item) => item.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const section = document.querySelector(".run-environment");
  const samples = (
    (run && run.context && run.context.load_samples) ||
    []
  ).filter((sample) => sample.at);
  if (!section || samples.length < 2 || section.querySelector(".load-timeline"))
    return;
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
  const colors = { one: "#63d297", five: "#f3c969", fifteen: "#70b7ff" };
  const labels = { one: "1 min", five: "5 min", fifteen: "15 min" };
  const wrap = document.createElement("div");
  wrap.className = "load-timeline";
  wrap.style.marginTop = "16px";
  wrap.style.overflowX = "auto";
  const legend = document.createElement("div");
  legend.style.display = "flex";
  legend.style.gap = "20px";
  legend.style.marginBottom = "8px";
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
  svg.style.display = "block";
  svg.style.width = "100%";
  svg.style.minWidth = "520px";
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
      stroke: "#344451",
      "stroke-width": 1,
    });
    add(
      "text",
      { x: 4, y: py + 4, fill: "#94a3b3", "font-size": 11 },
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
    { x: left, y: height - 12, fill: "#94a3b3", "font-size": 11 },
    format(start),
  );
  add(
    "text",
    {
      x: width - right,
      y: height - 12,
      fill: "#94a3b3",
      "font-size": 11,
      "text-anchor": "end",
    },
    format(end),
  );
  wrap.append(legend, svg);
  section.append(wrap);
}
function simplifyRunStatistics() {
  document.querySelectorAll(".run-statistics").forEach((section) => {
    [...section.children].slice(1).forEach((child) => {
      child.style.display = "none";
    });
  });
}
function addRunHostsColumn() {
  const parts = pageParts();
  if (parts[0] !== "project" || parts[2] !== "run") return;
  const queue = state.projects.find(
    (q) => q.project_name === decodeURIComponent(parts[1]),
  );
  const run =
    queue &&
    queue.runs.find((item) => item.run_id === decodeURIComponent(parts[3]));
  const table = document.querySelector("#app table.runs");
  if (!run || !table || table.querySelector(".job-host-header")) return;
  const headers = [...table.querySelectorAll("thead th")];
  const commandIndex = headers.findIndex(
    (header) => header.textContent.trim() === "Command",
  );
  if (commandIndex < 0) return;
  const header = document.createElement("th");
  header.className = "job-host-header";
  header.dataset.sort = "hosts";
  header.textContent = "Hosts";
  headers[commandIndex].after(header);
  table.querySelectorAll("tbody tr").forEach((row, index) => {
    const cell = document.createElement("td");
    const hosts =
      (run.jobs[index] &&
        run.jobs[index].result &&
        run.jobs[index].result.hosts) ||
      [];
    cell.textContent = hosts.length ? hosts.join(",") : "-";
    row.children[commandIndex].after(cell);
  });
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
  const server = state.server || {};
  const coordinator =
    (server.socket_exists ? "socket present" : "socket absent") +
    (server.pid_file_exists
      ? server.pid
        ? " / PID " + server.pid
        : " / PID record unreadable"
      : " / no PID record");
  const section = document.createElement("section");
  section.className = "project-runtime";
  section.innerHTML =
    '<h2>Project runtime</h2><div class="summary"><span class="' +
    (active ? "status-running" : "meta") +
    '">' +
    esc(runner) +
    "</span></div><details" +
    (projectRuntimeDetailsOpen ? " open" : "") +
    '><summary>Internal state</summary><div class="meta" style="margin-top:10px">Runner lock: ' +
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
