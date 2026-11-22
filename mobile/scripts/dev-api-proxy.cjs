const https = require("node:https");
const http = require("node:http");
const { URL } = require("node:url");

const PREFIXES = {
  ambatumove: "/__dev_proxy/ambatumove",
  ambatufit: "/__dev_proxy/ambatufit",
};

function apiBase(environmentKey, fallback) {
  const configured = process.env[environmentKey]?.trim();
  const parsed = new URL(configured || fallback);
  if (
    !["https:", "http:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password ||
    parsed.search ||
    parsed.hash
  ) {
    throw new Error(`${environmentKey} must be an HTTP(S) API base URL.`);
  }
  return parsed;
}

function responseError(response, status, code, message) {
  if (response.headersSent) {
    response.destroy();
    return;
  }
  response.writeHead(status, {
    "content-type": "application/json; charset=utf-8",
  });
  response.end(JSON.stringify({ error: { code, message } }));
}

function createDevApiProxy(request) {
  const targets = {
    ambatumove: apiBase(
      "EXPO_PUBLIC_AMBATUMOVE_API_URL",
      "https://nizdri.stei.fun/api/v1",
    ),
    ambatufit: apiBase(
      "EXPO_PUBLIC_AMBATUFIT_API_URL",
      "https://ambatufit.stei.fun/api/v1",
    ),
  };

  return function handleDevApiProxy(req, res) {
    const incoming = new URL(req.url || "/", "http://metro.local");
    const service = Object.keys(PREFIXES).find((key) => {
      const prefix = PREFIXES[key];
      return (
        incoming.pathname === prefix ||
        incoming.pathname.startsWith(`${prefix}/`)
      );
    });
    if (!service) return false;

    if (
      !["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"].includes(req.method)
    ) {
      responseError(
        res,
        405,
        "METHOD_NOT_ALLOWED",
        "Method is not supported by the development API proxy.",
      );
      return true;
    }

    const target = new URL(targets[service]);
    const apiPath = incoming.pathname.slice(PREFIXES[service].length);
    target.pathname = `${target.pathname.replace(/\/$/, "")}${apiPath || "/"}`;
    target.search = incoming.search;

    const headers = {};
    for (const name of [
      "accept",
      "authorization",
      "content-type",
      "idempotency-key",
      "if-match",
      "if-none-match",
      "x-api-key",
    ]) {
      if (req.headers[name]) headers[name] = req.headers[name];
    }

    if (process.env.NODE_ENV !== "production") {
      console.info(
        `[dev-api-proxy] ${req.method} ${incoming.pathname}${incoming.search} -> ${target.href}`,
      );
    }

    const makeRequest =
      request || (target.protocol === "https:" ? https.request : http.request);
    const upstreamRequest = makeRequest(
      target,
      { method: req.method, headers, timeout: 20000 },
      (upstreamResponse) => {
        const responseHeaders = {};
        for (const name of [
          "cache-control",
          "content-length",
          "content-type",
          "etag",
          "last-modified",
        ]) {
          const value = upstreamResponse.headers[name];
          if (value !== undefined) responseHeaders[name] = value;
        }
        res.writeHead(upstreamResponse.statusCode || 502, responseHeaders);
        upstreamResponse.pipe(res);
      },
    );

    upstreamRequest.setTimeout(20000, () => {
      upstreamRequest.destroy(
        new Error("Development proxy upstream timed out."),
      );
    });
    upstreamRequest.on("error", (error) => {
      console.error(
        `[dev-api-proxy] ${service} upstream error: ${error.message}`,
      );
      responseError(
        res,
        502,
        "DEV_PROXY_UPSTREAM_ERROR",
        "The upstream API could not be reached from the Expo development server.",
      );
    });
    req.on("aborted", () => upstreamRequest.destroy());
    req.pipe(upstreamRequest);
    return true;
  };
}

module.exports = { createDevApiProxy, PREFIXES };
