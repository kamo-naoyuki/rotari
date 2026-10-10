// Headless Chrome over the DevTools protocol, shared by screenshot-web.mjs
// and web-structure.mjs. The protocol, unlike `chrome --screenshot`, sets the
// exact viewport (the window-size flag leaves the bottom of the image outside
// the viewport) and emulates prefers-color-scheme. Each page opens in its own
// browser context, so localStorage from one page does not leak into the
// next. Needs Node 22 or later for WebSocket.
import { spawn } from "node:child_process";
import {
  existsSync,
  mkdtempSync,
  readdirSync,
  readFileSync,
  rmSync,
} from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

export const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));

// listStaticPages names each page of a static export by its place in it.
// Runs are numbered by run ID, which starts with the start time, so names
// match across exports of the same state.
export function listStaticPages(staticDir) {
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

// launchChrome starts a headless Chrome and returns its protocol connection
// and a close function that stops it.
export async function launchChrome(chrome) {
  const profile = mkdtempSync(join(tmpdir(), "rotari-chrome-"));
  const browser = spawn(
    chrome,
    [
      "--headless=new",
      "--no-sandbox",
      "--disable-gpu",
      "--hide-scrollbars",
      "--no-proxy-server",
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
  const cdp = await connect(`ws://127.0.0.1:${port}${path}`);
  return {
    cdp,
    async close() {
      cdp.close();
      browser.kill();
      await sleep(300);
      rmSync(profile, { recursive: true, force: true });
    },
  };
}

// withPage opens url at the given viewport and colour scheme, waits for the
// load event and settleMs more, runs use(page), and closes the page.
// page.send calls a protocol method on the page; page.evaluate returns the
// value of a JavaScript expression in it.
export async function withPage(cdp, url, options, use) {
  const {
    width,
    height,
    mobile = false,
    scheme = "light",
    settleMs = 1500,
  } = options;
  const { browserContextId } = await cdp.send("Target.createBrowserContext");
  const { targetId } = await cdp.send("Target.createTarget", {
    url: "about:blank",
    browserContextId,
  });
  const { sessionId } = await cdp.send("Target.attachToTarget", {
    targetId,
    flatten: true,
  });
  const page = {
    send: (method, params) => cdp.send(method, params, sessionId),
    async evaluate(expression) {
      const result = await cdp.send(
        "Runtime.evaluate",
        { expression, returnByValue: true, awaitPromise: true },
        sessionId,
      );
      if (result.exceptionDetails)
        throw new Error(
          result.exceptionDetails.exception?.description ||
            result.exceptionDetails.text,
        );
      return result.result.value;
    },
  };
  try {
    await page.send("Page.enable");
    await page.send("Emulation.setDeviceMetricsOverride", {
      width,
      height,
      deviceScaleFactor: 1,
      mobile,
    });
    await page.send("Emulation.setEmulatedMedia", {
      features: [{ name: "prefers-color-scheme", value: scheme }],
    });
    const loaded = cdp.waitFor("Page.loadEventFired", sessionId);
    await page.send("Page.navigate", { url });
    await loaded;
    await sleep(settleMs);
    return await use(page);
  } finally {
    await cdp.send("Target.closeTarget", { targetId });
    await cdp.send("Target.disposeBrowserContext", { browserContextId });
  }
}
