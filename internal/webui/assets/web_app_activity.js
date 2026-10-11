// The Activity page: runs across every registered basedir, as a month
// calendar and one day's timeline, with the projects worked on. It reads
// /api/activity for the month on show (WEB-10); the server decides which
// runs meet the range and where each run's span ends, and the page sorts
// them into the browser's days.
const activityView = {
  month: null,
  day: null,
  data: null,
  loading: "",
  fallback: false,
  busyHours: false,
  expanded: false,
  collapsed: new Set(),
  // actor narrows the page to runs with that actor label, "" to unlabelled
  // runs, or null to every run.
  actor: null,
};
const activityLaneLimit = 12;
const activityProjectColors = 8;

function activityDayStart(date) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate());
}
function activityAddDays(date, days) {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + days);
}
function activitySameDay(a, b) {
  return (
    a.getFullYear() === b.getFullYear() &&
    a.getMonth() === b.getMonth() &&
    a.getDate() === b.getDate()
  );
}
function activityDayKey(date) {
  return (
    date.getFullYear() +
    "-" +
    String(date.getMonth() + 1).padStart(2, "0") +
    "-" +
    String(date.getDate()).padStart(2, "0")
  );
}
function activityClock(date) {
  return (
    String(date.getHours()).padStart(2, "0") +
    ":" +
    String(date.getMinutes()).padStart(2, "0")
  );
}
function activityDate(date) {
  return formatDate(date, {
    weekday: "short",
    month: "short",
    day: "numeric",
  });
}
// The calendar shows whole weeks, Monday first.
function activityGrid(month) {
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  const last = new Date(month.getFullYear(), month.getMonth() + 1, 0);
  return {
    start: activityAddDays(first, -((first.getDay() + 6) % 7)),
    end: activityAddDays(last, 7 - ((last.getDay() + 6) % 7)),
  };
}
function activityIsStatic() {
  return typeof rewriteStaticLinks === "function";
}
// activityRuns turns the response's runs into dated runs, each with its
// project key, colour, and tone.
function activityRuns(data) {
  return (data.runs || []).map((run) => ({
    ...run,
    key: run.basedir_id + "/" + run.project_name,
    start: new Date(run.started_at),
    end: new Date(run.ends_at),
    tone: statusTone(run.lifecycle),
  }));
}
// activityOnDay keeps the runs whose span meets the day, a point run when it
// started on the day.
function activityOnDay(runs, day) {
  const from = activityDayStart(day);
  const to = activityAddDays(from, 1);
  return runs.filter((run) =>
    +run.start === +run.end
      ? run.start >= from && run.start < to
      : run.start < to && run.end > from,
  );
}
function activityClippedMs(run, day) {
  const from = activityDayStart(day);
  const to = activityAddDays(from, 1);
  return Math.max(0, Math.min(+run.end, +to) - Math.max(+run.start, +from));
}
// activityColors gives the eight projects with the most running time in the
// shown month their own colour; the rest share grey. It reads every run, so
// a filter never repaints a project.
function activityColors(runs, month) {
  const from = new Date(month.getFullYear(), month.getMonth(), 1);
  const to = new Date(month.getFullYear(), month.getMonth() + 1, 1);
  const time = new Map();
  runs.forEach((run) => {
    const ms = Math.max(
      0,
      Math.min(+run.end, +to) - Math.max(+run.start, +from),
    );
    if (run.start < to && run.end >= from)
      time.set(run.key, (time.get(run.key) || 0) + ms);
  });
  const colors = new Map();
  [...time.entries()]
    .sort((a, b) => b[1] - a[1] || (a[0] < b[0] ? -1 : 1))
    .forEach(([key], index) =>
      colors.set(
        key,
        index < activityProjectColors
          ? "var(--c" + (index + 1) + ")"
          : "var(--s-off)",
      ),
    );
  return colors;
}
function activityColor(colors, key) {
  return colors.get(key) || "var(--s-off)";
}
function activityBasedirs() {
  const byID = new Map();
  (activityView.data?.basedirs || registeredBasedirs).forEach((entry) =>
    byID.set(entry.id, entry),
  );
  return byID;
}
function activityRunURL(run) {
  return basedirURL(
    run.basedir_id,
    "/project/" +
      encodeURIComponent(run.project_name) +
      "/run/" +
      encodeURIComponent(run.run_id),
  );
}

function renderActivityPage() {
  setCrumbs([["Activity"]], "");
  document.getElementById("page-title").textContent = "Activity";
  document.getElementById("summary").textContent = activityIsStatic()
    ? "Runs in this static export's state directory."
    : "Runs across every registered state directory.";
  const app = document.getElementById("app");
  if (app.querySelector(".activity-page")) return;
  app.innerHTML =
    '<div class="activity-page"><div class="activity-problems"></div>' +
    '<div class="activity-toolbar"><button type="button" class="activity-prev-month" aria-label="Previous month">◀</button>' +
    '<h2 class="activity-month-title"></h2>' +
    '<button type="button" class="activity-next-month" aria-label="Next month">▶</button>' +
    '<button type="button" class="activity-today">Today</button>' +
    '<label class="activity-actor">Started by <select class="activity-actor-select"></select></label></div>' +
    '<section class="activity-calendar-panel" aria-label="Month calendar"><div class="activity-calendar"></div><div class="activity-calendar-foot"></div></section>' +
    '<section class="activity-day-panel" aria-label="Selected day"><div class="activity-day"></div></section>' +
    '<section class="activity-projects-panel" aria-label="Where you worked"><div class="activity-projects"></div></section>' +
    '<div class="activity-card" hidden></div></div>';
  const page = app.querySelector(".activity-page");
  page.querySelector(".activity-prev-month").onclick = () =>
    activityShowMonth(-1);
  page.querySelector(".activity-next-month").onclick = () =>
    activityShowMonth(1);
  page.querySelector(".activity-today").onclick = () =>
    activitySelectDay(new Date());
  page.querySelector(".activity-actor-select").onchange = (event) => {
    const value = event.target.value;
    activityView.actor = value === "*" ? null : value === "-" ? "" : value;
    activityView.expanded = false;
    activityDraw();
  };
  activityFollowPointer(page);
  const params = new URLSearchParams(location.search);
  const day = params.get("day");
  const month = params.get("month");
  if (day && /^\d{4}-\d{2}-\d{2}$/.test(day)) {
    const [y, m, d] = day.split("-").map(Number);
    activityView.day = new Date(y, m - 1, d);
  } else if (month && /^\d{4}-\d{2}$/.test(month)) {
    const [y, m] = month.split("-").map(Number);
    activityView.month = new Date(y, m - 1, 1);
  }
  activityView.fallback = false;
  activityStart().catch((error) => {
    page.querySelector(".activity-problems").innerHTML =
      '<div class="empty error">' + esc(error.message) + "</div>";
  });
}

// activityStart opens the requested day or month, or else today; when today
// has no runs it opens the latest day with runs, found from the response's
// latest run start in one more request, and says so.
async function activityStart() {
  const today = activityDayStart(new Date());
  if (activityView.day) {
    activityView.month = new Date(
      activityView.day.getFullYear(),
      activityView.day.getMonth(),
      1,
    );
    await activityLoad();
    return activityDraw();
  }
  if (activityView.month) {
    await activityLoad();
    const runs = activityRuns(activityView.data);
    const last = activityLastDay(runs, activityView.month, today);
    activityView.day =
      last ||
      (activityView.month.getMonth() === today.getMonth() &&
      activityView.month.getFullYear() === today.getFullYear()
        ? today
        : new Date(
            activityView.month.getFullYear(),
            activityView.month.getMonth(),
            1,
          ));
    return activityDraw();
  }
  activityView.month = new Date(today.getFullYear(), today.getMonth(), 1);
  activityView.day = today;
  await activityLoad();
  if (activityOnDay(activityRuns(activityView.data), today).length)
    return activityDraw();
  const latest = activityView.data.latest_started_at
    ? new Date(activityView.data.latest_started_at)
    : null;
  if (latest) {
    activityView.month = new Date(latest.getFullYear(), latest.getMonth(), 1);
    await activityLoad();
    const runs = activityRuns(activityView.data);
    const last =
      activityLastDay(runs, activityView.month, today) ||
      activityDayStart(latest);
    activityView.day = last;
    activityView.fallback = true;
  }
  activityDraw();
}
// activityLastDay is the latest day of month, not after today, with a run.
function activityLastDay(runs, month, today) {
  const first = new Date(month.getFullYear(), month.getMonth(), 1);
  let day = new Date(month.getFullYear(), month.getMonth() + 1, 0);
  if (day > today) day = today;
  for (; day >= first; day = activityAddDays(day, -1)) {
    if (activityOnDay(runs, day).length) return day;
  }
  return null;
}
async function activityLoad() {
  const grid = activityGrid(activityView.month);
  const query =
    "from=" +
    encodeURIComponent(grid.start.toISOString().replace(/\.\d{3}Z$/, "Z")) +
    "&to=" +
    encodeURIComponent(grid.end.toISOString().replace(/\.\d{3}Z$/, "Z"));
  if (activityView.loading === query && activityView.data) return;
  const response = await fetch("/api/activity?" + query);
  if (!response.ok) throw new Error(await response.text());
  activityView.data = await response.json();
  activityView.loading = query;
}
async function activityShowMonth(step) {
  activityView.month = new Date(
    activityView.month.getFullYear(),
    activityView.month.getMonth() + step,
    1,
  );
  await activityLoad();
  activityDraw();
  history.replaceState(
    null,
    "",
    location.pathname +
      "?month=" +
      activityDayKey(activityView.month).slice(0, 7),
  );
}
async function activitySelectDay(day) {
  activityView.day = activityDayStart(day);
  activityView.fallback = false;
  activityView.expanded = false;
  const month = new Date(day.getFullYear(), day.getMonth(), 1);
  if (+month !== +activityView.month) {
    activityView.month = month;
    await activityLoad();
  }
  activityDraw();
  history.replaceState(
    null,
    "",
    location.pathname + "?day=" + activityDayKey(activityView.day),
  );
}

function activityDraw() {
  const page = document.querySelector(".activity-page");
  if (!page || !activityView.data) return;
  const all = activityRuns(activityView.data);
  // Colours come from every run, so narrowing by actor repaints nothing.
  const colors = activityColors(all, activityView.month);
  const runs =
    activityView.actor === null
      ? all
      : all.filter(
          (run) => (run.started_by?.actor || "") === activityView.actor,
        );
  activityDrawActors(page, all);
  activityDrawProblems(page);
  activityDrawCalendar(page, runs, colors);
  activityDrawDay(page, runs, colors);
  activityDrawProjects(page, runs, colors);
}
// activityDrawActors offers every actor label among the loaded runs, and
// unlabelled runs; a run without an actor is unlabelled, never a person's.
function activityDrawActors(page, runs) {
  const actors = [
    ...new Set(runs.map((run) => run.started_by?.actor).filter(Boolean)),
  ].sort();
  if (activityView.actor && !actors.includes(activityView.actor))
    actors.push(activityView.actor);
  const selected =
    activityView.actor === null
      ? "*"
      : activityView.actor === ""
        ? "-"
        : activityView.actor;
  page.querySelector(".activity-actor-select").innerHTML =
    '<option value="*">all runs</option>' +
    actors
      .map(
        (actor) =>
          '<option value="' + esc(actor) + '">' + esc(actor) + "</option>",
      )
      .join("") +
    '<option value="-">unlabelled</option>';
  page.querySelector(".activity-actor-select").value = selected;
}
function activityDrawProblems(page) {
  const notes = [];
  if (activityIsStatic()) {
    const entry = (activityView.data.basedirs || [])[0];
    notes.push(
      "This static export holds one state directory" +
        (entry ? ", " + basedirLabelHTML(entry) : "") +
        ". Run <code>rotari web</code> to see every registered one.",
    );
  }
  (activityView.data.basedirs || []).forEach((entry) => {
    if (entry.missing)
      notes.push(
        basedirLabelHTML(entry) +
          " is registered but no longer exists (" +
          esc(entry.path) +
          ").",
      );
    else if (entry.error)
      notes.push(
        basedirLabelHTML(entry) + " could not be read: " + esc(entry.error),
      );
  });
  page.querySelector(".activity-problems").innerHTML = notes
    .map((note) => '<div class="activity-notice">' + note + "</div>")
    .join("");
}

function activityDayStats(runs, day) {
  const list = activityOnDay(runs, day);
  const perProject = new Map();
  let ms = 0;
  list.forEach((run) => {
    const clipped = activityClippedMs(run, day);
    ms += clipped;
    perProject.set(run.key, (perProject.get(run.key) || 0) + clipped);
  });
  return {
    list,
    ms,
    perProject,
    projects: [...perProject.keys()].sort(
      (a, b) => perProject.get(b) - perProject.get(a),
    ),
    failed: list.filter((run) => run.tone === "bad" || run.tone === "warn")
      .length,
    running: list.some((run) => run.lifecycle === "running"),
  };
}
// A day's shade grows with the time its runs spent running.
function activityHeat(ms) {
  const hours = ms / 3600000;
  return hours <= 0
    ? 0
    : hours < 1
      ? 1
      : hours < 3
        ? 2
        : hours < 6
          ? 3
          : hours < 12
            ? 4
            : 5;
}
function activityProjectName(key) {
  return key.slice(key.indexOf("/") + 1);
}
function activityDrawCalendar(page, runs, colors) {
  const month = activityView.month;
  const today = activityDayStart(new Date());
  page.querySelector(".activity-month-title").textContent = formatDate(month, {
    month: "long",
    year: "numeric",
  });
  page.querySelector(".activity-next-month").disabled =
    month.getFullYear() === today.getFullYear() &&
    month.getMonth() === today.getMonth();
  const grid = activityGrid(month);
  let html = ["Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"]
    .map((name) => '<div class="activity-dow">' + name + "</div>")
    .join("");
  const shown = new Set();
  for (let day = grid.start; day < grid.end; day = activityAddDays(day, 1)) {
    const outside = day.getMonth() !== month.getMonth();
    const future = day > today;
    const stats = future ? null : activityDayStats(runs, day);
    if (stats && !outside) stats.projects.forEach((key) => shown.add(key));
    const count = stats ? stats.list.length : 0;
    const dots = stats
      ? stats.projects
          .slice(0, 3)
          .map(
            (key) =>
              '<span class="activity-dot" style="background:' +
              activityColor(colors, key) +
              '"></span>',
          )
          .join("") +
        (stats.projects.length > 3
          ? '<span class="activity-more-dots">+' +
            (stats.projects.length - 3) +
            "</span>"
          : "")
      : "";
    const classes = [
      "activity-cell",
      "heat-" + (stats ? activityHeat(stats.ms) : 0),
      outside && "outside",
      activitySameDay(day, today) && "today",
      activityView.day && activitySameDay(day, activityView.day) && "selected",
    ]
      .filter(Boolean)
      .join(" ");
    html +=
      '<button type="button" class="' +
      classes +
      '" data-day="' +
      +day +
      '"' +
      (future ? " disabled" : "") +
      ' aria-label="' +
      esc(activityDate(day)) +
      ": " +
      count +
      (count === 1 ? " run" : " runs") +
      (stats && stats.failed ? ", " + stats.failed + " failed" : "") +
      '"><span class="activity-cell-head"><span class="activity-day-number">' +
      day.getDate() +
      "</span>" +
      (count
        ? '<span class="activity-count">' +
          count +
          '<span class="activity-count-unit">' +
          (count === 1 ? " run" : " runs") +
          "</span></span>"
        : "") +
      '</span><span class="activity-dots">' +
      dots +
      '</span><span class="activity-marks">' +
      (stats && stats.failed
        ? '<span class="activity-mark-bad">✕' + stats.failed + "</span>"
        : "") +
      (stats && stats.running
        ? '<span class="activity-mark-run" title="running">▶</span>'
        : "") +
      "</span></button>";
  }
  const calendar = page.querySelector(".activity-calendar");
  calendar.innerHTML = html;
  calendar
    .querySelectorAll(".activity-cell:not([disabled])")
    .forEach(
      (cell) =>
        (cell.onclick = () =>
          activitySelectDay(new Date(Number(cell.dataset.day)))),
    );
  const legend = [...shown]
    .sort((a, b) =>
      activityColor(colors, a) < activityColor(colors, b) ? -1 : 1,
    )
    .map(
      (key) =>
        '<span class="activity-legend-item"><span class="activity-dot" style="background:' +
        activityColor(colors, key) +
        '"></span>' +
        esc(activityProjectName(key)) +
        "</span>",
    )
    .join("");
  page.querySelector(".activity-calendar-foot").innerHTML =
    '<div class="activity-legend">' +
    (legend || '<span class="meta">No runs this month.</span>') +
    '</div><div class="activity-legend meta"><span class="activity-heat-key">Less <i class="heat-1"></i><i class="heat-2"></i><i class="heat-3"></i><i class="heat-4"></i><i class="heat-5"></i> more time running</span><span><span class="activity-mark-bad">✕</span> failed or interrupted</span><span><span class="activity-mark-run">▶</span> running</span></div>';
}

function activityDrawDay(page, runs, colors) {
  const container = page.querySelector(".activity-day");
  const day = activityView.day;
  const today = activityDayStart(new Date());
  const stats = activityDayStats(runs, day);
  const basedirs = activityBasedirs();
  const notice =
    activityView.fallback && !activitySameDay(day, today)
      ? '<div class="activity-notice">No runs today. Showing <strong>' +
        esc(activityDate(day)) +
        "</strong>, the latest day with runs.</div>"
      : "";
  const head =
    '<div class="activity-day-head"><div><h2>' +
    esc(
      formatDate(day, {
        weekday: "long",
        year: "numeric",
        month: "long",
        day: "numeric",
      }),
    ) +
    (activitySameDay(day, today) ? " · today" : "") +
    '</h2><div class="meta">' +
    (stats.list.length
      ? stats.list.length +
        (stats.list.length === 1 ? " run · " : " runs · ") +
        stats.projects.length +
        (stats.projects.length === 1 ? " project · " : " projects · ") +
        formatSpan(stats.ms) +
        " running"
      : "") +
    '</div></div><div class="activity-day-controls"><span class="activity-zoom" role="group" aria-label="Time range"><button type="button" class="activity-zoom-full" aria-pressed="' +
    !activityView.busyHours +
    '">00–24</button><button type="button" class="activity-zoom-busy" aria-pressed="' +
    activityView.busyHours +
    '">Busy hours</button></span><button type="button" class="activity-prev-day" aria-label="Previous day">‹</button><button type="button" class="activity-next-day" aria-label="Next day"' +
    (day >= today ? " disabled" : "") +
    ">›</button></div></div>";
  if (!stats.list.length) {
    container.innerHTML =
      notice +
      head +
      '<div class="meta activity-empty">No runs on this day' +
      (activityView.actor === null ? "" : " for this actor") +
      ".</div>";
    activityBindDay(container, day);
    return;
  }
  const from = activityDayStart(day);
  const to = activityAddDays(from, 1);
  let start = from;
  let end = to;
  if (activityView.busyHours) {
    const low = Math.max(
      +from,
      Math.min(...stats.list.map((run) => +run.start)),
    );
    const high = Math.min(+to, Math.max(...stats.list.map((run) => +run.end)));
    start = new Date(+from + Math.floor((low - +from) / 3600000) * 3600000);
    end = new Date(+from + Math.ceil((high - +from) / 3600000) * 3600000);
    if (end - start < 2 * 3600000)
      end = new Date(Math.min(+to, +start + 2 * 3600000));
  }
  const span = end - start;
  const percent = (time) =>
    ((Math.min(Math.max(+time, +start), +end) - start) / span) * 100;
  const hours = span / 3600000;
  const step = hours <= 4 ? 0.5 : hours <= 8 ? 1 : hours <= 14 ? 2 : 3;
  const ticks = [];
  for (let time = +start; time <= +end + 1; time += step * 3600000)
    ticks.push(time);
  const now = new Date(activityView.data.now || Date.now());
  const nowInside = now > start && now < end;
  const tickHTML = (labelled) =>
    ticks
      .map(
        (time, index) =>
          '<div class="activity-tick' +
          (index === 0 ? " first" : index === ticks.length - 1 ? " last" : "") +
          '" style="left:' +
          percent(time) +
          '%">' +
          // A tick label next to the now line would collide with it.
          (labelled &&
          !(nowInside && Math.abs(percent(time) - percent(now)) < 5)
            ? "<span>" +
              (time >= +to ? "24:00" : activityClock(new Date(time))) +
              "</span>"
            : "") +
          "</div>",
      )
      .join("") +
    (nowInside
      ? '<div class="activity-now" style="left:' +
        percent(now) +
        '%">' +
        (labelled ? "<span>now " + activityClock(now) + "</span>" : "") +
        "</div>"
      : "");
  // One lane per (basedir, project) active on the day, newest first, grouped
  // by basedir; past activityLaneLimit lanes the rest fold away.
  const lanes = stats.projects
    .map((key) => {
      const laneRuns = stats.list.filter((run) => run.key === key);
      return {
        key,
        basedir: laneRuns[0].basedir_id,
        runs: laneRuns,
        latest: Math.max(...laneRuns.map((run) => +run.end)),
      };
    })
    .sort((a, b) => b.latest - a.latest);
  const shown = activityView.expanded
    ? lanes
    : lanes.slice(0, activityLaneLimit);
  const groups = new Map();
  lanes.forEach((lane) => {
    if (!groups.has(lane.basedir))
      groups.set(lane.basedir, { all: [], shown: [] });
    groups.get(lane.basedir).all.push(lane);
  });
  shown.forEach((lane) => groups.get(lane.basedir).shown.push(lane));
  let rows =
    '<div class="activity-row activity-axis"><div></div><div class="activity-track">' +
    tickHTML(true) +
    "</div></div>";
  groups.forEach((group, basedirID) => {
    if (!group.shown.length) return;
    const entry = basedirs.get(basedirID) || { id: basedirID, path: basedirID };
    const open = !activityView.collapsed.has(basedirID);
    const runCount = group.all.reduce((sum, lane) => sum + lane.runs.length, 0);
    rows +=
      '<button type="button" class="activity-basedir" data-basedir="' +
      esc(basedirID) +
      '" aria-expanded="' +
      open +
      '" title="' +
      esc(entry.path) +
      '"><span class="activity-caret">▾</span><span>' +
      basedirLabelHTML(entry) +
      "</span>" +
      '<span class="meta">' +
      group.all.length +
      (group.all.length === 1 ? " project · " : " projects · ") +
      runCount +
      (runCount === 1 ? " run" : " runs") +
      "</span></button>";
    if (!open) return;
    group.shown.forEach((lane) => {
      const bars = lane.runs
        .map((run) => {
          const left = percent(run.start);
          const right = percent(run.end);
          const cut =
            (run.start < start ? " cut-start" : "") +
            (run.end > end ? " cut-end" : "");
          return (
            '<a class="activity-bar tone-' +
            run.tone +
            (run.lifecycle === "running" ? " running" : "") +
            cut +
            '" href="' +
            esc(activityRunURL(run)) +
            '" data-run="' +
            esc(run.basedir_id + "/" + run.project_name + "/" + run.run_id) +
            '" style="left:' +
            left +
            "%;width:" +
            Math.max(right - left, 0.4) +
            '%" aria-label="' +
            esc(
              (run.run_name || run.run_id) +
                ", " +
                run.lifecycle +
                ", " +
                activityClock(run.start) +
                " to " +
                (run.lifecycle === "running" ? "now" : activityClock(run.end)),
            ) +
            '">' +
            (run.started_by?.actor
              ? '<span class="activity-actor-mark" title="' +
                esc(run.started_by.actor) +
                '">◆</span>'
              : "") +
            esc(run.run_name || run.run_id) +
            "</a>"
          );
        })
        .join("");
      rows +=
        '<div class="activity-row activity-lane"><div class="activity-lane-label" title="' +
        esc(entry.path + " / " + activityProjectName(lane.key)) +
        '"><span class="activity-chip" style="background:' +
        activityColor(colors, lane.key) +
        '"></span>' +
        esc(activityProjectName(lane.key)) +
        '</div><div class="activity-track">' +
        tickHTML(false) +
        bars +
        "</div></div>";
    });
  });
  const hidden = lanes.length - shown.length;
  const more =
    hidden > 0
      ? '<button type="button" class="activity-more">+' +
        hidden +
        (hidden === 1 ? " more project" : " more projects") +
        "</button>"
      : activityView.expanded && lanes.length > activityLaneLimit
        ? '<button type="button" class="activity-more">Show the ' +
          activityLaneLimit +
          " most recent only</button>"
        : "";
  container.innerHTML =
    notice +
    head +
    '<div class="activity-key meta"><span><i class="tone-ok"></i>finished</span><span><i class="tone-bad"></i>failed</span><span><i class="tone-warn"></i>interrupted</span><span><i class="tone-run"></i>running</span><span>◆ has an actor label</span><span>‹ › continues from or into another day</span></div>' +
    '<div class="activity-timeline-scroll"><div class="activity-timeline">' +
    rows +
    "</div></div>" +
    more;
  activityBindDay(container, day);
  container.querySelectorAll(".activity-basedir").forEach(
    (button) =>
      (button.onclick = () => {
        const id = button.dataset.basedir;
        if (activityView.collapsed.has(id)) activityView.collapsed.delete(id);
        else activityView.collapsed.add(id);
        activityDraw();
      }),
  );
  const moreButton = container.querySelector(".activity-more");
  if (moreButton)
    moreButton.onclick = () => {
      activityView.expanded = !activityView.expanded;
      activityDraw();
    };
}
function activityBindDay(container, day) {
  container.querySelector(".activity-prev-day").onclick = () =>
    activitySelectDay(activityAddDays(day, -1));
  const next = container.querySelector(".activity-next-day");
  if (next) next.onclick = () => activitySelectDay(activityAddDays(day, 1));
  container.querySelector(".activity-zoom-full").onclick = () => {
    activityView.busyHours = false;
    activityDraw();
  };
  container.querySelector(".activity-zoom-busy").onclick = () => {
    activityView.busyHours = true;
    activityDraw();
  };
}

// activityDrawProjects answers "where was I working": each project with runs
// in the loaded weeks up to the selected day, its last run, working
// directories, and latest source revision.
function activityDrawProjects(page, runs, colors) {
  const until = activityAddDays(activityView.day, 1);
  const basedirs = activityBasedirs();
  const byKey = new Map();
  runs
    .filter((run) => run.start < until)
    .forEach((run) => {
      if (!byKey.has(run.key)) byKey.set(run.key, []);
      byKey.get(run.key).push(run);
    });
  const rows = [...byKey.entries()]
    .map(([key, list]) => ({
      key,
      list,
      last: list.reduce((a, b) => (b.start > a.start ? b : a)),
    }))
    .sort((a, b) => b.last.start - a.last.start)
    .map(({ key, list, last }) => {
      const entry = basedirs.get(last.basedir_id) || {
        id: last.basedir_id,
        path: last.basedir_id,
      };
      const directories = [
        ...new Set(list.map((run) => run.cwd).filter(Boolean)),
      ];
      return (
        '<tr data-day="' +
        +activityDayStart(last.start) +
        '"><td><div class="activity-project-name"><span class="activity-chip" style="background:' +
        activityColor(colors, key) +
        '"></span><strong>' +
        esc(activityProjectName(key)) +
        '</strong></div><div class="meta">' +
        basedirLabelHTML(entry) +
        '</div></td><td class="activity-last-run">' +
        statusPill(last.lifecycle) +
        ' <a href="' +
        esc(activityRunURL(last)) +
        '">' +
        esc(activityDate(last.start) + " " + activityClock(last.start)) +
        '</a><div class="meta">' +
        esc(last.run_name || last.run_id) +
        '</div></td><td class="activity-mono">' +
        directories.map(esc).join("<br>") +
        '</td><td class="activity-mono">' +
        esc((last.sources || [])[0] || "-") +
        '</td><td class="activity-runs">' +
        list.length +
        "</td></tr>"
      );
    })
    .join("");
  const grid = activityGrid(activityView.month);
  page.querySelector(".activity-projects").innerHTML =
    '<div class="table-bar"><h2>Where you worked</h2><span class="meta">' +
    esc(activityDate(grid.start)) +
    " – " +
    esc(activityDate(activityView.day)) +
    '</span></div><div class="activity-table-scroll"><table class="runs activity-project-table"><thead><tr><th>Project</th><th>Last run</th><th>Working directories</th><th>Source</th><th>Runs</th></tr></thead><tbody>' +
    (rows ||
      '<tr><td colspan="5" class="meta">No runs in these weeks.</td></tr>') +
    "</tbody></table></div>";
  page.querySelectorAll(".activity-project-table tr[data-day]").forEach(
    (row) =>
      (row.onclick = (event) => {
        if (event.target.closest("a")) return;
        activitySelectDay(new Date(Number(row.dataset.day)));
      }),
  );
}

// Hovering a run bar shows where and how it ran, and its first note.
function activityFollowPointer(page) {
  const card = page.querySelector(".activity-card");
  const find = (id) =>
    activityRuns(activityView.data || {}).find(
      (run) =>
        run.basedir_id + "/" + run.project_name + "/" + run.run_id === id,
    );
  page.addEventListener("pointerover", (event) => {
    const bar = event.target.closest && event.target.closest(".activity-bar");
    if (!bar) return;
    const run = find(bar.dataset.run);
    if (!run) return;
    const entry = activityBasedirs().get(run.basedir_id);
    card.innerHTML =
      "<h3>" +
      statusPill(run.lifecycle) +
      " " +
      esc(run.run_name || run.run_id) +
      '</h3><div class="meta">' +
      (entry ? basedirLabelHTML(entry) + " / " : "") +
      esc(run.project_name) +
      "</div><dl><dt>Time</dt><dd>" +
      esc(
        activityDate(run.start) +
          " " +
          activityClock(run.start) +
          " – " +
          (run.lifecycle === "running"
            ? "now"
            : (activitySameDay(run.start, run.end)
                ? ""
                : activityDate(run.end) + " ") + activityClock(run.end)) +
          " (" +
          formatSpan(run.end - run.start) +
          ")",
      ) +
      "</dd>" +
      (run.cwd
        ? '<dt>Directory</dt><dd class="activity-mono">' +
          esc(run.cwd) +
          "</dd>"
        : "") +
      (run.host ? "<dt>Host</dt><dd>" + esc(run.host) + "</dd>" : "") +
      "<dt>Started by</dt><dd>" +
      esc(run.started_by ? startedByLabel(run.started_by) : "not recorded") +
      "</dd>" +
      (run.sources && run.sources.length
        ? '<dt>Source</dt><dd class="activity-mono">' +
          run.sources.map(esc).join("<br>") +
          "</dd>"
        : "") +
      "</dl>" +
      (run.first_note
        ? '<div class="activity-card-note">' +
          esc(run.first_note) +
          (run.note_count > 1 ? " (+" + (run.note_count - 1) + ")" : "") +
          "</div>"
        : "");
    card.hidden = false;
    activityPlaceCard(card, event.clientX, event.clientY);
  });
  page.addEventListener("pointermove", (event) => {
    if (!card.hidden) activityPlaceCard(card, event.clientX, event.clientY);
  });
  page.addEventListener("pointerout", (event) => {
    const bar = event.target.closest && event.target.closest(".activity-bar");
    if (bar && !(event.relatedTarget && bar.contains(event.relatedTarget)))
      card.hidden = true;
  });
}
function activityPlaceCard(card, x, y) {
  const box = card.getBoundingClientRect();
  let left = x + 14;
  let top = y + 14;
  if (left + box.width > window.innerWidth - 8)
    left = Math.max(8, x - box.width - 14);
  if (top + box.height > window.innerHeight - 8)
    top = Math.max(8, y - box.height - 14);
  card.style.left = left + "px";
  card.style.top = top + "px";
}
