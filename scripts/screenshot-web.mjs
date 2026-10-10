// Capture full-page screenshots of a static web export over the Chrome
// DevTools protocol. Called by screenshot-web.sh:
//
//   node scripts/screenshot-web.mjs CHROME STATIC_DIR OUTPUT_DIR
//
// The protocol, unlike `chrome --screenshot`, sets the exact viewport (the
// window-size flag leaves the bottom of the image outside the viewport),
// emulates prefers-color-scheme, and captures the whole page height. Each
// capture runs in its own browser context, so localStorage from one page
// does not leak into the next. Needs Node 22 or later for WebSocket.
import { spawn } from "node:child_process";
import {
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
  writeFileSync,
  existsSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { execFileSync } from "node:child_process";

const [chrome, staticDir, outputDir] = process.argv.slice(2);
if (!outputDir) {
  console.error("usage: node screenshot-web.mjs CHROME STATIC_DIR OUTPUT_DIR");
  process.exit(2);
}

const sizes = [
  { name: "desktop", width: 1400, height: 900, mobile: false },
  { name: "phone", width: 390, height: 844, mobile: true },
];
const schemes = ["light", "dark"];
// Very long pages (a run with many jobs) are cut here to keep files small.
const maxHeight = 8000;
const settleMs = 1500;

// Name each page by its place in the export. Runs are numbered by run ID,
// which starts with the start time, so names match across exports.
function listPages() {
  const pages = [
    ["home", "index.html"],
    ["jobs", "jobs/index.html"],
    ["search", "search/index.html"],
  ];
  const projectsDir = join(staticDir, "project");
  for (const project of readdirSync(projectsDir).sort()) {
    pages.push([`project-${project}`, `project/${project}/index.html`]);
    const runsDir = join(projectsDir, project, "run");
    if (!existsSync(runsDir)) continue;
    readdirSync(runsDir)
      .sort()
      .forEach((runID, index) => {
        pages.push([
          `project-${project}-run-${index + 1}`,
          `project/${project}/run/${runID}/index.html`,
        ]);
      });
  }
  return pages.filter(([, path]) => existsSync(join(staticDir, path)));
}

const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

async function launch() {
  const profile = mkdtempSync(join(tmpdir(), "rotari-shots-"));
  const browser = spawn(
    chrome,
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-gpu",
      "--hide-scrollbars",
      "--remote-debugging-port=0",
      `--user-data-dir=${profile}`,
      "about:blank",
    ],
    { stdio: "ignore" },
  );
  const portFile = join(profile, "DevToolsActivePort");
  for (let i = 0; i < 100 && !existsSync(portFile); i++) await sleep(100);
  if (!existsSync(portFile))
    throw new Error("Chrome did not open a DevTools port");
  const [port, path] = readFileSync(portFile, "utf8").trim().split("\n");
  return { browser, profile, url: `ws://127.0.0.1:${port}${path}` };
}

function connect(url) {
  const socket = new WebSocket(url);
  let nextID = 0;
  const pending = new Map();
  const listeners = [];
  socket.addEventListener("message", (event) => {
    const message = JSON.parse(event.data);
    if (message.id !== undefined && pending.has(message.id)) {
      const { resolve, reject } = pending.get(message.id);
      pending.delete(message.id);
      message.error
        ? reject(new Error(message.error.message))
        : resolve(message.result);
    } else if (message.method) {
      listeners.forEach((listener) => listener(message));
    }
  });
  const send = (method, params = {}, sessionId) =>
    new Promise((resolve, reject) => {
      const id = ++nextID;
      pending.set(id, { resolve, reject });
      socket.send(JSON.stringify({ id, method, params, sessionId }));
    });
  const waitFor = (method, sessionId) =>
    new Promise((resolve) => {
      const listener = (message) => {
        if (message.method === method && message.sessionId === sessionId) {
          listeners.splice(listeners.indexOf(listener), 1);
          resolve(message.params);
        }
      };
      listeners.push(listener);
    });
  return new Promise((resolve, reject) => {
    socket.addEventListener("open", () =>
      resolve({ send, waitFor, close: () => socket.close() }),
    );
    socket.addEventListener("error", reject);
  });
}

async function capture(cdp, url, size, scheme, file) {
  const { browserContextId } = await cdp.send("Target.createBrowserContext");
  const { targetId } = await cdp.send("Target.createTarget", {
    url: "about:blank",
    browserContextId,
  });
  const { sessionId } = await cdp.send("Target.attachToTarget", {
    targetId,
    flatten: true,
  });
  try {
    await cdp.send("Page.enable", {}, sessionId);
    await cdp.send(
      "Emulation.setDeviceMetricsOverride",
      {
        width: size.width,
        height: size.height,
        deviceScaleFactor: 1,
        mobile: size.mobile,
      },
      sessionId,
    );
    await cdp.send(
      "Emulation.setEmulatedMedia",
      { features: [{ name: "prefers-color-scheme", value: scheme }] },
      sessionId,
    );
    const loaded = cdp.waitFor("Page.loadEventFired", sessionId);
    await cdp.send("Page.navigate", { url }, sessionId);
    await loaded;
    await sleep(settleMs);
    const metrics = await cdp.send("Page.getLayoutMetrics", {}, sessionId);
    const height = Math.min(
      maxHeight,
      Math.max(size.height, Math.ceil(metrics.cssContentSize.height)),
    );
    const width = Math.max(size.width, Math.ceil(metrics.cssContentSize.width));
    const { data } = await cdp.send(
      "Page.captureScreenshot",
      {
        format: "png",
        captureBeyondViewport: true,
        clip: { x: 0, y: 0, width, height, scale: 1 },
      },
      sessionId,
    );
    writeFileSync(join(outputDir, file), Buffer.from(data, "base64"));
    return { width, height };
  } finally {
    await cdp.send("Target.closeTarget", { targetId });
    await cdp.send("Target.disposeBrowserContext", { browserContextId });
  }
}

let revision = "unknown";
try {
  revision = execFileSync("git", ["rev-parse", "--short", "HEAD"], {
    encoding: "utf8",
  }).trim();
} catch {}

const { browser, profile, url } = await launch();
let failed = false;
try {
  const cdp = await connect(url);
  const pages = listPages();
  let html =
    '<!doctype html><meta charset="utf-8"><title>rotari Web screenshots</title>' +
    "<style>body{font:14px system-ui,sans-serif;margin:16px}figure{display:inline-block;vertical-align:top;margin:0 12px 24px 0}" +
    "img{border:1px solid #888;max-width:700px}figcaption{margin-bottom:4px}</style>" +
    `<p>Captured ${new Date().toISOString()} from ${revision}. A width larger than the viewport means the page scrolls sideways.</p>`;
  for (const [name, path] of pages) {
    html += `<h2>${name}</h2>`;
    for (const size of sizes) {
      for (const scheme of schemes) {
        const file = `${name}-${size.name}-${scheme}.png`;
        try {
          const shot = await capture(
            cdp,
            "file://" + join(staticDir, path),
            size,
            scheme,
            file,
          );
          html += `<figure><figcaption>${size.name} / ${scheme} (${shot.width}×${shot.height})</figcaption><a href="${file}"><img src="${file}" loading="lazy"></a></figure>`;
        } catch (error) {
          failed = true;
          console.error(`${file}: ${error.message}`);
        }
      }
    }
  }
  writeFileSync(join(outputDir, "index.html"), html);
  cdp.close();
  console.log(
    `screenshots of ${pages.length} pages in ${join(outputDir, "index.html")}`,
  );
} finally {
  browser.kill();
  await sleep(300);
  rmSync(profile, { recursive: true, force: true });
}
process.exit(failed ? 1 : 0);
