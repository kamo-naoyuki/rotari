// Record the visible structure of every Web UI page: title, summary,
// toolbar buttons, headings, and each table's headers and cells (text,
// buttons, status pills). Called by web-structure.sh:
//
//   node scripts/web-structure.mjs CHROME STATIC_DIR LIVE_URL OUTPUT_JSON
//
// It visits each page of the static export and, when LIVE_URL is not "-",
// the same page on a live `rotari web` server. Two outputs of the same state
// from two builds can be compared with diff to show what a change did to the
// pages.
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { launchChrome, listStaticPages, withPage } from "./web-browser.mjs";

const [chrome, staticDir, liveURL, outputJSON] = process.argv.slice(2);
if (!outputJSON) {
  console.error(
    "usage: node web-structure.mjs CHROME STATIC_DIR LIVE_URL|- OUTPUT_JSON",
  );
  process.exit(2);
}

// The live path of a static page.
function livePath(path) {
  const parts = path.split("/").slice(0, -1);
  if (parts.length === 0) return "/";
  if (parts.length === 1) return "/" + parts[0] + "/";
  return "/" + parts.join("/");
}

// Runs in the page. Sidebar and modals are left out: the sidebar is shared
// and the modals are not open at load.
const extract = `(() => {
  const text = (node) => (node ? node.textContent.replace(/\\s+/g, " ").trim() : "");
  const ownText = (cell) => {
    const copy = cell.cloneNode(true);
    copy.querySelectorAll("button, details.attempt-menu").forEach((node) => node.remove());
    return text(copy);
  };
  const buttons = (root) =>
    [...root.querySelectorAll("button, a.link, a.show-path")]
      .filter((node) => !node.closest(".sidebar, .output-modal, .orbit-game, #card"))
      .map((node) => text(node) + (node.disabled ? " (disabled)" : ""))
      .filter(Boolean);
  const main = document.querySelector(".content") || document.body;
  const tables = [...main.querySelectorAll("table")].map((table) => ({
    in: table.closest("section")?.className || table.parentElement?.className || "",
    headers: [...table.querySelectorAll("thead th")].map((header) =>
      text(header).replace(/ [↑↓↕]$/, ""),
    ),
    rows: [...table.querySelectorAll("tbody tr")].map((row) => ({
      hidden: row.style.display === "none" || undefined,
      cells: [...row.children].map((cell) => {
        const entry = { text: ownText(cell) };
        const cellButtons = [...cell.querySelectorAll("button")].map(
          (button) => (button.title && !text(button) ? "[" + button.title + "]" : text(button)) + (button.disabled ? " (disabled)" : ""),
        );
        if (cellButtons.length) entry.buttons = cellButtons;
        const pills = [...cell.querySelectorAll(".status-pill")].map((pill) => pill.className);
        if (pills.length) entry.pills = pills;
        const inputs = [...cell.querySelectorAll("input, select, textarea")].map(
          (field) => field.tagName.toLowerCase() + (field.type ? ":" + field.type : ""),
        );
        if (inputs.length) entry.inputs = inputs;
        return entry;
      }),
    })),
  }));
  return {
    title: text(document.getElementById("page-title") || main.querySelector("h1")),
    summary: [...(document.getElementById("summary")?.children || [])].map(text),
    toolbars: [...main.querySelectorAll(".toolbar")].map(buttons),
    headings: [...main.querySelectorAll("h2")].map(text),
    // Empty-state messages; a container that happens to share the class and
    // holds the page is not one.
    empty: [...main.querySelectorAll(".empty")]
      .filter((node) => !node.querySelector("table, section, h2"))
      .map(text),
    tables,
  };
})()`;

const chromeSession = await launchChrome(chrome);
const result = {};
let failed = false;
try {
  for (const [name, path] of listStaticPages(staticDir)) {
    const targets = [["static", "file://" + join(staticDir, path)]];
    if (liveURL !== "-")
      targets.push(["live", liveURL.replace(/\/$/, "") + livePath(path)]);
    for (const [mode, url] of targets) {
      try {
        result[name + " (" + mode + ")"] = await withPage(
          chromeSession.cdp,
          url,
          { width: 1400, height: 900, settleMs: mode === "live" ? 3000 : 1500 },
          (page) => page.evaluate(extract),
        );
      } catch (error) {
        failed = true;
        console.error(`${name} (${mode}): ${error.message}`);
      }
    }
  }
} finally {
  await chromeSession.close();
}
writeFileSync(outputJSON, JSON.stringify(result, null, 1) + "\n");
console.log(
  `structure of ${Object.keys(result).length} pages in ${outputJSON}`,
);
process.exit(failed ? 1 : 0);
