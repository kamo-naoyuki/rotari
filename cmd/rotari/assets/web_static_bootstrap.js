window.__ROTARI_STATIC_STATE__ = __ROTARI_STATIC_STATE_DATA__;
window.__ROTARI_STATIC_LOGS__ = __ROTARI_STATIC_LOGS_DATA__;
window.__ROTARI_STATIC_REPORTS__ = __ROTARI_STATIC_REPORTS_DATA__;
// prettier-ignore
window.__ROTARI_STATIC_CONFIG_TEMPLATE__ = __ROTARI_STATIC_CONFIG_TEMPLATE_DATA__;

window.fetch = async function (input, init) {
  const request = new URL(input, window.location.href);
  if (request.pathname.endsWith("/api/state")) {
    return new Response(JSON.stringify(window.__ROTARI_STATIC_STATE__), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/log")) {
    const key = staticLogKey(
      request.searchParams.get("project_name"),
      request.searchParams.get("run_id"),
      request.searchParams.get("job_id"),
      request.searchParams.get("attempt_id"),
    );
    return new Response(window.__ROTARI_STATIC_LOGS__[key] || "", {
      headers: { "Content-Type": "text/plain" },
    });
  }
  if (request.pathname.endsWith("/api/report")) {
    const jobIDs = request.searchParams.getAll("job_ids");
    if (jobIDs.length) {
      const selectedReports = jobIDs.map(
        (jobID) =>
          window.__ROTARI_STATIC_REPORTS__[
            staticReportKey(
              request.searchParams.get("project_name"),
              request.searchParams.get("run_id"),
              jobID,
            )
          ],
      );
      const found = selectedReports.every(Boolean);
      return new Response(
        found ? selectedReports.join("\n\n") : "Report not found",
        {
          status: found ? 200 : 404,
          headers: { "Content-Type": "text/markdown" },
        },
      );
    }
    const key = staticReportKey(
      request.searchParams.get("project_name"),
      request.searchParams.get("run_id"),
      request.searchParams.get("job_id"),
    );
    const report = window.__ROTARI_STATIC_REPORTS__[key];
    return new Response(report || "Report not found", {
      status: report ? 200 : 404,
      headers: { "Content-Type": "text/markdown" },
    });
  }
  return new Response("This is a read-only static demo.", { status: 405 });
};

function staticLogKey(queue, run, job, attempt) {
  return [queue, run, job, attempt || ""].join("/");
}

function staticReportKey(project, run, job) {
  return [project, run, job || ""].join("/");
}

function staticRootPath() {
  const pathname = window.location.pathname;
  const parts = pathname.split("/").filter(Boolean);
  const projectIndex = parts.indexOf("project");
  if (projectIndex >= 0) {
    return "/" + parts.slice(0, projectIndex).join("/");
  }
  if (pathname.endsWith("/index.html")) {
    return "/" + parts.slice(0, -1).join("/");
  }
  if (pathname.endsWith("/")) {
    return parts.length ? "/" + parts.join("/") : "";
  }
  return "/" + parts.slice(0, -1).join("/");
}

function routeParts() {
  const root = staticRootPath().split("/").filter(Boolean);
  return window.location.pathname.split("/").filter(Boolean).slice(root.length);
}

function staticPath(path) {
  const root = staticRootPath().replace(/\/$/, "");
  return path === root || path.startsWith(root + "/") ? path : root + path;
}

function rewriteStaticLinks() {
  document.querySelectorAll('a[href^="/"]').forEach((link) => {
    link.setAttribute("href", staticPath(link.getAttribute("href")));
  });
}

rewriteStaticLinks();
new MutationObserver(rewriteStaticLinks).observe(document.body, {
  childList: true,
  subtree: true,
});
