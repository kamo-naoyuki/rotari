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
  const whiteBalls = document.getElementById("orbit-game-white-balls");
  const gameOver = document.getElementById("orbit-game-over");
  if (!dialog || !player || !board || !whiteBalls || !gameOver) return;

  const ringRadius = 84;
  const redRadius = 13;
  const ballCount = 6;
  const whiteRadiusMin = 7;
  const whiteRadiusMax = 17;
  const gravityStrength = 4;
  const redControlStrength = 4;
  const whiteControlStrength = 1.7;
  let returnFocus = null;
  let turnDirection = 0;
  let animationFrame = 0;
  let lastFrameTime = null;
  let running = false;
  let redAngle = -Math.PI / 2 - 0.18;
  let redAngularVelocity = 0;
  let whiteStates = [];

  const angleDelta = (from, to) => {
    let difference = Math.abs(from - to) % (Math.PI * 2);
    if (difference > Math.PI) difference = Math.PI * 2 - difference;
    return difference;
  };
  const drawBall = (element, angle, angularVelocity) => {
    element.setAttribute("cx", (ringRadius * Math.cos(angle)).toFixed(2));
    element.setAttribute("cy", (ringRadius * Math.sin(angle)).toFixed(2));
    element.dataset.angle = String(angle);
    element.dataset.angularVelocity = String(angularVelocity);
  };
  // Gravity accelerates each ball down the ring; inertia carries it through the bottom.
  const gravityAcceleration = (radius, angle) =>
    (gravityStrength / radius) * Math.cos(angle);
  const makeWhiteBalls = () => {
    whiteBalls.replaceChildren();
    whiteStates = Array.from({ length: ballCount }, (_, index) => {
      const radius =
        whiteRadiusMin + Math.random() * (whiteRadiusMax - whiteRadiusMin);
      const angle =
        -Math.PI / 2 + ((index + 1) * Math.PI * 2) / (ballCount + 1);
      const element = document.createElementNS(
        "http://www.w3.org/2000/svg",
        "circle",
      );
      element.setAttribute("class", "orbit-game-node");
      element.setAttribute("r", radius.toFixed(2));
      whiteBalls.append(element);
      const white = { angle, angularVelocity: 0, radius, element };
      drawBall(element, angle, white.angularVelocity);
      return white;
    });
    redAngle = -Math.PI / 2 - 0.18;
    redAngularVelocity = 0;
    drawBall(player, redAngle, redAngularVelocity);
    gameOver.hidden = true;
  };
  const hasCollision = () =>
    whiteStates.some(
      (white) =>
        angleDelta(redAngle, white.angle) <=
        (redRadius + white.radius) / ringRadius,
    );
  const stop = () => {
    running = false;
    turnDirection = 0;
    lastFrameTime = null;
    if (animationFrame) cancelAnimationFrame(animationFrame);
    animationFrame = 0;
  };
  const endGame = () => {
    stop();
    gameOver.hidden = false;
    document.getElementById("orbit-game-restart").focus();
  };
  const tick = (timestamp) => {
    if (!running || dialog.hidden) return;
    const elapsed =
      lastFrameTime === null
        ? 0
        : Math.min((timestamp - lastFrameTime) / 1000, 0.05);
    lastFrameTime = timestamp;
    redAngularVelocity +=
      (gravityAcceleration(redRadius, redAngle) +
        turnDirection * redControlStrength) *
      elapsed;
    redAngle += redAngularVelocity * elapsed;
    drawBall(player, redAngle, redAngularVelocity);
    for (const white of whiteStates) {
      const sizeFactor = 0.45 + (whiteRadiusMax - white.radius) * 0.065;
      white.angularVelocity +=
        (gravityAcceleration(white.radius, white.angle) +
          turnDirection * whiteControlStrength * sizeFactor) *
        elapsed;
      white.angle += white.angularVelocity * elapsed;
      drawBall(white.element, white.angle, white.angularVelocity);
    }
    if (hasCollision()) {
      endGame();
      return;
    }
    animationFrame = requestAnimationFrame(tick);
  };
  const start = () => {
    stop();
    makeWhiteBalls();
    running = true;
    animationFrame = requestAnimationFrame(tick);
  };
  const open = () => {
    returnFocus = document.activeElement;
    dialog.hidden = false;
    start();
    document.getElementById("orbit-game-left").focus();
  };
  const close = () => {
    stop();
    dialog.hidden = true;
    if (returnFocus && typeof returnFocus.focus === "function")
      returnFocus.focus();
  };
  const setTurnDirection = (direction) => {
    if (running) turnDirection = direction;
  };
  const nudge = (direction) => {
    if (!running) return;
    redAngularVelocity += direction * 0.12 * redControlStrength;
    for (const white of whiteStates) {
      white.angularVelocity +=
        direction *
        0.12 *
        whiteControlStrength *
        (0.45 + (whiteRadiusMax - white.radius) * 0.065);
      drawBall(white.element, white.angle, white.angularVelocity);
    }
    drawBall(player, redAngle, redAngularVelocity);
    if (hasCollision()) endGame();
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
  for (const button of dialog.querySelectorAll("[data-orbit-direction]")) {
    const direction = Number(button.dataset.orbitDirection);
    button.addEventListener("pointerdown", (event) => {
      setTurnDirection(direction);
      if (button.setPointerCapture) button.setPointerCapture(event.pointerId);
    });
    for (const eventName of [
      "pointerup",
      "pointercancel",
      "lostpointercapture",
    ]) {
      button.addEventListener(eventName, () => setTurnDirection(0));
    }
    button.addEventListener("keydown", (event) => {
      if (event.key === "Enter" || event.key === " ")
        setTurnDirection(direction);
    });
    button.addEventListener("keyup", (event) => {
      if (event.key === "Enter" || event.key === " ") setTurnDirection(0);
    });
    button.addEventListener("click", () => nudge(direction));
  }
  document
    .getElementById("orbit-game-restart")
    .addEventListener("click", start);
  dialog.addEventListener("click", (event) => {
    if (event.target === dialog) close();
  });
  dialog.addEventListener("keydown", (event) => {
    if (event.key === "Tab") {
      const focusable = [
        ...dialog.querySelectorAll(
          'button:not([disabled]):not([hidden]), [tabindex="0"]',
        ),
      ].filter((element) => !element.closest("[hidden]"));
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
    } else if (event.key === "ArrowLeft") {
      event.preventDefault();
      setTurnDirection(-1);
    } else if (event.key === "ArrowRight") {
      event.preventDefault();
      setTurnDirection(1);
    }
  });
  dialog.addEventListener("keyup", (event) => {
    if (event.key === "ArrowLeft" || event.key === "ArrowRight")
      setTurnDirection(0);
  });
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
