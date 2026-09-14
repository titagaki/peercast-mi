import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";

test("generated FLV decodes in the browser after explicit play", async ({
  page,
}, testInfo) => {
  test.skip(
    !process.env.PEERCAST_TEST_FLV,
    "Set PEERCAST_TEST_FLV to a generated H.264/AAC FLV fixture",
  );
  const ch = {
    id: "01234567890123456789012345678901",
    name: "Playback test",
    genre: "",
    description: "",
    contentType: "FLV",
    receiving: true,
    listeners: 1,
    contactUrl: "",
  };
  let streams = 0;
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Test" }, csrf: "csrf" } }),
  );
  await page.route("**/site/api/channels", (r) => r.fulfill({ json: [ch] }));
  await page.route("**/site/stream/*", (r) => {
    streams++;
    return r.fulfill({
      contentType: "video/x-flv",
      body: readFileSync(process.env.PEERCAST_TEST_FLV!),
    });
  });
  await page.goto("/watch");
  await page.getByRole("button", { name: "「Playback test」を視聴" }).click();
  expect(streams).toBe(0);
  await page.getByRole("button", { name: "再生を開始", exact: true }).click();
  await expect
    .poll(() =>
      page.locator("video").evaluate((v) => (v as HTMLVideoElement).videoWidth),
    )
    .toBe(160);
  expect(streams).toBe(1);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("site-playback.png"),
    fullPage: true,
  });
});

test("anonymous visitor sees login and no administrative controls", async ({
  page,
}) => {
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: null } }),
  );
  await page.goto("/watch");
  await expect(
    page.getByRole("link", { name: "X でログイン" }),
  ).toHaveAttribute("href", "/auth/x/start");
  await expect(
    page.getByRole("button", { name: "ストリームキー", exact: true }),
  ).toHaveCount(0);
});

test("user can find channels, close player and manage only own broadcast", async ({
  page,
}) => {
  let loggedIn = true;
  let key = "";
  let created = false;
  const ch = {
    id: "01234567890123456789012345678901",
    name: "Music Live",
    genre: "Music",
    description: "Live session",
    contentType: "FLV",
    receiving: true,
    listeners: 1,
    contactUrl: "",
  };
  await page.route("**/site/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname;
    const method = route.request().method();
    if (method !== "GET")
      expect(route.request().headers()["x-csrf-token"]).toBe("csrf");
    if (path.endsWith("/me"))
      return route.fulfill({
        json: loggedIn
          ? { user: { id: "1", name: "Alice" }, csrf: "csrf" }
          : { user: null },
      });
    if (path.endsWith("/channels")) return route.fulfill({ json: [ch] });
    if (path.endsWith("/logout")) {
      loggedIn = false;
      return route.fulfill({ json: { ok: true } });
    }
    if (path.endsWith("/key")) {
      key = "my-secret-key";
      return route.fulfill({ json: { streamKey: key } });
    }
    if (path.endsWith("/broadcast")) {
      if (method === "POST") {
        expect(route.request().postDataJSON()).toEqual({
          name: "My live",
          genre: "",
          description: "",
        });
        created = true;
      }
      if (method === "DELETE") created = false;
      return route.fulfill({
        json: {
          streamKey: key,
          rtmpUrl: "rtmps://live.example/live",
          channel: created ? { ...ch, name: "My live" } : null,
        },
      });
    }
    return route.abort();
  });
  await page.goto("/watch");
  await page.getByRole("button", { name: "「Music Live」を視聴" }).click();
  await expect(
    page.getByRole("region", { name: "視聴プレイヤー" }),
  ).toBeVisible();
  await page.getByRole("button", { name: "視聴を閉じる" }).click();
  await expect(
    page.getByRole("region", { name: "視聴プレイヤー" }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "自分の配信", exact: true }).click();
  await page
    .getByRole("button", { name: "配信キーを発行", exact: true })
    .click();
  await expect(page.getByText("my-secret-key", { exact: true })).toHaveCount(0);
  await page
    .getByRole("textbox", { name: "配信名", exact: true })
    .fill("My live");
  await page.getByRole("button", { name: "配信枠を作成", exact: true }).click();
  await expect(page.getByRole("heading", { name: "My live" })).toBeVisible();
  page.on("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "自分の配信を停止" }).click();
  await expect(
    page.getByRole("textbox", { name: "配信名", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "ログアウト", exact: true }).click();
  await expect(page.getByRole("link", { name: "X でログイン" })).toBeVisible();
});
