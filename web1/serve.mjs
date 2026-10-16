#!/usr/bin/env node
// AgentPlane control plane — development server.
//
// Serves the static app AND proxies the gateway API, which a plain static file
// server cannot do. This is the dev equivalent of nginx.conf.
//
//   node serve.mjs                 # http://localhost:8080 -> gateway :3001
//   PORT=9001 GATEWAY_URL=http://gateway:3001 node serve.mjs
//
// Zero dependencies.

import { createServer, request as httpRequest } from "node:http";
import { createReadStream, existsSync, statSync } from "node:fs";
import { extname, join, normalize, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";

const ROOT = resolve(fileURLToPath(new URL(".", import.meta.url)));
const PORT = Number(process.env.PORT || 8080);
const GATEWAY = process.env.GATEWAY_URL || "http://localhost:3001";
const gw = new URL(GATEWAY);
const GRAFANA = process.env.GRAFANA_URL || "http://localhost:3000";
const graf = new URL(GRAFANA);

// Paths forwarded to the gateway (kept in sync with nginx.conf).
const API_PREFIXES = [
  "/chat", "/events", "/provision", "/admin", "/metrics", "/api/observability", "/analytics",
];

const isGrafana = (pathname) => pathname === "/grafana" || pathname.startsWith("/grafana/");

const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".mjs": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".svg": "image/svg+xml",
  ".png": "image/png",
  ".jpg": "image/jpeg",
  ".ico": "image/x-icon",
  ".woff2": "font/woff2",
  ".map": "application/json; charset=utf-8",
};

const isApi = (pathname) =>
  API_PREFIXES.some((p) => pathname === p || pathname.startsWith(p + "/"));

function log(...a) {
  console.log(`[web1]`, ...a);
}

function proxy(req, res, pathname, search, target = gw) {
  const headers = { ...req.headers, host: target.host, "accept-encoding": "identity" };
  const upstream = httpRequest(
    {
      protocol: target.protocol,
      hostname: target.hostname,
      port: target.port || (target.protocol === "https:" ? 443 : 80),
      path: pathname + search,
      method: req.method,
      headers,
    },
    (up) => {
      res.writeHead(up.statusCode || 502, up.headers);
      up.pipe(res); // streams; SSE (/events) passes through unbuffered
    }
  );
  upstream.on("error", (err) => {
    if (res.headersSent) return res.destroy();
    res.writeHead(502, { "Content-Type": "application/json" });
    res.end(
      JSON.stringify({
        error: { type: "gateway_unreachable", message: `Cannot reach ${target.href}: ${err.message}` },
      })
    );
  });
  req.pipe(upstream);
}

function serveStatic(pathname, req, res) {
  let rel = decodeURIComponent(pathname);
  if (rel.endsWith("/")) rel += "index.html";
  let filePath = normalize(join(ROOT, rel));
  if (!filePath.startsWith(ROOT + sep) && filePath !== ROOT) {
    res.writeHead(403);
    return res.end("Forbidden");
  }

  const exists = existsSync(filePath) && statSync(filePath).isFile();
  if (!exists) {
    // SPA fallback: extension-less paths render the app shell.
    if (extname(rel) && extname(rel) !== ".html") {
      res.writeHead(404, { "Content-Type": "text/plain" });
      return res.end("Not found");
    }
    filePath = join(ROOT, "index.html");
  }
  if (!existsSync(filePath)) {
    res.writeHead(404, { "Content-Type": "text/plain" });
    return res.end("Not found");
  }

  res.writeHead(200, {
    "Content-Type": MIME[extname(filePath)] || "application/octet-stream",
    "Cache-Control": "no-cache",
  });
  createReadStream(filePath).pipe(res);
}

const server = createServer((req, res) => {
  const url = new URL(req.url, `http://${req.headers.host || "localhost"}`);
  if (isGrafana(url.pathname)) {
    if (url.pathname === "/grafana") {
      res.writeHead(301, { Location: "/grafana/" });
      return res.end();
    }
    return proxy(req, res, url.pathname, url.search, graf);
  }
  if (isApi(url.pathname)) return proxy(req, res, url.pathname, url.search, gw);
  if (req.method !== "GET" && req.method !== "HEAD") {
    res.writeHead(405, { "Content-Type": "text/plain" });
    return res.end("Method not allowed");
  }
  serveStatic(url.pathname, req, res);
});

server.listen(PORT, () => {
  log(`control plane  →  http://localhost:${PORT}`);
  log(`api gateway    →  ${GATEWAY}`);
  log(`grafana        →  http://localhost:${PORT}/grafana/  (${GRAFANA})`);
  log(`proxying         ${API_PREFIXES.join(", ")}`);
});
