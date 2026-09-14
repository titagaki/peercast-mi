import assert from "node:assert/strict";
import { createServer as createHTTPServer } from "node:http";
import { test } from "node:test";
import { createServer } from "vite";

test("Vite forwards site API, OAuth and media without exposing admin API", async () => {
  const calls = [];
  const backend = createHTTPServer(async (req, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    calls.push({
      url: req.url,
      headers: req.headers,
      method: req.method,
      body,
    });
    if (req.url.startsWith("/auth/")) {
      res.writeHead(302, {
        Location: "/watch",
        "Set-Cookie": "mi_session=test; Path=/; HttpOnly; SameSite=Lax",
      });
      return res.end();
    }
    if (req.url.startsWith("/site/stream/")) {
      res.writeHead(200, { "Content-Type": "video/x-flv" });
      return res.end("FLV-test");
    }
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ user: null }));
  });
  await new Promise((resolve) => backend.listen(0, "127.0.0.1", resolve));
  const oldTarget = process.env.PEERCAST_SITE_TARGET;
  process.env.PEERCAST_SITE_TARGET = `http://127.0.0.1:${backend.address().port}`;
  let vite;
  try {
    // Load the real vite.config.ts, including the proxy and error handler.
    vite = await createServer({
      server: { host: "127.0.0.1", port: 0 },
      logLevel: "silent",
    });
    // Vite's listen(0) falls back to its default port. Bind its HTTP server
    // directly to an ephemeral port so the user's running 5173 is untouched.
    await new Promise((resolve, reject) => {
      vite.httpServer.once("error", reject);
      vite.httpServer.listen(0, "127.0.0.1", resolve);
    });
    const origin = `http://127.0.0.1:${vite.httpServer.address().port}`;
    const request = (path, options = {}) =>
      fetch(origin + path, { ...options, signal: AbortSignal.timeout(3000) });
    assert.deepEqual(await (await request("/site/api/me")).json(), {
      user: null,
    });
    await request("/site/api/broadcast", {
      method: "POST",
      headers: {
        Origin: origin,
        Cookie: "mi_session=test",
        "X-CSRF-Token": "csrf",
        "Content-Type": "application/json",
      },
      body: '{"name":"test"}',
    });
    assert.equal(calls[1].headers.origin, origin);
    assert.equal(calls[1].headers.host, new URL(origin).host);
    assert.equal(calls[1].headers.cookie, "mi_session=test");
    assert.equal(calls[1].headers["x-csrf-token"], "csrf");
    assert.equal(calls[1].body, '{"name":"test"}');
    for (const path of [
      "/auth/x/start",
      "/auth/x/callback?code=code&state=state",
    ]) {
      const response = await request(path, { redirect: "manual" });
      assert.equal(response.status, 302);
      assert.equal(response.headers.get("location"), "/watch");
      assert.match(response.headers.get("set-cookie"), /mi_session=test/);
      assert.equal(calls.at(-1).url, path);
    }
    assert.equal(
      await (
        await request("/site/stream/01234567890123456789012345678901")
      ).text(),
      "FLV-test",
    );
    const count = calls.length;
    await request("/api/1", { method: "POST", body: "{}" });
    await request("/watch");
    assert.equal(
      calls.length,
      count,
      "admin and UI paths must not reach the site backend",
    );
    backend.closeAllConnections();
    await new Promise((resolve) => backend.close(resolve));
    const failed = await request("/site/api/me");
    assert.equal(failed.status, 503);
    assert.match(await failed.text(), /site.enabled/);
  } finally {
    await vite?.close();
    backend.closeAllConnections();
    if (backend.listening)
      await new Promise((resolve) => backend.close(resolve));
    if (oldTarget === undefined) delete process.env.PEERCAST_SITE_TARGET;
    else process.env.PEERCAST_SITE_TARGET = oldTarget;
  }
});
