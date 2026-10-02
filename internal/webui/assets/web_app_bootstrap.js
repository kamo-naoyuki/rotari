const originalEnhancePage = enhancePage;
enhancePage = function () {
  originalEnhancePage();
  addRunStatistics();
  addRunEnvironment();
  addLoadTimeline();
  renderJobTimelineScratch();
  const load = document.querySelector(".run-environment");
  const timeline = document.querySelector(".job-timeline");
  if (load && timeline) load.before(timeline);
  spaceGraphicLegends();
  simplifyRunStatistics();
  fixTimelineBarWidths();
  syncTimelineBar();
  addOutputWordCloud();
  collapseRunGraphics();
  alignTimelineHeading();
  alignGraphicHeadings();
};
function esc(v) {
  return String(v == null ? "" : v).replace(
    /[&<>"']/g,
    (c) =>
      ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" })[
        c
      ],
  );
}
function initOrbitGame() {
  const dialog = document.getElementById("orbit-game");
  const player = document.getElementById("orbit-game-player");
  const board = dialog?.querySelector(".orbit-game-board");
  if (!dialog || !player || !board) return;

  let angle = -Math.PI / 2;
  let returnFocus = null;
  let dragging = false;
  const moveTo = (nextAngle) => {
    angle = nextAngle;
    const degrees = ((Math.round((angle * 180) / Math.PI) % 360) + 360) % 360;
    player.setAttribute("cx", (84 * Math.cos(angle)).toFixed(2));
    player.setAttribute("cy", (84 * Math.sin(angle)).toFixed(2));
    player.setAttribute("aria-valuenow", String(degrees));
  };
  const open = () => {
    returnFocus = document.activeElement;
    dialog.hidden = false;
    player.focus();
  };
  const close = () => {
    dialog.hidden = true;
    dragging = false;
    if (returnFocus && typeof returnFocus.focus === "function")
      returnFocus.focus();
  };
  const nudge = (direction) => moveTo(angle + direction * (Math.PI / 18));
  const pointerAngle = (event) => {
    const bounds = board.getBoundingClientRect();
    if (!bounds.width || !bounds.height) return;
    const x = ((event.clientX - bounds.left) * 240) / bounds.width - 120;
    const y = ((event.clientY - bounds.top) * 240) / bounds.height - 120;
    moveTo(Math.atan2(y, x) - Math.PI / 4);
  };

  for (const link of document.querySelectorAll(
    ".sidebar-brand, .header-home",
  )) {
    link.addEventListener("click", (event) => {
      if (!event.shiftKey || event.button !== 0) return;
      event.preventDefault();
      open();
    });
  }
  document.getElementById("orbit-game-close").addEventListener("click", close);
  document
    .getElementById("orbit-game-left")
    .addEventListener("click", () => nudge(-1));
  document
    .getElementById("orbit-game-right")
    .addEventListener("click", () => nudge(1));
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) close();
  });
  dialog.addEventListener("keydown", (event) => {
    if (event.key === "Tab") {
      const focusable = [
        ...dialog.querySelectorAll('button:not([disabled]), [tabindex="0"]'),
      ];
      const first = focusable[0];
      const last = focusable.at(-1);
      if (event.shiftKey && document.activeElement === first) {
        event.preventDefault();
        last.focus();
      } else if (!event.shiftKey && document.activeElement === last) {
        event.preventDefault();
        first.focus();
      }
    } else if (event.key === "Escape") {
      event.preventDefault();
      close();
    } else if (event.target === player && event.key === "ArrowLeft") {
      event.preventDefault();
      nudge(-1);
    } else if (event.target === player && event.key === "ArrowRight") {
      event.preventDefault();
      nudge(1);
    }
  });
  board.addEventListener("pointerdown", (event) => {
    if (event.target.closest("button")) return;
    dragging = true;
    pointerAngle(event);
    if (board.setPointerCapture) board.setPointerCapture(event.pointerId);
  });
  board.addEventListener("pointermove", (event) => {
    if (dragging) pointerAngle(event);
  });
  for (const eventName of [
    "pointerup",
    "pointercancel",
    "lostpointercapture",
  ]) {
    board.addEventListener(eventName, () => {
      dragging = false;
    });
  }
}
const originalRender = render;
render = function () {
  if (pageParts()[0] !== "search")
    document.getElementById("page-title").closest("section").hidden = false;
  const runtimeDetails = document.querySelector(".project-runtime details");
  if (runtimeDetails) projectRuntimeDetailsOpen = runtimeDetails.open;
  originalRender();
  const parts = pageParts();
  if (parts[0] === "search") return;
  enhancePage();
  enhanceQueueOverview();
  addQueueOverviewPathActions();
  if (parts[0] === "project" && !parts[2]) {
    const queue = state.projects.find(
      (q) => q.project_name === decodeURIComponent(parts[1]),
    );
    if (queue) {
      const commands = queue.queue.commands || [];
      ensureQueueWorkingDirectoryColumn(commands);
      addQueueEditors(queue, commands);
      enhanceQueueSourceContext(commands);
    }
  }
  addProjectRuntime();
  addRunHostLine();
  addExecutionGuide();
  addDeleteRunButton();
  addPathTableActions();
  removeLegacyOutputBox();
  keepGlobalOutputBox();
  placeOutputBox();
  renameCopyButtons();
  labelEquivalentCommand();
  addRunJobStatusColumn();
  addRunHostsColumn();
  addRunningOutputButtons();
  addRunJobSelection();
  arrangeRunControls();
  mergeActionColumns();
  labelJobActionHeaders();
  styleActionColumns();
  markJobHeaders();
  markLatestRun();
  enableTableSorting();
  restoreSelectedOutput();
  applyStatusColors();
  fixRunStatisticsColors();
  fixTimelineLegendColors();
  addConfigButton();
  openRequestedConfigAction();
  addAIButtons();
  arrangeRunControls();
  orderJobActions();
  clampLongTableCells();
  addMatrixPanels();
  applyBasedirLinks();
  focusHistorySearchJob();
};
initOrbitGame();
window.addEventListener("popstate", () => refresh(true));
initSidebarResizer();
refresh(true);
setInterval(() => refresh(false), 2000);
