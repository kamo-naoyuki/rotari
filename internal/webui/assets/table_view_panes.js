// The Table viewer's panes: files a table names, opened in tabs, in panes
// split right or down. One pane is active, and files clicked in the table
// open there. Images and video show as they are; audio has a player, its
// waveform, and its spectrogram, which the browser computes from the
// decoded samples for the time range in view.
"use strict";

// panes is the layout: a tree of splits whose leaves are panes, or null when
// nothing is open.
const panes = { layout: null, active: null, nextID: 1 };
// tabViews keeps each open tab's elements, so re-laying out the panes moves
// them without reloading or stopping a player.
const tabViews = new Map();

// Bounds on what the browser decodes and analyzes for one file.
const maxDecodeBytes = 512 * 1024 * 1024;
const maxAnalysisSeconds = 600;
// The spectrogram's range below its loudest point, in decibels.
const spectrogramRange = 80;

function newPane(tabs) {
  return {
    type: "pane",
    id: "p" + panes.nextID++,
    tabs: tabs || [],
    current: tabs && tabs.length ? tabs[0].id : null,
    wave: true,
    spec: true,
    // range is the time range in view, in seconds, or null for the whole.
    range: null,
  };
}

function newTab(media) {
  return { id: "t" + panes.nextID++, media };
}

// eachPane visits the panes of a layout in order.
function eachPane(node, visit) {
  if (!node) return;
  if (node.type === "pane") {
    visit(node);
    return;
  }
  node.children.forEach((child) => eachPane(child, visit));
}

function allPanes() {
  const found = [];
  eachPane(panes.layout, (pane) => found.push(pane));
  return found;
}

function findPane(id) {
  return allPanes().find((pane) => pane.id === id) || null;
}

// parentOf returns the split holding a node, or null for the root.
function parentOf(target, node = panes.layout) {
  if (!node || node.type === "pane") return null;
  for (const child of node.children) {
    if (child === target) return node;
    const found = parentOf(target, child);
    if (found) return found;
  }
  return null;
}

function activePane() {
  return findPane(panes.active) || allPanes()[0] || null;
}

function sameMedia(a, b) {
  return a.ref === b.ref && (a.base || "") === (b.base || "");
}

// openMedia opens a file in a tab of the active pane, or shows its tab
// there if it is already open.
function openMedia(media) {
  if (!panes.layout) {
    panes.layout = newPane();
    panes.active = panes.layout.id;
  }
  const pane = activePane();
  let tab = pane.tabs.find((tab) => sameMedia(tab.media, media));
  if (!tab) {
    tab = newTab(media);
    pane.tabs.push(tab);
  }
  selectTab(pane, tab.id);
}

// openMediaRow lays out files side by side, one pane each, replacing what
// was open: a stems view's row, to compare its groups.
function openMediaRow(list) {
  if (!list.length) return;
  tabViews.forEach((tabView) => tabView.dispose());
  tabViews.clear();
  const row = list.map((media) => newPane([newTab(media)]));
  panes.layout =
    row.length === 1
      ? row[0]
      : {
          type: "split",
          dir: "row",
          children: row,
          sizes: row.map(() => 1),
        };
  panes.active = row[0].id;
  if (typeof setLinked === "function") setLinked(true);
  renderPanes();
}

function selectTab(pane, tabID) {
  if (pane.current !== tabID) pane.range = null;
  pane.current = tabID;
  panes.active = pane.id;
  renderPanes();
}

function closeTab(pane, tabID) {
  pane.tabs = pane.tabs.filter((tab) => tab.id !== tabID);
  const view = tabViews.get(tabID);
  if (view) {
    view.dispose();
    tabViews.delete(tabID);
  }
  if (!pane.tabs.length) {
    closePane(pane);
    return;
  }
  if (pane.current === tabID) {
    pane.current = pane.tabs[pane.tabs.length - 1].id;
    pane.range = null;
  }
  renderPanes();
}

// closePane removes a pane and its tabs; the pane beside it becomes active
// when it was.
function closePane(pane) {
  pane.tabs.forEach((tab) => {
    const view = tabViews.get(tab.id);
    if (view) view.dispose();
    tabViews.delete(tab.id);
  });
  const order = allPanes();
  const index = order.indexOf(pane);
  removeNode(pane);
  if (panes.active === pane.id) {
    const rest = allPanes();
    const next = rest[Math.max(0, index - 1)] || null;
    panes.active = next ? next.id : null;
  }
  renderPanes();
}

// removeNode takes a node out of the layout, folding a split left with one
// child into that child.
function removeNode(node) {
  const parent = parentOf(node);
  if (!parent) {
    panes.layout = null;
    return;
  }
  const index = parent.children.indexOf(node);
  parent.children.splice(index, 1);
  parent.sizes.splice(index, 1);
  if (parent.children.length === 1) {
    const only = parent.children[0];
    const grand = parentOf(parent);
    if (!grand) {
      panes.layout = only;
    } else {
      grand.children[grand.children.indexOf(parent)] = only;
    }
  }
}

// insertBeside puts a new pane beside an existing one, to its right
// ("row") or below it ("column"), or before it when before is set.
function insertBeside(pane, created, dir, before) {
  const parent = parentOf(pane);
  if (parent && parent.dir === dir) {
    const index = parent.children.indexOf(pane) + (before ? 0 : 1);
    const share = parent.sizes[parent.children.indexOf(pane)] / 2;
    parent.sizes[parent.children.indexOf(pane)] = share;
    parent.children.splice(index, 0, created);
    parent.sizes.splice(index, 0, share);
    return;
  }
  const split = {
    type: "split",
    dir,
    children: before ? [created, pane] : [pane, created],
    sizes: [1, 1],
  };
  if (!parent) {
    panes.layout = split;
  } else {
    parent.children[parent.children.indexOf(pane)] = split;
  }
}

// splitPane opens a new pane beside one, with its current file, and makes
// it active, so the next file clicked opens beside the first.
function splitPane(pane, dir) {
  const current = pane.tabs.find((tab) => tab.id === pane.current);
  const created = newPane(current ? [newTab(current.media)] : []);
  created.wave = pane.wave;
  created.spec = pane.spec;
  insertBeside(pane, created, dir, false);
  panes.active = created.id;
  renderPanes();
}

function iconButton(label, path, onclick) {
  const svg = document.createElementNS("http://www.w3.org/2000/svg", "svg");
  svg.setAttribute("viewBox", "0 0 16 16");
  svg.setAttribute("aria-hidden", "true");
  svg.innerHTML = path;
  return el(
    "button",
    {
      type: "button",
      className: "tv-icon",
      title: label,
      "aria-label": label,
      onclick,
    },
    svg,
  );
}

const splitRightIcon =
  '<rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M8 2.5v11"/>';
const splitDownIcon =
  '<rect x="1.5" y="2.5" width="13" height="11" rx="1.5"/><path d="M1.5 8h13"/>';
const closeIcon = '<path d="M4 4l8 8M12 4l-8 8"/>';

function renderPanes() {
  const area = document.getElementById("tv-panes");
  const handle = document.getElementById("tv-panes-handle");
  area.hidden = handle.hidden = !panes.layout;
  if (typeof renderPaneToolbar === "function") renderPaneToolbar();
  if (!panes.layout) {
    setChildren(document.getElementById("tv-pane-root"));
    return;
  }
  if (!findPane(panes.active)) panes.active = allPanes()[0].id;
  setChildren(
    document.getElementById("tv-pane-root"),
    renderNode(panes.layout),
  );
  tabViews.forEach((view) => view.redraw());
}

function renderNode(node) {
  if (node.type === "pane") return renderPane(node);
  const box = el("div", { className: "tv-split tv-split-" + node.dir });
  node.children.forEach((child, index) => {
    if (index) box.append(splitHandle(node, index));
    const rendered = renderNode(child);
    rendered.style.flex = node.sizes[index] + " 1 0";
    box.append(rendered);
  });
  return box;
}

// splitHandle resizes the two children of a split on either side of it.
function splitHandle(split, index) {
  const handle = el("div", {
    className: "tv-split-handle",
    role: "separator",
    "aria-orientation": split.dir === "row" ? "vertical" : "horizontal",
  });
  handle.addEventListener("pointerdown", (event) => {
    event.preventDefault();
    const before = handle.previousElementSibling;
    const after = handle.nextElementSibling;
    const horizontal = split.dir === "row";
    const total = split.sizes[index - 1] + split.sizes[index];
    const startSize = horizontal ? before.offsetWidth : before.offsetHeight;
    const pairSize =
      startSize + (horizontal ? after.offsetWidth : after.offsetHeight);
    const start = horizontal ? event.clientX : event.clientY;
    const move = (moved) => {
      const delta = (horizontal ? moved.clientX : moved.clientY) - start;
      const share = Math.min(
        0.9,
        Math.max(0.1, (startSize + delta) / pairSize),
      );
      split.sizes[index - 1] = total * share;
      split.sizes[index] = total * (1 - share);
      before.style.flex = split.sizes[index - 1] + " 1 0";
      after.style.flex = split.sizes[index] + " 1 0";
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      tabViews.forEach((view) => view.redraw());
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  });
  return handle;
}

function renderPane(pane) {
  const active = pane.id === panes.active;
  const tabs = pane.tabs.map((tab) =>
    el(
      "div",
      {
        className:
          "tv-tab" + (tab.id === pane.current ? " tv-tab-current" : ""),
        role: "tab",
        "aria-selected": String(tab.id === pane.current),
        "data-tab": tab.id,
        title: tab.media.ref,
      },
      el(
        "button",
        {
          type: "button",
          className: "tv-tab-name",
          onclick: () => selectTab(pane, tab.id),
        },
        el("span", { className: "tv-tab-kind" }, tab.media.kind),
        tab.media.ref.slice(tab.media.ref.lastIndexOf("/") + 1),
      ),
      el(
        "button",
        {
          type: "button",
          className: "tv-tab-close",
          "aria-label": "Close tab",
          onclick: () => closeTab(pane, tab.id),
        },
        "×",
      ),
    ),
  );
  const strip = el(
    "div",
    { className: "tv-tabs", role: "tablist", "data-pane": pane.id },
    tabs,
    el("span", { className: "tv-spacer" }),
    active
      ? el("span", { className: "tv-active-label" }, "Files open here")
      : null,
    iconButton("Split right", splitRightIcon, () => splitPane(pane, "row")),
    iconButton("Split down", splitDownIcon, () => splitPane(pane, "column")),
    iconButton("Close pane", closeIcon, () => closePane(pane)),
  );
  const current = pane.tabs.find((tab) => tab.id === pane.current);
  const body = el("div", { className: "tv-pane-body" });
  if (current) {
    let view = tabViews.get(current.id);
    if (!view) {
      view = new TabView(current.media);
      tabViews.set(current.id, view);
    }
    view.pane = pane;
    body.append(view.root);
    view.update();
  }
  const section = el(
    "section",
    {
      className: "tv-pane" + (active ? " tv-pane-active" : ""),
      "aria-label": active ? "Active pane" : "Pane",
      "data-pane": pane.id,
    },
    strip,
    body,
  );
  // Any click in a pane makes it the one files open in.
  section.addEventListener("pointerdown", () => {
    if (panes.active === pane.id) return;
    panes.active = pane.id;
    document.querySelectorAll(".tv-pane").forEach((other) => {
      const now = other.dataset.pane === pane.id;
      other.classList.toggle("tv-pane-active", now);
      other.setAttribute("aria-label", now ? "Active pane" : "Pane");
    });
    document
      .querySelectorAll(".tv-active-label")
      .forEach((label) => label.remove());
    strip.insertBefore(
      el("span", { className: "tv-active-label" }, "Files open here"),
      strip.querySelector(".tv-icon"),
    );
    if (typeof renderPaneToolbar === "function") renderPaneToolbar();
  });
  return section;
}

// The radix-2 FFT of one window, reused across columns of one size.
const fftTables = new Map();

function fftTable(size) {
  let table = fftTables.get(size);
  if (table) return table;
  const window = new Float32Array(size);
  for (let index = 0; index < size; index++) {
    window[index] = 0.5 - 0.5 * Math.cos((2 * Math.PI * index) / (size - 1));
  }
  const reversed = new Uint32Array(size);
  const bits = Math.log2(size);
  for (let index = 0; index < size; index++) {
    let value = 0;
    for (let bit = 0; bit < bits; bit++)
      value |= ((index >> bit) & 1) << (bits - 1 - bit);
    reversed[index] = value;
  }
  table = {
    window,
    reversed,
    real: new Float32Array(size),
    imag: new Float32Array(size),
  };
  fftTables.set(size, table);
  return table;
}

// magnitudes writes the windowed spectrum of samples from start into out,
// one value per bin up to half the size.
function magnitudes(samples, start, size, out) {
  const { window, reversed, real, imag } = fftTable(size);
  for (let index = 0; index < size; index++) {
    const sample = samples[start + index];
    real[reversed[index]] = sample === undefined ? 0 : sample * window[index];
    imag[reversed[index]] = 0;
  }
  for (let length = 2; length <= size; length <<= 1) {
    const half = length >> 1;
    const angle = (-2 * Math.PI) / length;
    for (let offset = 0; offset < size; offset += length) {
      for (let index = 0; index < half; index++) {
        const cos = Math.cos(angle * index);
        const sin = Math.sin(angle * index);
        const a = offset + index;
        const b = a + half;
        const tr = real[b] * cos - imag[b] * sin;
        const ti = real[b] * sin + imag[b] * cos;
        real[b] = real[a] - tr;
        imag[b] = imag[a] - ti;
        real[a] += tr;
        imag[a] += ti;
      }
    }
  }
  for (let bin = 0; bin < size / 2; bin++) {
    out[bin] = Math.hypot(real[bin], imag[bin]);
  }
}

// viridis maps 0..1 to a perceptually even colour scale, from its stops.
const viridisStops = [
  [68, 1, 84],
  [59, 82, 139],
  [33, 145, 140],
  [94, 201, 98],
  [253, 231, 37],
];

// viridisTable holds 256 colours of the scale, three bytes each.
const viridisTable = (() => {
  const table = new Uint8Array(256 * 3);
  for (let step = 0; step < 256; step++) {
    const scaled = (step / 255) * (viridisStops.length - 1);
    const low = Math.floor(scaled);
    const high = Math.min(viridisStops.length - 1, low + 1);
    const mix = scaled - low;
    for (let channel = 0; channel < 3; channel++) {
      table[step * 3 + channel] = Math.round(
        viridisStops[low][channel] +
          (viridisStops[high][channel] - viridisStops[low][channel]) * mix,
      );
    }
  }
  return table;
})();

// decoded keeps decoded audio by URL, a few files at a time.
const decoded = new Map();

// decodeAudio fetches a file and decodes it in the browser into one channel
// of samples, at most maxAnalysisSeconds long.
function decodeAudio(url, size) {
  if (decoded.has(url)) return decoded.get(url);
  const pending = (async () => {
    if (size > maxDecodeBytes) {
      throw new Error(
        "too large to analyze in the browser (" + formatBytes(size) + ")",
      );
    }
    const Context = window.AudioContext || window.webkitAudioContext;
    if (!Context) throw new Error("this browser cannot decode audio");
    const response = await fetch(url);
    if (!response.ok)
      throw new Error(
        (await response.text()).trim() || "HTTP " + response.status,
      );
    const context = new Context();
    try {
      const buffer = await context.decodeAudioData(
        await response.arrayBuffer(),
      );
      const length = Math.min(
        buffer.length,
        Math.floor(maxAnalysisSeconds * buffer.sampleRate),
      );
      const samples = new Float32Array(length);
      for (let channel = 0; channel < buffer.numberOfChannels; channel++) {
        const data = buffer.getChannelData(channel);
        for (let index = 0; index < length; index++) {
          samples[index] += data[index] / buffer.numberOfChannels;
        }
      }
      return {
        samples,
        rate: buffer.sampleRate,
        duration: buffer.duration,
        truncated: length < buffer.length,
      };
    } finally {
      if (context.close) context.close();
    }
  })();
  decoded.set(url, pending);
  if (decoded.size > 8) decoded.delete(decoded.keys().next().value);
  pending.catch(() => decoded.delete(url));
  return pending;
}

// TabView is one open file's elements: its title line, the image, video, or
// audio player, and for audio the waveform and spectrogram.
class TabView {
  constructor(media) {
    this.media = media;
    this.url = mediaURL(media);
    this.pane = null;
    this.audioData = null;
    this.cache = {};
    this.selection = null;
    this.root = el("div", { className: "tv-tab-view" });
    const properties = media.properties
      ? formatProperties(media.properties)
      : "";
    const title = el(
      "div",
      { className: "tv-tab-title" },
      el("strong", {}, media.ref),
      el("span", { className: "tv-media-properties" }, properties),
      el(
        "a",
        { href: this.url, target: "_blank", rel: "noopener" },
        "Open in a new tab",
      ),
    );
    this.root.append(title);
    if (media.kind === "image") {
      this.root.append(
        el("img", { className: "tv-image", src: this.url, alt: media.ref }),
      );
      return;
    }
    if (media.kind === "video") {
      this.root.append(
        el("video", {
          className: "tv-video",
          src: this.url,
          controls: true,
          preload: "metadata",
        }),
      );
      return;
    }
    this.player = el("audio", {
      className: "tv-audio",
      src: this.url,
      controls: true,
      preload: "metadata",
    });
    this.waveButton = el(
      "button",
      { type: "button", onclick: () => this.toggle("wave") },
      "Waveform",
    );
    this.specButton = el(
      "button",
      { type: "button", onclick: () => this.toggle("spec") },
      "Spectrogram",
    );
    this.status = el("p", { className: "tv-analysis-status" });
    this.wave = this.plot("tv-waveform");
    this.spec = this.plot("tv-spectrogram");
    this.axis = el("div", { className: "tv-time-axis" });
    this.root.append(
      el(
        "div",
        { className: "tv-audio-controls" },
        this.player,
        this.waveButton,
        this.specButton,
      ),
      this.status,
      this.wave.box,
      this.spec.box,
      this.axis,
    );
    ["play", "pause", "seeked", "timeupdate", "loadedmetadata"].forEach(
      (name) =>
        this.player.addEventListener(name, () => this.playbackChanged(name)),
    );
    this.resizer =
      typeof ResizeObserver === "function"
        ? new ResizeObserver(() => this.redraw())
        : null;
    if (this.resizer) {
      this.resizer.observe(this.wave.canvas);
      this.resizer.observe(this.spec.canvas);
    }
  }

  // plot makes a canvas for one view of the samples, with its pointer
  // handling: a click seeks, a drag zooms to its range, and the wheel
  // scrolls.
  plot(className) {
    const canvas = el("canvas", { className });
    const box = el(
      "div",
      { className: "tv-plot " + className + "-box" },
      canvas,
    );
    canvas.addEventListener("pointerdown", (event) =>
      this.pointerDown(event, canvas),
    );
    canvas.addEventListener("wheel", (event) => this.wheel(event), {
      passive: false,
    });
    return { canvas, box };
  }

  toggle(which) {
    if (!this.pane) return;
    this.pane[which] = !this.pane[which];
    this.update();
  }

  // update matches the elements to the pane's settings and draws.
  update() {
    if (!this.player) return;
    const pane = this.pane;
    this.waveButton.setAttribute("aria-pressed", String(pane.wave));
    this.specButton.setAttribute("aria-pressed", String(pane.spec));
    this.wave.box.hidden = !pane.wave;
    this.spec.box.hidden = !pane.spec;
    this.axis.hidden = !pane.wave && !pane.spec;
    if ((pane.wave || pane.spec) && !this.audioData && !this.loading)
      this.analyze();
    this.redraw();
  }

  async analyze() {
    this.loading = true;
    this.status.textContent = "Decoding…";
    try {
      this.audioData = await decodeAudio(
        this.url,
        this.media.properties ? this.media.properties.size : 0,
      );
      this.status.textContent = this.audioData.truncated
        ? "Only the first " + maxAnalysisSeconds / 60 + " minutes are drawn."
        : "";
    } catch (error) {
      this.status.textContent = "Cannot draw the waveform: " + error.message;
    }
    this.loading = false;
    this.redraw();
  }

  duration() {
    if (this.audioData)
      return this.audioData.samples.length / this.audioData.rate;
    return this.player && Number.isFinite(this.player.duration)
      ? this.player.duration
      : 0;
  }

  // range is the time range in view: the linked one, the pane's, or all.
  range() {
    if (typeof linkedRange === "function") {
      const linked = linkedRange();
      if (linked) return linked;
    }
    return this.pane && this.pane.range
      ? this.pane.range
      : { start: 0, end: this.duration() || 1 };
  }

  setRange(range) {
    if (typeof setLinkedRange === "function" && setLinkedRange(range)) return;
    this.pane.range = range;
    this.redraw();
    if (typeof renderPaneToolbar === "function") renderPaneToolbar();
  }

  // cursor is the time the playback line marks.
  cursor() {
    if (typeof linkedTime === "function") {
      const linked = linkedTime(this);
      if (linked !== null) return linked;
    }
    return this.player ? this.player.currentTime : 0;
  }

  playbackChanged(name) {
    if (typeof playbackMoved === "function") playbackMoved(this, name);
    if (name === "play") this.follow();
    this.drawOverlays();
  }

  // follow redraws the playback line while the player plays, and pages the
  // view along when it runs past the end.
  follow() {
    const step = () => {
      if (!this.player || this.player.paused) return;
      const time = this.player.currentTime;
      const range = this.range();
      if (time > range.end && range.end < this.duration()) {
        const width = range.end - range.start;
        this.setRange({ start: time, end: time + width });
      }
      if (typeof playbackMoved === "function")
        playbackMoved(this, "timeupdate");
      this.drawOverlays();
      requestAnimationFrame(step);
    };
    requestAnimationFrame(step);
  }

  timeAt(event, canvas) {
    const box = canvas.getBoundingClientRect();
    const range = this.range();
    const share = box.width ? (event.clientX - box.left) / box.width : 0;
    return (
      range.start + Math.min(1, Math.max(0, share)) * (range.end - range.start)
    );
  }

  pointerDown(event, canvas) {
    const start = this.timeAt(event, canvas);
    const startX = event.clientX;
    const move = (moved) => {
      this.selection = [start, this.timeAt(moved, canvas)];
      this.drawOverlays();
    };
    const up = (released) => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      this.selection = null;
      const end = this.timeAt(released, canvas);
      if (Math.abs(released.clientX - startX) < 4) {
        this.seek(end);
      } else {
        this.setRange({
          start: Math.min(start, end),
          end: Math.max(start, end),
        });
      }
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  }

  seek(time) {
    if (this.player) this.player.currentTime = time;
    if (typeof playbackMoved === "function")
      playbackMoved(this, "seeked", time);
    this.drawOverlays();
  }

  wheel(event) {
    event.preventDefault();
    const range = this.range();
    const width = range.end - range.start;
    const delta = Math.sign(event.deltaY || event.deltaX) * width * 0.1;
    const start = Math.max(
      0,
      Math.min(this.duration() - width, range.start + delta),
    );
    this.setRange({ start, end: start + width });
  }

  // zoom scales the range about its middle; Fit shows the whole file.
  zoom(factor) {
    const range = this.range();
    const middle = (range.start + range.end) / 2;
    const width = Math.min(
      this.duration(),
      Math.max(0.01, (range.end - range.start) * factor),
    );
    const start = Math.max(
      0,
      Math.min(this.duration() - width, middle - width / 2),
    );
    this.setRange({ start, end: start + width });
  }

  redraw() {
    if (!this.player || !this.root.isConnected) return;
    this.cache = {};
    this.drawAxis();
    this.drawOverlays();
  }

  // size matches a canvas's pixels to its box; 0 when it is not shown.
  size(canvas) {
    const ratio = window.devicePixelRatio || 1;
    const width = Math.round(canvas.clientWidth * ratio);
    const height = Math.round(canvas.clientHeight * ratio);
    if (canvas.width !== width) canvas.width = width;
    if (canvas.height !== height) canvas.height = height;
    return { width, height };
  }

  drawAxis() {
    const range = this.range();
    const ticks = [];
    for (let index = 0; index <= 4; index++) {
      const time = range.start + ((range.end - range.start) * index) / 4;
      ticks.push(
        el(
          "span",
          {},
          time.toFixed(range.end - range.start < 2 ? 2 : 1) + " s",
        ),
      );
    }
    setChildren(this.axis, ticks);
  }

  // drawOverlays draws both plots from their cached pixels, with the
  // playback line and any selection over them.
  drawOverlays() {
    if (!this.player || !this.audioData || !this.root.isConnected) return;
    if (this.pane.wave) this.drawWave();
    if (this.pane.spec) this.drawSpectrogram();
  }

  // overlay draws the playback line and a selection on a plot.
  overlay(context, width, height, color) {
    const range = this.range();
    const span = range.end - range.start || 1;
    const x = ((this.cursor() - range.start) / span) * width;
    if (this.selection) {
      const [a, b] = this.selection;
      context.fillStyle = "rgba(31, 99, 196, 0.25)";
      const left = ((Math.min(a, b) - range.start) / span) * width;
      context.fillRect(left, 0, (Math.abs(b - a) / span) * width, height);
    }
    if (x >= 0 && x <= width) {
      context.fillStyle = color;
      context.fillRect(Math.round(x) - 1, 0, 2, height);
    }
  }

  drawWave() {
    const canvas = this.wave.canvas;
    const context = canvas.getContext && canvas.getContext("2d");
    if (!context) return;
    const { width, height } = this.size(canvas);
    if (!width || !height) return;
    const { samples, rate } = this.audioData;
    const range = this.range();
    if (!this.cache.wave || this.cache.wave.width !== width) {
      const low = new Float32Array(width);
      const high = new Float32Array(width);
      const perPixel = ((range.end - range.start) * rate) / width;
      for (let x = 0; x < width; x++) {
        const from = Math.floor(range.start * rate + x * perPixel);
        const to = Math.max(
          from + 1,
          Math.floor(range.start * rate + (x + 1) * perPixel),
        );
        let min = 0;
        let max = 0;
        for (let index = from; index < to && index < samples.length; index++) {
          const sample = samples[index];
          if (sample < min) min = sample;
          if (sample > max) max = sample;
        }
        low[x] = min;
        high[x] = max;
      }
      this.cache.wave = { width, low, high };
    }
    const style = getComputedStyle(document.documentElement);
    const played = style.getPropertyValue("--link").trim() || "#1f63c4";
    const rest = style.getPropertyValue("--muted").trim() || "#6b7887";
    context.clearRect(0, 0, width, height);
    const span = range.end - range.start || 1;
    const cursorX = ((this.cursor() - range.start) / span) * width;
    const { low, high } = this.cache.wave;
    for (let x = 0; x < width; x++) {
      context.fillStyle = x < cursorX ? played : rest;
      const top = ((1 - high[x]) / 2) * height;
      const bottom = ((1 - low[x]) / 2) * height;
      context.fillRect(x, top, 1, Math.max(1, bottom - top));
    }
    this.overlay(
      context,
      width,
      height,
      style.getPropertyValue("--s-bad").trim() || "#c42b2b",
    );
  }

  drawSpectrogram() {
    const canvas = this.spec.canvas;
    const context = canvas.getContext && canvas.getContext("2d");
    if (!context) return;
    const { width, height } = this.size(canvas);
    if (!width || !height) return;
    const { samples, rate } = this.audioData;
    const range = this.range();
    if (
      !this.cache.spec ||
      this.cache.spec.width !== width ||
      this.cache.spec.height !== height
    ) {
      // A window of about 32 ms, as a power of two.
      const size = Math.min(
        4096,
        Math.max(256, 2 ** Math.round(Math.log2(rate * 0.032))),
      );
      const bins = size / 2;
      const columns = new Float32Array(width * bins);
      const column = new Float32Array(bins);
      const perPixel = ((range.end - range.start) * rate) / width;
      let loudest = -Infinity;
      for (let x = 0; x < width; x++) {
        const middle = Math.floor(range.start * rate + (x + 0.5) * perPixel);
        magnitudes(samples, middle - bins, size, column);
        for (let bin = 0; bin < bins; bin++) {
          const level = 20 * Math.log10(column[bin] + 1e-10);
          columns[x * bins + bin] = level;
          if (level > loudest) loudest = level;
        }
      }
      const image = context.createImageData(width, height);
      for (let y = 0; y < height; y++) {
        const bin = Math.min(
          bins - 1,
          Math.floor((1 - (y + 0.5) / height) * bins),
        );
        for (let x = 0; x < width; x++) {
          const level =
            (columns[x * bins + bin] - loudest + spectrogramRange) /
            spectrogramRange;
          const color = Math.round(Math.min(1, Math.max(0, level)) * 255) * 3;
          const offset = (y * width + x) * 4;
          image.data[offset] = viridisTable[color];
          image.data[offset + 1] = viridisTable[color + 1];
          image.data[offset + 2] = viridisTable[color + 2];
          image.data[offset + 3] = 255;
        }
      }
      this.cache.spec = { width, height, image };
      this.spec.box.dataset.top = formatRate(rate / 2);
    }
    context.putImageData(this.cache.spec.image, 0, 0);
    this.overlay(context, width, height, "#ffffff");
  }

  dispose() {
    if (this.player && !this.player.paused) this.player.pause();
    if (this.resizer) this.resizer.disconnect();
    this.root.remove();
  }
}

// The handle between the panes and the table sets the panes' height.
function startPanesHandle() {
  const handle = document.getElementById("tv-panes-handle");
  const area = document.getElementById("tv-panes");
  handle.addEventListener("pointerdown", (event) => {
    event.preventDefault();
    const start = event.clientY;
    const height = area.offsetHeight;
    const move = (moved) => {
      const next = Math.max(160, height + moved.clientY - start);
      area.style.height = next + "px";
    };
    const up = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      tabViews.forEach((view) => view.redraw());
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
  });
}

// renderPaneToolbar lays out the zoom controls, which act on the active
// pane's current file.
function renderPaneToolbar() {
  const toolbar = document.getElementById("tv-pane-toolbar");
  if (!toolbar) return;
  const pane = activePane();
  const view = pane && tabViews.get(pane.current);
  const audio = view && view.player;
  const range = audio ? view.range() : null;
  setChildren(
    toolbar,
    typeof linkToggle === "function" ? linkToggle() : null,
    el("span", { className: "tv-spacer" }),
    el("span", { className: "tv-muted" }, "Zoom"),
    el(
      "span",
      { className: "tv-segmented" },
      el(
        "button",
        {
          type: "button",
          "aria-label": "Zoom out",
          disabled: !audio,
          onclick: () => zoomActive(2),
        },
        "−",
      ),
      el(
        "button",
        {
          type: "button",
          "aria-label": "Zoom in",
          disabled: !audio,
          onclick: () => zoomActive(0.5),
        },
        "+",
      ),
      el(
        "button",
        { type: "button", disabled: !audio, onclick: () => zoomActive(0) },
        "Fit",
      ),
    ),
    range
      ? el(
          "span",
          { className: "tv-range" },
          range.start.toFixed(2) + " – " + range.end.toFixed(2) + " s",
        )
      : null,
    el(
      "span",
      { className: "tv-muted tv-hint" },
      "drag on a plot to zoom to a range · wheel to scroll · click to seek",
    ),
  );
}

// zoomActive zooms the active pane's audio by a factor, or fits it for 0.
function zoomActive(factor) {
  const pane = activePane();
  const view = pane && tabViews.get(pane.current);
  if (!view || !view.player) return;
  if (factor) {
    view.zoom(factor);
  } else {
    view.setRange({ start: 0, end: view.duration() || 1 });
  }
  renderPaneToolbar();
}
