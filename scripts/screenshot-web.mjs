// Capture full-page screenshots of a static web export. Called by
// screenshot-web.sh:
//
//   node scripts/screenshot-web.mjs CHROME STATIC_DIR OUTPUT_DIR
//
// Each page is captured in full at desktop and phone widths, in light and
// dark, over the DevTools protocol (web-browser.mjs).
import { writeFileSync } from "node:fs";
import { join } from "node:path";
import { execFileSync } from "node:child_process";
import { launchChrome, listStaticPages, withPage } from "./web-browser.mjs";

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

async function capture(page, size, file) {
  const metrics = await page.send("Page.getLayoutMetrics");
  const height = Math.min(
    maxHeight,
    Math.max(size.height, Math.ceil(metrics.cssContentSize.height)),
  );
  const width = Math.max(size.width, Math.ceil(metrics.cssContentSize.width));
  const { data } = await page.send("Page.captureScreenshot", {
    format: "png",
    captureBeyondViewport: true,
    clip: { x: 0, y: 0, width, height, scale: 1 },
  });
  writeFileSync(join(outputDir, file), Buffer.from(data, "base64"));
  return { width, height };
}

let revision = "unknown";
try {
  revision = execFileSync("git", ["rev-parse", "--short", "HEAD"], {
    encoding: "utf8",
  }).trim();
} catch {}

const chromeSession = await launchChrome(chrome);
let failed = false;
try {
  const pages = listStaticPages(staticDir);
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
          const shot = await withPage(
            chromeSession.cdp,
            "file://" + join(staticDir, path),
            { ...size, scheme },
            (page) => capture(page, size, file),
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
  console.log(
    `screenshots of ${pages.length} pages in ${join(outputDir, "index.html")}`,
  );
} finally {
  await chromeSession.close();
}
process.exit(failed ? 1 : 0);
