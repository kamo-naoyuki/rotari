window.__ROTARI_STATIC_STATE__ = __ROTARI_STATIC_STATE_DATA__;
window.__ROTARI_STATIC_LOGS__ = __ROTARI_STATIC_LOGS_DATA__;
window.__ROTARI_STATIC_REPORTS__ = __ROTARI_STATIC_REPORTS_DATA__;
window.__ROTARI_STATIC_CONFIG_TARGETS__ = __ROTARI_STATIC_CONFIG_TARGETS_DATA__;
window.__ROTARI_STATIC_CONFIGS__ = __ROTARI_STATIC_CONFIGS_DATA__;
window.__ROTARI_STATIC_WORD_CLOUDS__ = __ROTARI_STATIC_WORD_CLOUDS_DATA__;
window.__ROTARI_STATIC_ARTIFACTS__ = __ROTARI_STATIC_ARTIFACTS_DATA__;
window.__ROTARI_STATIC_ARTIFACT_CONTENTS__ =
  __ROTARI_STATIC_ARTIFACT_CONTENTS_DATA__;

window.fetch = async function (input, init) {
  const request = new URL(input, window.location.href);
  if (request.pathname.endsWith("/api/state")) {
    return new Response(JSON.stringify(window.__ROTARI_STATIC_STATE__), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/config-targets")) {
    const project = request.searchParams.get("project_name") || "";
    const targets = window.__ROTARI_STATIC_CONFIG_TARGETS__[project];
    if (!targets) {
      return new Response("invalid project_name " + JSON.stringify(project), {
        status: 400,
      });
    }
    return new Response(JSON.stringify({ targets }), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/config")) {
    const key = staticConfigKey(
      request.searchParams.get("project_name") || "",
      request.searchParams.get("run_id") || "",
    );
    const configs = window.__ROTARI_STATIC_CONFIGS__[key];
    if (!configs) return new Response("Config not found", { status: 404 });
    return new Response(JSON.stringify({ configs }), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/output-word-cloud")) {
    const key = staticWordCloudKey(
      request.searchParams.get("project_name"),
      request.searchParams.get("run_id"),
    );
    const cloud = window.__ROTARI_STATIC_WORD_CLOUDS__[key];
    return new Response(JSON.stringify(cloud || { terms: [] }), {
      status: cloud ? 200 : 404,
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/artifacts")) {
    const listing =
      window.__ROTARI_STATIC_ARTIFACTS__[
        staticArtifactsKey(
          request.searchParams.get("project_name"),
          request.searchParams.get("run_id"),
          request.searchParams.get("job_id"),
          request.searchParams.get("attempt_id"),
        )
      ];
    if (!listing) return new Response("Artifacts not found", { status: 404 });
    return new Response(JSON.stringify(listing), {
      headers: { "Content-Type": "application/json" },
    });
  }
  const artifactRoute = request.pathname.match(
    /\/api\/artifact-(text|array|directory|file)$/,
  );
  if (artifactRoute) {
    const contents = window.__ROTARI_STATIC_ARTIFACT_CONTENTS__;
    const key = staticArtifactEntryKey(request.searchParams);
    if (artifactRoute[1] === "file") {
      const meta = (contents.meta || {})[key];
      if (!meta)
        return new Response("Not included in this static export", {
          status: 404,
        });
      return new Response(null, {
        headers: {
          "Content-Length": String(meta.size),
          "Last-Modified": meta.modified,
        },
      });
    }
    const page = (contents.pages || {})[key + artifactRoute[1]];
    if (!page || (request.searchParams.get("offset") || "0") !== "0") {
      return new Response("Not included in this static export", {
        status: 404,
      });
    }
    return new Response(JSON.stringify(page), {
      headers: { "Content-Type": "application/json" },
    });
  }
  if (request.pathname.endsWith("/api/log")) {
    const key = staticLogKey(
      request.searchParams.get("project_name"),
      request.searchParams.get("run_id"),
      request.searchParams.get("job_id"),
      request.searchParams.get("stream") || "stdout",
      request.searchParams.get("attempt_id"),
    );
    return new Response(window.__ROTARI_STATIC_LOGS__[key] || "", {
      headers: { "Content-Type": "text/plain" },
    });
  }
  if (request.pathname.endsWith("/api/report")) {
    const redact = request.searchParams.get("redact") !== "false";
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
          ]?.[redact ? "redacted" : "unredacted"],
      );
      const found = selectedReports.every(Boolean);
      if (!found) {
        return new Response("Report not found", {
          status: 404,
          headers: { "Content-Type": "text/markdown" },
        });
      }
      // Each precomputed per-job report ends with its own redaction notice;
      // keep only one when joining them into a single selected-jobs report.
      const noticePattern = /\n> [^\n]*\n$/;
      const notices = selectedReports.map((report) =>
        report.match(noticePattern),
      );
      const bodies = selectedReports.map((report, index) =>
        notices[index]
          ? report.slice(0, report.length - notices[index][0].length)
          : report,
      );
      const notice = notices.find(Boolean);
      return new Response(bodies.join("\n\n") + (notice ? notice[0] : ""), {
        status: 200,
        headers: { "Content-Type": "text/markdown" },
      });
    }
    const key = staticReportKey(
      request.searchParams.get("project_name"),
      request.searchParams.get("run_id"),
      request.searchParams.get("job_id"),
    );
    const report =
      window.__ROTARI_STATIC_REPORTS__[key]?.[
        redact ? "redacted" : "unredacted"
      ];
    return new Response(report || "Report not found", {
      status: report ? 200 : 404,
      headers: { "Content-Type": "text/markdown" },
    });
  }
  return new Response(
    "the web UI is read-only; restart with --allow-control to enable job control\n",
    { status: 403 },
  );
};

function staticLogKey(queue, run, job, stream, attempt) {
  return [queue, run, job, stream, attempt || ""].join("/");
}

function staticArtifactsKey(project, run, job, attempt) {
  return [project, run, job, attempt || ""].join("/");
}

// staticArtifactEntryKey identifies a listed entry's embedded contents,
// matching staticArtifactEntryKey in internal/webui/static_artifacts.go.
// Children of a listed directory are not embedded, so their keys match
// nothing.
function staticArtifactEntryKey(params) {
  return [
    params.get("project_name"),
    params.get("run_id"),
    params.get("job_id"),
    params.get("attempt_id") || "",
    params.get("entry"),
    params.get("child") || "",
  ].join("/");
}

// staticArtifactFileURL is the export's copy of a listed file, for img,
// audio, video, and download links, or "" when none was copied.
function staticArtifactFileURL(params) {
  const files = window.__ROTARI_STATIC_ARTIFACT_CONTENTS__.files || {};
  return files[staticArtifactEntryKey(params)] || "";
}

function staticReportKey(project, run, job) {
  return [project, run, job || ""].join("/");
}

function staticConfigKey(project, run) {
  return [project, run].join("/");
}

function staticWordCloudKey(project, run) {
  return [project, run].join("/");
}

function staticRootPath() {
  const pathname = window.location.pathname;
  const parts = pathname.split("/").filter(Boolean);
  const searchIndex = parts.lastIndexOf("search");
  if (
    searchIndex >= 0 &&
    (searchIndex === parts.length - 1 ||
      (parts.at(-1) === "index.html" && searchIndex === parts.length - 2))
  )
    return "/" + parts.slice(0, searchIndex).join("/");
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
