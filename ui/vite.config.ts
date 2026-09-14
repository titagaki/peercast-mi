import { ServerResponse } from "node:http";
import { defineConfig, loadEnv, type ProxyOptions } from "vite";
import react from "@vitejs/plugin-react";

// https://vite.dev/config/
export default defineConfig(({ mode }) => {
  // Server-only setting: never put OAuth secrets in VITE_* variables.
  const env = loadEnv(mode, process.cwd(), "PEERCAST_SITE_");
  const target = env.PEERCAST_SITE_TARGET || "http://127.0.0.1:8080";
  const siteProxy: ProxyOptions = {
    target,
    // Preserve the browser's Host, Origin and cookies for OAuth and CSRF.
    changeOrigin: false,
    configure(proxy) {
      proxy.on("error", (_error, _request, response) => {
        if (response instanceof ServerResponse && !response.headersSent) {
          response.writeHead(503, {
            "Content-Type": "text/plain; charset=utf-8",
            "Cache-Control": "no-store",
          });
          response.end(
            "サイト API に接続できません。Go の site.enabled、起動状態、PEERCAST_SITE_TARGET（既定 http://127.0.0.1:8080）を確認してください。",
          );
        }
      });
    },
  };
  const base = env.PEERCAST_SITE_BASE_PATH || "";
  if (base && !/^(\/[A-Za-z0-9_-]+)+$/.test(base)) {
    throw new Error(
      "PEERCAST_SITE_BASE_PATH must be a path such as /mi without a trailing slash",
    );
  }
  return {
    base: `${base}/`,
    plugins: [react()],
    server: {
      // OAuth callbacks must not silently move to another port.
      port: 5173,
      strictPort: true,
      // Do not proxy the administrative /api/1 endpoint.
      proxy: {
        [`${base}/site/`]: siteProxy,
        [`${base}/auth/`]: siteProxy,
        [`${base}/admin/api/`]: siteProxy,
      },
    },
  };
});
