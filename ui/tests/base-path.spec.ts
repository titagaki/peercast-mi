import { test, expect } from "@playwright/test";

test("compiled /mi site keeps navigation, login and media under its mount point", async ({
  page,
}) => {
  const id = "0123456789abcdef0123456789abcdef";
  let loggedIn = false;
  const requests: string[] = [];
  page.on("request", (r) => {
    if (r.url().startsWith("http://127.0.0.1:4174/"))
      requests.push(new URL(r.url()).pathname);
  });
  await page.route("**/mi/site/api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    if (path.endsWith("/me"))
      return r.fulfill({
        json: {
          user: loggedIn ? { id: "1", name: "Viewer" } : null,
          csrf: "csrf",
        },
      });
    if (path.endsWith("/directory"))
      return r.fulfill({
        json: {
          channels: [
            {
              id,
              name: "Test Live",
              genre: "Music",
              description: "Live",
              contactUrl: "",
              contentType: "FLV",
              receiving: true,
              listeners: 1,
            },
          ],
          sources: [],
        },
      });
    if (path.endsWith("/comments"))
      return r.fulfill({
        json: { supported: false, comments: [], threads: [] },
      });
    if (path.endsWith("/broadcast"))
      return r.fulfill({
        json: {
          streamKey: "",
          rtmpUrl: "rtmp://localhost/live",
          channel: null,
        },
      });
    return r.abort();
  });
  await page.route("**/mi/site/stream/**", (r) =>
    r.fulfill({ status: 503, body: "test stream" }),
  );
  await page.goto(`/mi/channels/${id}`);
  await expect(
    page.getByRole("link", { name: "X でログイン" }),
  ).toHaveAttribute(
    "href",
    `/mi/auth/x/start?next=${encodeURIComponent(`/mi/channels/${id}`)}`,
  );
  await expect(page.getByRole("link", { name: "管理パネル" })).toHaveAttribute(
    "href",
    "/mi/admin",
  );
  loggedIn = true;
  await page.goto("/mi/");
  const channel = page.getByRole("link", { name: /Test Live/ });
  await expect(channel).toHaveAttribute("href", `/mi/channels/${id}`);
  await channel.click();
  await expect
    .poll(() => requests.includes(`/mi/site/stream/${id}`))
    .toBe(true);
  await page.getByRole("button", { name: "メニュー", exact: true }).click();
  await page.getByRole("link", { name: "配信する", exact: true }).click();
  await expect(page).toHaveURL(/\/mi\/broadcast$/);
  await expect
    .poll(() => requests.includes("/mi/site/api/broadcast"))
    .toBe(true);
  expect(requests.some((path) => path.startsWith("/mi/assets/"))).toBe(true);
  expect(requests.filter((path) => !path.startsWith("/mi/"))).toEqual([]);
});

test("production admin checks X permissions and sends same-origin RPC with CSRF", async ({
  page,
}) => {
  let role = "anonymous";
  const calls: string[] = [];
  await page.route("**/mi/site/api/me", (r) =>
    r.fulfill({
      json: {
        user: role === "anonymous" ? null : { id: "123", name: "Viewer" },
        admin: role === "admin",
        csrf: "admin-csrf",
      },
    }),
  );
  await page.route("**/mi/admin/api/1", (r) => {
    expect(r.request().headers()["x-csrf-token"]).toBe("admin-csrf");
    const rpc = r.request().postDataJSON();
    calls.push(rpc.method);
    return r.fulfill({ json: { jsonrpc: "2.0", id: rpc.id, result: [] } });
  });
  await page.goto("/mi/admin");
  await expect(
    page.getByRole("link", { name: "X でログイン" }),
  ).toHaveAttribute("href", "/mi/auth/x/start?next=%2Fmi%2Fadmin");
  expect(calls).toEqual([]);
  role = "viewer";
  await page.reload();
  await expect(page.getByRole("alert")).toContainText("管理権限がありません");
  expect(calls).toEqual([]);
  role = "admin";
  await page.reload();
  await expect.poll(() => calls.includes("getChannels")).toBe(true);
  await page
    .getByRole("button", { name: "ストリームキー", exact: true })
    .click();
  await expect.poll(() => calls.includes("listStreamKeys")).toBe(true);
});

for (const viewport of [
  { width: 1440, height: 1000 },
  { width: 390, height: 844 },
]) {
  test(`audit history searches, pages and opens broadcast inputs at ${viewport.width}px`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    const runID = "1234567890abcdef1234567890abcdef";
    const event = {
      id: "event-1",
      node: "mi-production",
      boot: "boot",
      seq: 1,
      at: "2026-09-16T14:49:00Z",
      recordedAt: "2026-09-16T14:49:01Z",
      type: "auth.login",
      actor: {
        account: "site:x:123",
        name: "配信者",
        ip: "2001:db8::1",
        source: "site",
      },
      outcome: "success",
      version: 1,
      payload: {},
    };
    const settings = {
      name: "いまいch",
      genre: "ypゲーム",
      inputGenre: "ゲーム",
      description: "配信内容",
      comment: "こんにちは",
      contactUrl: "https://example.test/",
      bitrate: 800,
      contentType: "FLV",
    };
    let fail = false;
    const queries: URL[] = [];
    await page.route("**/mi/admin/api/1", (r) =>
      r.fulfill({
        json: { jsonrpc: "2.0", id: r.request().postDataJSON().id, result: [] },
      }),
    );
    await page.route("**/mi/site/api/**", (r) => {
      const url = new URL(r.request().url());
      queries.push(url);
      if (url.pathname.endsWith("/me"))
        return r.fulfill({
          json: {
            user: { id: "123", name: "管理者" },
            admin: true,
            csrf: "csrf",
          },
        });
      if (url.pathname.endsWith("/audit/status"))
        return r.fulfill({
          json: {
            enabled: true,
            degraded: false,
            lastSuccess: event.at,
            pendingBytes: 0,
            pendingFiles: 0,
            queued: 0,
            dropped: 0,
            quarantinedFiles: 0,
          },
        });
      if (url.pathname.endsWith("/audit/events")) {
        if (fail)
          return r.fulfill({
            status: 503,
            body: "ログを取得できませんでした。",
          });
        if (url.searchParams.get("actor") === "site:x:999")
          return r.fulfill({ json: { items: [] } });
        return r.fulfill({
          json: {
            items: [
              {
                ...event,
                id: url.searchParams.get("cursor") ? "event-2" : "event-1",
                actor: {
                  ...event.actor,
                  name: url.searchParams.get("cursor")
                    ? "次のページの利用者"
                    : "配信者",
                },
              },
            ],
            nextCursor: url.searchParams.get("cursor") ? undefined : "page-2",
          },
        });
      }
      if (url.pathname.endsWith("/audit/broadcasts"))
        return r.fulfill({
          json: {
            items: [
              {
                id: runID,
                channelId: runID,
                owner: "site:x:123",
                ownerName: "配信者",
                actor: event.actor,
                settings,
                created: event.at,
                firstMedia: event.at,
                lastMedia: event.at,
                interrupted: "2026-09-17T00:00:00Z",
                status: "interrupted",
                reason: "process_interrupted",
                incomplete: true,
              },
            ],
          },
        });
      if (url.pathname.endsWith(`/broadcasts/${runID}/inputs`))
        return r.fulfill({
          json: {
            items: [
              {
                id: "input-1",
                broadcastId: runID,
                connectionId: "conn-1",
                remoteIp: "192.0.2.55",
                started: event.at,
                lastMedia: event.at,
                interrupted: "2026-09-17T00:00:00Z",
                reason: "process_interrupted",
                incomplete: true,
              },
            ],
          },
        });
      return r.abort();
    });
    await page.goto("/mi/admin");
    await page.getByRole("button", { name: "ログ", exact: true }).click();
    await expect(page.getByText("記録は正常です")).toBeVisible();
    const events = page.getByRole("region", { name: "操作ログ一覧" });
    await expect(
      events.getByRole("cell", { name: "配信者 site:x:123", exact: true }),
    ).toBeVisible();
    await page.getByRole("button", { name: "次のページ", exact: true }).click();
    await expect(events.getByText("次のページの利用者")).toBeVisible();
    await page.getByRole("button", { name: "前のページ", exact: true }).click();
    await expect(
      events.getByRole("cell", { name: "配信者 site:x:123", exact: true }),
    ).toBeVisible();
    await page.getByLabel("操作したアカウント").fill("123");
    await page
      .getByRole("combobox", { name: "イベント", exact: true })
      .selectOption("auth.login");
    await page.getByRole("button", { name: "検索", exact: true }).click();
    await expect
      .poll(() =>
        queries.some(
          (u) =>
            u.searchParams.get("actor") === "site:x:123" &&
            u.searchParams.get("type") === "auth.login" &&
            !u.searchParams.get("cursor"),
        ),
      )
      .toBe(true);
    await page.getByLabel("操作したアカウント").fill("999");
    await page.getByRole("button", { name: "検索", exact: true }).click();
    await expect(page.getByText("該当する記録はありません。")).toBeVisible();
    fail = true;
    await page.getByRole("button", { name: "最新を取得", exact: true }).click();
    await expect(page.getByRole("alert")).toContainText(
      "ログを取得できませんでした",
    );
    fail = false;
    await page.getByRole("button", { name: "再試行", exact: true }).click();
    await expect(page.getByRole("alert")).toHaveCount(0);
    await page
      .getByRole("button", { name: "条件をクリア", exact: true })
      .click();
    await page.getByRole("button", { name: "配信履歴", exact: true }).click();
    const broadcasts = page.getByRole("region", { name: "配信履歴一覧" });
    await expect(
      broadcasts.getByText("いまいch", { exact: true }),
    ).toBeVisible();
    await expect(
      broadcasts.getByText("不明（中断）", { exact: true }),
    ).toBeVisible();
    await broadcasts.getByRole("button", { name: "詳細", exact: true }).click();
    await expect(
      page.getByRole("cell", { name: /^192\.0\.2\.55/ }),
    ).toBeVisible();
    const detailBox = await page
      .getByRole("region", { name: "いまいchの詳細", exact: true })
      .boundingBox();
    expect(detailBox).not.toBeNull();
    expect(detailBox!.x).toBeGreaterThanOrEqual(0);
    expect(detailBox!.x + detailBox!.width).toBeLessThanOrEqual(viewport.width);

    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth + 1,
      ),
    ).toBe(true);
    if (viewport.width === 1440)
      await page.screenshot({
        path: "/tmp/pecami-audit-history-desktop.png",
        fullPage: true,
      });
    else
      await page.screenshot({
        path: "/tmp/pecami-audit-history-mobile.png",
        fullPage: true,
      });
    await page.getByRole("button", { name: "この配信の操作ログ" }).click();
    await expect
      .poll(() =>
        queries.some(
          (u) =>
            u.pathname.endsWith("/audit/events") &&
            u.searchParams.get("broadcastId") === runID &&
            !u.searchParams.has("from"),
        ),
      )
      .toBe(true);
  });
}

test("audit history is not requested for ordinary users and explains disabled recording", async ({
  page,
}) => {
  let admin = false;
  let historyRequests = 0;
  await page.route("**/mi/site/api/**", (r) => {
    const path = new URL(r.request().url()).pathname;
    if (path.endsWith("/me"))
      return r.fulfill({
        json: { user: { id: "1", name: "利用者" }, admin, csrf: "csrf" },
      });
    if (path.endsWith("/audit/status"))
      return r.fulfill({
        json: {
          enabled: false,
          pendingBytes: 0,
          pendingFiles: 0,
          queued: 0,
          dropped: 0,
          quarantinedFiles: 0,
        },
      });
    historyRequests++;
    return r.abort();
  });
  await page.route("**/mi/admin/api/1", (r) =>
    r.fulfill({
      json: { jsonrpc: "2.0", id: r.request().postDataJSON().id, result: [] },
    }),
  );
  await page.goto("/mi/admin");
  await expect(page.getByRole("alert")).toContainText("管理権限がありません");
  expect(historyRequests).toBe(0);
  admin = true;
  await page.reload();
  await page.getByRole("button", { name: "ログ", exact: true }).click();
  await expect(page.getByText("記録は無効です")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "検索", exact: true }),
  ).toHaveCount(0);
  expect(historyRequests).toBe(0);
});
