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
const originalRender = render;
render = function () {
  const runtimeDetails = document.querySelector(".project-runtime details");
  if (runtimeDetails) projectRuntimeDetailsOpen = runtimeDetails.open;
  originalRender();
  enhancePage();
  enhanceQueueOverview();
  addQueueOverviewPathActions();
  const parts = pageParts();
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
  addRunningCancelButtons();
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
  addAIButtons();
  arrangeRunControls();
  orderJobActions();
  restoreMatrixActions();
};
window.addEventListener("popstate", render);
refresh();
setInterval(refresh, 2000);
