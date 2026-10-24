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

// The run page's sections: failure causes, job timeline, load average, and
// output word cloud. renderRunGraphics draws them whole, above the jobs
// table. Failure causes and the job timeline start open and the others
// closed; a section the viewer opens or closes stays so in this browser
// (runSectionOpen).
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
  return typeof stored === "boolean"
    ? stored
    : kind === "job-timeline" || kind === "failure-causes";
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
//
// The timeline's height is bounded whatever the job count or parallelism:
// rows shrink from 22px to fit 260px. While a row is at least 16px each lane
// carries its job's name; below that the lanes are drawn as thin rows
// without names, down to 2px, and past that one pixel row holds several
// jobs, drawn failed over running over success. Hovering a row lists its
// jobs; clicking it shows the most notable one's row in the table.
const laneBudget = 260;
const laneMaxHeight = 22;
const laneLabelledHeight = 16;
const laneMinHeight = 2;
/** @type {Record<string, number>} */
const lanePriority = { ok: 0, off: 0, warn: 1, run: 2, bad: 3 };
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
          ? formatDate(start, { month: "short", day: "numeric" }) +
            " " +
            formatClock(start)
          : formatClock(start))
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
  const entries = lanes.map((lane) => {
    const job = byID.get(lane.job_id) || { id: lane.job_id };
    const status = jobDisplayStatus(job, run);
    const start = time(lane.start);
    const finish = time(lane.end);
    const drawn = !lane.carried && Number.isFinite(start);
    const open = drawn && !Number.isFinite(finish);
    const left = drawn ? percent(start) : 0;
    const right = !drawn
      ? 0
      : open
        ? run.running
          ? percent(Date.now())
          : 100
        : percent(finish);
    // A bar keeps a visible width; one that ends at the axis end grows
    // leftward instead of past it.
    const width = Math.max(right - left, 0.6);
    return {
      lane,
      job,
      name: job.name || job.id,
      status,
      tone: statusTone(status),
      drawn,
      faint: open && !run.running,
      left: Math.min(left, 100 - width),
      width,
    };
  });
  const tones = new Set(
    entries.filter((entry) => entry.drawn).map((entry) => entry.tone),
  );
  const rowHeight = Math.min(
    laneMaxHeight,
    Math.max(laneBudget / Math.max(entries.length, 1), laneMinHeight),
  );
  const labelled = rowHeight >= laneLabelledHeight;
  const rows = labelled
    ? laneRowsHTML(entries, rowHeight)
    : denseLanesHTML(entries, rowHeight);
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
  const perRow = entries.length / Math.floor(laneBudget / laneMinHeight);
  const section = runGraphicSection(
    "job-timeline",
    "Job timeline",
    graphicNote(
      entries.length +
        (entries.length === 1 ? " job" : " jobs") +
        (perRow > 1 ? " · 1 row ≈ " + Math.ceil(perRow) + " jobs" : "") +
        " · " +
        formatSpan(span),
      "meta",
    ),
  );
  const chart = document.createElement("div");
  chart.className = "lane-chart";
  chart.innerHTML = entries.length
    ? rows + axis + (tones.size > 1 ? legend : "")
    : '<div class="meta">No jobs.</div>';
  section.append(chart);
  const dense = chart.querySelector(".lane-dense svg");
  if (dense instanceof SVGSVGElement) followDenseLanes(dense, entries);
  return section;
}
/**
 * @typedef {{lane: WebJobLane, job: WebJob, name: string, status: string,
 *   tone: string, drawn: boolean, faint: boolean, left: number,
 *   width: number}} LaneEntry
 */
/**
 * One labelled lane per job.
 * @param {LaneEntry[]} entries
 * @param {number} rowHeight
 */
function laneRowsHTML(entries, rowHeight) {
  return entries
    .map((entry) => {
      const track = entry.lane.carried
        ? '<span class="lane-note" title="carried from an earlier run">carried from an earlier run</span>'
        : !entry.drawn
          ? '<span class="lane-note" title="' +
            esc(entry.status) +
            '">' +
            esc(entry.status) +
            "</span>"
          : '<span class="lane-bar tone-' +
            entry.tone +
            (entry.faint ? " open" : "") +
            '" style="left:' +
            entry.left.toFixed(2) +
            "%;width:" +
            entry.width.toFixed(2) +
            '%" title="' +
            esc(laneTitle(entry)) +
            '"></span>';
      return (
        '<div class="lane-row" style="height:' +
        rowHeight.toFixed(1) +
        'px"><button type="button" class="lane-label" title="Show ' +
        esc(entry.name) +
        ' in the table" onclick="focusMatrixJob(' +
        esc(JSON.stringify(entry.lane.job_id)) +
        ')">' +
        esc(entry.name) +
        '</button><div class="lane-track">' +
        track +
        "</div></div>"
      );
    })
    .join("");
}
/** @param {LaneEntry} entry */
function laneTitle(entry) {
  return (
    entry.name +
    " · " +
    entry.status +
    (entry.job.elapsed ? " · " + entry.job.elapsed : "") +
    (entry.faint ? " · no finish recorded" : "")
  );
}
/**
 * Thin unlabelled lanes in one SVG of at most laneBudget pixels, its width
 * stretched to the plot. Bars are drawn in priority order so a failed job
 * stays visible when it shares a pixel row.
 * @param {LaneEntry[]} entries
 * @param {number} rowHeight
 */
function denseLanesHTML(entries, rowHeight) {
  const height = Math.min(laneBudget, Math.ceil(entries.length * rowHeight));
  const unit = height / entries.length;
  const barHeight = Math.max(1, unit - (unit >= 4 ? 1 : 0));
  const bars = entries
    .map((entry, index) => ({ entry, index }))
    .filter(({ entry }) => entry.drawn)
    .sort(
      (a, b) =>
        (lanePriority[a.entry.tone] || 0) - (lanePriority[b.entry.tone] || 0),
    )
    .map(
      ({ entry, index }) =>
        '<rect class="lane-rect tone-' +
        entry.tone +
        (entry.faint ? " open" : "") +
        '" x="' +
        (entry.left * 10).toFixed(1) +
        '" y="' +
        (index * unit).toFixed(2) +
        '" width="' +
        (entry.width * 10).toFixed(1) +
        '" height="' +
        barHeight.toFixed(2) +
        '"></rect>',
    )
    .join("");
  const grid = [250, 500, 750]
    .map(
      (x) =>
        '<rect class="lane-grid" x="' +
        x +
        '" y="0" width="1" height="' +
        height +
        '"></rect>',
    )
    .join("");
  return (
    '<div class="lane-row lane-dense"><div class="lane-ends"><span>' +
    esc(entries[0].name) +
    "</span><span>" +
    esc(entries[entries.length - 1].name) +
    '</span></div><div class="lane-plot"><svg viewBox="0 0 1000 ' +
    height +
    '" preserveAspectRatio="none" shape-rendering="crispEdges" style="height:' +
    height +
    'px" role="img" aria-label="Job timeline of ' +
    entries.length +
    ' jobs">' +
    grid +
    '<rect class="lane-hover" x="0" y="0" width="1000" height="0"></rect>' +
    bars +
    '</svg><div class="lane-tip" hidden></div></div></div>'
  );
}
/**
 * Hovering the thin lanes lists the jobs in the pointer's row; clicking
 * shows the most notable of them in the table.
 * @param {SVGSVGElement} svg
 * @param {LaneEntry[]} entries
 */
function followDenseLanes(svg, entries) {
  const plot = /** @type {HTMLElement} */ (svg.parentElement);
  const tip = /** @type {HTMLElement} */ (plot.querySelector(".lane-tip"));
  const band = svg.querySelector(".lane-hover");
  const height = Number(svg.getAttribute("viewBox")?.split(" ")[3]) || 1;
  /** @param {PointerEvent | MouseEvent} event */
  const rowAt = (event) => {
    const y = event.clientY - svg.getBoundingClientRect().top;
    const per = entries.length / height;
    const from = Math.max(0, Math.floor((y - 1) * per));
    const to = Math.min(
      entries.length,
      Math.max(from + 1, Math.ceil((y + 1) * per)),
    );
    return { y, hits: entries.slice(from, to) };
  };
  /** @param {LaneEntry[]} hits */
  const notable = (hits) =>
    hits
      .slice()
      .sort(
        (a, b) => (lanePriority[b.tone] || 0) - (lanePriority[a.tone] || 0),
      );
  svg.addEventListener("pointermove", (event) => {
    const { y, hits } = rowAt(event);
    if (!hits.length) return;
    band?.setAttribute("y", String(Math.max(0, y - 1.5)));
    band?.setAttribute("height", "3");
    const shown = notable(hits).slice(0, 6);
    tip.innerHTML =
      '<div class="meta">' +
      (hits.length > 1 ? hits.length + " jobs in this row" : "1 job") +
      "</div>" +
      shown
        .map(
          (entry) =>
            '<div class="lane-tip-row"><i class="lane-swatch tone-' +
            entry.tone +
            '"></i><span>' +
            esc(entry.name) +
            "</span><span>" +
            esc(
              entry.drawn
                ? entry.job.elapsed || entry.status
                : entry.lane.carried
                  ? "carried"
                  : entry.status,
            ) +
            "</span></div>",
        )
        .join("") +
      (hits.length > shown.length
        ? '<div class="meta">+' + (hits.length - shown.length) + " more</div>"
        : "");
    tip.hidden = false;
    tip.style.top = Math.min(y + 12, height + 4) + "px";
  });
  svg.addEventListener("pointerleave", () => {
    tip.hidden = true;
    band?.setAttribute("height", "0");
  });
  svg.addEventListener("click", (event) => {
    const [first] = notable(rowAt(event).hits);
    if (first) focusMatrixJob(first.lane.job_id);
  });
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
  const format = (value) => formatClock(value);
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
    cloud.total_bytes.toLocaleString(uiLocale) +
    " bytes / generated " +
    formatDateTime(cloud.generated_at);
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
// failureGroupMemberLimit caps the jobs a failure cause lists by name; the
// CLI's failureMemberLimit does the same.
const failureGroupMemberLimit = 10;
// The failure causes section lists the run's failed and blocked jobs grouped
// by cause (runlineage.FailureGroups, in lineage_summary): each cause's count
// and exit codes, its first job's evidence line and the suggested fix, its
// jobs, which show their row in the table, and a button that selects them
// for the table's actions. For the project's latest run once it has
// finished, a cause also shows the command that previews a retry of its
// jobs, in the form of the run page's retry command, selected by
// --filter-failure-cause as the CLI's retry hint selects them. It is drawn
// only when the run has a failure.
/**
 * @param {WebRun} run
 * @param {string} runID
 * @param {boolean} latest whether run is the project's latest run
 */
function failureCausesSection(run, runID, latest) {
  const groups = (run.lineage_summary && run.lineage_summary.failures) || [];
  if (!groups.length) return null;
  const total = groups.reduce((sum, group) => sum + group.count, 0);
  const section = runGraphicSection(
    "failure-causes",
    "Failure causes",
    graphicNote(
      total +
        (total === 1 ? " job · " : " jobs · ") +
        groups.length +
        (groups.length === 1 ? " cause" : " causes"),
    ),
  );
  const list = document.createElement("div");
  list.className = "failure-causes";
  groups.forEach((group) => {
    const item = document.createElement("div");
    item.className = "failure-cause";
    const head = document.createElement("div");
    head.className = "failure-cause-head";
    const count = document.createElement("span");
    count.className = "failure-cause-count";
    count.textContent = String(group.count);
    const cause = document.createElement("strong");
    cause.className = "failure-cause-name";
    cause.textContent = group.cause;
    const facts = [];
    if ((group.exit_codes || []).length)
      facts.push("exit " + group.exit_codes.join(","));
    if (group.carried) facts.push(group.carried + " carried");
    head.append(count, cause);
    if (facts.length) head.append(graphicNote(facts.join(" · ")));
    const select = document.createElement("button");
    select.type = "button";
    select.className = "failure-cause-select";
    select.textContent =
      group.count === 1 ? "Select job" : "Select " + group.count + " jobs";
    select.title = "Select this cause's jobs in the table";
    select.onclick = () =>
      selectRunJobIDs((group.jobs || []).map((member) => member.id));
    head.append(select);
    item.append(head);
    if (group.example && group.example.evidence) {
      const evidence = document.createElement("code");
      evidence.className = "failure-cause-evidence";
      evidence.textContent = group.example.evidence;
      evidence.title = group.example.evidence;
      item.append(evidence);
    }
    if (group.suggestion) {
      const fix = document.createElement("p");
      fix.className = "failure-cause-fix meta";
      fix.textContent = "Fix: " + group.suggestion;
      item.append(fix);
    }
    const members = document.createElement("div");
    members.className = "failure-cause-jobs";
    const jobs = group.jobs || [];
    jobs.slice(0, failureGroupMemberLimit).forEach((member) => {
      const button = document.createElement("button");
      button.type = "button";
      button.className = "lane-label";
      button.textContent = member.name || member.id;
      button.title =
        "Show " +
        (member.name || member.id) +
        " in the table" +
        (member.carried ? " (carried from an earlier run)" : "");
      button.onclick = () => focusMatrixJob(member.id);
      members.append(button);
    });
    const rest = jobs.length - failureGroupMemberLimit;
    if (rest > 0) members.append(graphicNote("+" + rest + " more"));
    item.append(members);
    if (latest && !run.running) {
      const command = document.createElement("div");
      command.innerHTML = commandGuideHTML(
        (run.cwd && run.cwd !== "-" ? "cd " + shellQuote(run.cwd) + "\n" : "") +
          "rotari retry -r " +
          shellQuote(runID) +
          " --filter-failure-cause " +
          shellQuote(group.cause) +
          " --dry-run",
      );
      item.append(...command.childNodes);
    }
    list.append(item);
  });
  section.append(list);
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
  const latest = latestRun(queue.runs);
  const sections = [
    failureCausesSection(run, runID, !!latest && latest.run_id === runID),
    jobTimelineSection(run),
    loadAverageSection(run),
    outputWordCloudSection(queue, runID),
  ].filter((section) => section !== null);
  const table =
    app.querySelector(".run-jobs-bar") || app.querySelector("table.runs");
  if (table) sections.forEach((section) => app.insertBefore(section, table));
  else app.prepend(...sections);
}
