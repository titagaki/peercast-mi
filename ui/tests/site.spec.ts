import { test, expect } from "@playwright/test";
import { readFileSync } from "node:fs";
import { channelExplanation } from "../src/site-channel-text";

test("local development login works without X and remains visibly marked", async ({
  page,
}) => {
  let loggedIn = false;
  await page.route("**/site/api/**", async (r) => {
    const path = new URL(r.request().url()).pathname;
    if (path.endsWith("/dev-login")) {
      expect(r.request().method()).toBe("POST");
      loggedIn = true;
      return r.fulfill({ json: { ok: true } });
    }
    if (path.endsWith("/me"))
      return r.fulfill({
        json: {
          devLogin: true,
          user: loggedIn
            ? { id: "dev-local", name: "ローカル開発ユーザー" }
            : null,
          csrf: loggedIn ? "csrf" : undefined,
        },
      });
    if (path.endsWith("/directory"))
      return r.fulfill({ json: { channels: [], sources: [] } });
    if (path.endsWith("/logout")) {
      expect(r.request().headers()["x-csrf-token"]).toBe("csrf");
      loggedIn = false;
      return r.fulfill({ json: { ok: true } });
    }
    return r.abort();
  });
  await page.goto("/");
  await expect(page.getByRole("link", { name: "X でログイン" })).toHaveCount(0);
  await page.getByRole("button", { name: "開発用ユーザーでログイン" }).click();
  await page.getByRole("button", { name: "メニュー", exact: true }).click();
  await expect(page.getByText("ローカル開発ユーザー さん")).toBeVisible();
  await expect(page.getByText(/ローカル開発モード：/)).toBeVisible();
  await page.getByRole("button", { name: "ログアウト", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "開発用ユーザーでログイン" }),
  ).toBeVisible();
});

test("HTML API response gives setup guidance and can recover", async ({
  page,
}) => {
  let ready = false;
  await page.route("**/site/api/me", (r) =>
    r.fulfill(
      ready
        ? { json: { user: null } }
        : {
            contentType: "text/html",
            body: "<!doctype html><html>Vite fallback</html>",
          },
    ),
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toContainText("JSON ではなく HTML");
  await expect(page.getByRole("alert")).not.toContainText("Unexpected token");
  await expect(page.getByRole("link", { name: "X でログイン" })).toHaveCount(0);
  ready = true;
  await page.getByRole("button", { name: "ログイン状態を更新" }).click();
  await expect(page.getByRole("link", { name: "X でログイン" })).toBeVisible();
});

test("unavailable site API shows connection instructions", async ({ page }) => {
  await page.route("**/site/api/me", (r) =>
    r.fulfill({
      status: 503,
      contentType: "text/plain",
      body: "サイト API に接続できません。Go の site.enabled を確認してください。",
    }),
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toContainText("site.enabled");
  await expect(
    page.getByRole("button", { name: "ログイン状態を更新" }),
  ).toBeEnabled();
});

const ch = {
  id: "01234567890123456789012345678901",
  name: "Music Live",
  genre: "Music",
  description: "Live session",
  comment: "リクエスト歓迎",
  uptime: 5400,
  contentType: "FLV",
  receiving: false,
  yellowPage: "SP",
  playable: true,
  listeners: -1,
  contactUrl: "https://bbs.jpnkn.com/board/",
};

test("YP control genre syntax is presentation-only and preserves ordinary genres", () => {
  const cases = [
    ["sp@@", "真・女神転生Ⅴ Vengeance"],
    ["sp@@ - ", "真・女神転生Ⅴ Vengeance"],
    ["sp@@ゲーム", "ゲーム - 真・女神転生Ⅴ Vengeance"],
    ["ypABC:?@@ゲーム", "ゲーム - 真・女神転生Ⅴ Vengeance"],
    ["tp?@@@音楽", "音楽 - 真・女神転生Ⅴ Vengeance"],
    ["ypゲーム", "ゲーム - 真・女神転生Ⅴ Vengeance"],
    ["sp?", "真・女神転生Ⅴ Vengeance"],
    ["sports", "sports - 真・女神転生Ⅴ Vengeance"],
    ["space", "space - 真・女神転生Ⅴ Vengeance"],
    ["Endgame", "Endgame - 真・女神転生Ⅴ Vengeance"],
    ["RPG: 音楽", "RPG: 音楽 - 真・女神転生Ⅴ Vengeance"],
    ["音楽 sp@@", "音楽 sp@@ - 真・女神転生Ⅴ Vengeance"],
    ["@@音楽", "@@音楽 - 真・女神転生Ⅴ Vengeance"],
  ];
  for (const [genre, expected] of cases) {
    const channel = {
      ...ch,
      genre,
      description: "真・女神転生Ⅴ Vengeance",
      comment: "",
    };
    expect(channelExplanation(channel)).toBe(expected);
    expect(channel.genre).toBe(genre);
  }
});

test("YP controls are hidden in both channel cards and watch details", async ({
  page,
}) => {
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" } } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({
      json: {
        channels: [
          {
            ...ch,
            genre: "sp@@",
            description: "真・女神転生Ⅴ Vengeance",
            comment: "",
            contentType: "RAW",
          },
        ],
        sources: [],
      },
    }),
  );
  await page.route("**/site/api/channels/*/comments*", (r) =>
    r.fulfill({ json: board }),
  );
  await page.goto("/");
  await expect(page.locator(".site-channel-description")).toHaveText(
    "真・女神転生Ⅴ Vengeance",
  );
  await page.getByRole("searchbox").fill("sp@@");
  await expect(page.locator(".site-channel-card")).toHaveCount(0);
  await page.getByRole("searchbox").fill("女神転生");
  await page.getByRole("link", { name: ch.name, exact: true }).click();
  await expect(page.locator(".site-channel-description")).toHaveText(
    "真・女神転生Ⅴ Vengeance",
  );
});
const board = {
  supported: true,
  threadId: "123",
  threadTitle: "配信スレ",
  commentCount: 1,
  threads: [
    { id: "123", title: "配信スレ", comments: 1 },
    { id: "456", title: "次スレ", comments: 1 },
  ],
  comments: [
    { no: 1, name: "名無し", date: "2026/09/14", body: "こんにちは\nコメント" },
  ],
};

test("root is a channel list with card links and no operator status", async ({
  page,
}) => {
  let streams = 0;
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" }, csrf: "csrf" } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({
      json: {
        channels: [ch],
        sources: [
          { name: "0yp", configured: true, stale: false },
          {
            name: "p@YP",
            configured: false,
            stale: false,
            error: "channels_url",
          },
        ],
      },
    }),
  );
  await page.route("**/site/stream/*", (r) => {
    streams++;
    return r.abort();
  });
  await page.goto("/");
  await expect(
    page.getByRole("heading", { name: "チャンネル一覧" }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", { name: "Music Live", exact: true }),
  ).toHaveAttribute("href", "/channels/" + ch.id);
  await expect(
    page.getByText(
      /一覧取得済み|channels_url|公開 PCP 中継 \/ 認証付きサイト視聴/,
    ),
  ).toHaveCount(0);
  await expect(
    page.getByRole("button", { name: "視聴する", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("link", { name: "配信する", exact: true }),
  ).toHaveCount(0);
  const menu = page.getByRole("button", { name: "メニュー", exact: true });
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  await expect(page.getByRole("link", { name: "管理パネル" })).toHaveAttribute(
    "href",
    "/admin",
  );
  await menu.click();
  await expect(
    page.getByRole("link", { name: "配信する", exact: true }),
  ).toHaveAttribute("href", "/broadcast");
  await page.keyboard.press("Escape");
  await expect(menu).toBeFocused();
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  await menu.click();
  await page.getByRole("heading", { name: "チャンネル一覧" }).click();
  await expect(menu).toHaveAttribute("aria-expanded", "false");
  await page.setViewportSize({ width: 360, height: 740 });
  await menu.click();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= innerWidth,
    ),
  ).toBe(true);
  await expect(
    page.getByRole("button", { name: "ストリームキー", exact: true }),
  ).toHaveCount(0);
  expect(streams).toBe(0);
});

test("whole channel card supports icon, description, padding and keyboard navigation", async ({
  page,
}) => {
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" } } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({
      json: { channels: [{ ...ch, contentType: "RAW" }], sources: [] },
    }),
  );
  await page.route("**/site/api/channels/*/comments*", (r) =>
    r.fulfill({ json: board }),
  );
  for (const target of ["icon", "description", "padding", "keyboard"]) {
    await page.goto("/");
    const card = page.getByRole("link", { name: ch.name, exact: true });
    await expect(card).toHaveClass(/site-channel-card/);
    await expect(card.locator("a, button")).toHaveCount(0);
    if (target === "icon") await card.locator("img").click();
    else if (target === "description")
      await card.locator(".site-channel-description").click();
    else if (target === "padding")
      await card.click({ position: { x: 5, y: 5 } });
    else {
      await page.getByRole("searchbox").focus();
      await page.keyboard.press("Tab");
      await expect(card).toBeFocused();
      await page.keyboard.press("Enter");
    }
    await expect(page).toHaveURL(`/channels/${ch.id}`);
    await expect(
      page.getByRole("heading", { name: ch.name, exact: true }),
    ).toBeVisible();
  }
});

test("white site groups channel text and serves local YP icons", async ({
  page,
}, testInfo) => {
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" }, csrf: "csrf" } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({
      json: {
        channels: [
          {
            ...ch,
            listeners: 12,
            genre: "game Music",
            description: "Live session - <Open>",
          },
          {
            ...ch,
            id: "2".repeat(32),
            name: "別の配信",
            yellowPage: "0yp",
            genre: "Endgame",
            description: "<img src=x onerror=alert(1)>",
          },
        ],
        sources: [],
      },
    }),
  );
  await page.goto("/");
  await expect(page.locator(".site-app")).toHaveCSS(
    "background-color",
    "rgb(255, 255, 255)",
  );
  await expect(page.locator("body")).toHaveCSS(
    "background-color",
    "rgb(255, 255, 255)",
  );
  await expect(
    page.getByText("Music - Live session リクエスト歓迎", { exact: true }),
  ).toBeVisible();
  await expect(page.getByText("12人が視聴中")).toBeVisible();
  await expect(page.getByText("1時間30分前から配信")).toHaveCount(2);
  await expect(page.getByText(/Endgame - <img/)).toBeVisible();
  await expect(page.locator(".site-channel-copy img")).toHaveCount(0);
  const icons = page.locator(".site-channel-icon");
  await expect(icons).toHaveCount(2);
  await expect
    .poll(() =>
      icons.evaluateAll((images) =>
        images.every((image) => (image as HTMLImageElement).naturalWidth > 0),
      ),
    )
    .toBe(true);
  await expect(icons.first()).toHaveAttribute("src", /yp-sp/);
  await expect(icons.last()).toHaveAttribute("src", /mouneyou/);
  await page.screenshot({
    path: testInfo.outputPath("site-white-list.png"),
    fullPage: true,
  });
  await page.getByRole("searchbox").fill("リクエスト歓迎");
  await expect(page.locator(".site-channel-info")).toHaveCount(2);
});

test("generated FLV autoplays on channel navigation and direct URL with mute fallback", async ({
  page,
}, testInfo) => {
  test.skip(
    !process.env.PEERCAST_TEST_FLV,
    "Set PEERCAST_TEST_FLV to a generated FLV fixture",
  );
  let streams = 0;
  await page.addInitScript(() => {
    const play = HTMLMediaElement.prototype.play;
    HTMLMediaElement.prototype.play = function () {
      if (!this.muted)
        return Promise.reject(
          new DOMException("Autoplay blocked", "NotAllowedError"),
        );
      return play.call(this);
    };
  });
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" }, csrf: "csrf" } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({ json: { channels: [ch], sources: [] } }),
  );
  await page.route("**/site/api/channels/*/comments*", (r) =>
    r.fulfill({ json: board }),
  );
  await page.route("**/site/stream/*", (r) => {
    streams++;
    return r.fulfill({
      contentType: "video/x-flv",
      body: readFileSync(process.env.PEERCAST_TEST_FLV!),
    });
  });
  await page.goto("/");
  await page.getByRole("link", { name: "Music Live", exact: true }).click();
  await expect(page).toHaveURL("/channels/" + ch.id);
  await expect
    .poll(() =>
      page.locator("video").evaluate((v) => (v as HTMLVideoElement).videoWidth),
    )
    .toBe(160);
  await expect
    .poll(() =>
      page.locator("video").evaluate((v) => (v as HTMLVideoElement).paused),
    )
    .toBe(false);
  expect(
    await page.locator("video").evaluate((v) => (v as HTMLVideoElement).muted),
  ).toBe(true);
  await page.locator("video").evaluate((v) => (v as HTMLVideoElement).pause());
  expect(
    await page.locator("video").evaluate((v) => (v as HTMLVideoElement).paused),
  ).toBe(true);
  await expect(
    page.getByRole("button", {
      name: /^(再生を開始|全画面|ミニプレイヤー|視聴する|視聴を閉じる)$/,
    }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("region", { name: "掲示板コメント" }),
  ).toContainText("こんにちは");
  expect(streams).toBeGreaterThan(0);
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("site-watch-comments.png"),
    fullPage: true,
  });
  await page.reload();
  await expect
    .poll(() =>
      page.locator("video").evaluate((v) => (v as HTMLVideoElement).videoWidth),
    )
    .toBe(160);
});

test("thread selection updates comments without navigation and safely displays text", async ({
  page,
}) => {
  let reads = 0;
  await page.clock.install();
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: { id: "1", name: "Alice" }, csrf: "csrf" } }),
  );
  await page.route("**/site/api/directory", (r) =>
    r.fulfill({
      json: { channels: [{ ...ch, contentType: "RAW" }], sources: [] },
    }),
  );
  await page.route("**/site/api/channels/*/comments*", (r) => {
    reads++;
    const thread = new URL(r.request().url()).searchParams.get("thread") ?? "";
    return r.fulfill({
      json: {
        ...board,
        threadId: thread,
        comments: thread
          ? [
              {
                no: 2,
                name: "名無し",
                date: "date",
                body: "<img src=x onerror=alert(1)> 次のコメント",
              },
            ]
          : [],
      },
    });
  });
  await page.goto("/channels/" + ch.id);
  await page
    .getByRole("combobox", { name: "スレッド", exact: true })
    .selectOption("456");
  await expect(page.getByLabel("コメント一覧")).toContainText(
    "<img src=x onerror=alert(1)> 次のコメント",
  );
  await expect(page.locator(".site-comment-list img")).toHaveCount(0);
  await expect(page).toHaveURL("/channels/" + ch.id);
  expect(reads).toBeGreaterThanOrEqual(2);
  const before = reads;
  await page.clock.runFor(11000);
  await expect.poll(() => reads).toBeGreaterThan(before);
});

test("anonymous direct channel URL preserves login destination", async ({
  page,
}) => {
  let streams = 0;
  await page.route("**/site/api/me", (r) =>
    r.fulfill({ json: { user: null } }),
  );
  await page.route("**/site/stream/*", (r) => {
    streams++;
    return r.abort();
  });
  await page.goto("/channels/" + ch.id);
  await expect(
    page.getByRole("link", { name: "X でログイン" }),
  ).toHaveAttribute(
    "href",
    "/auth/x/start?next=" + encodeURIComponent("/channels/" + ch.id),
  );
  expect(streams).toBe(0);
});

test("broadcast page retains owner-only key and broadcast workflow", async ({
  page,
}) => {
  let loggedIn = true,
    key = "",
    created = false;
  await page.route("**/site/api/**", async (route) => {
    const path = new URL(route.request().url()).pathname,
      method = route.request().method();
    if (method !== "GET")
      expect(route.request().headers()["x-csrf-token"]).toBe("csrf");
    if (path.endsWith("/me"))
      return route.fulfill({
        json: loggedIn
          ? { user: { id: "1", name: "Alice" }, csrf: "csrf" }
          : { user: null },
      });
    if (path.endsWith("/directory"))
      return route.fulfill({ json: { channels: [ch], sources: [] } });
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
          genre: "Music",
          description: "Live session",
          comment: "リクエスト歓迎",
          contactUrl: "https://bbs.jpnkn.com/board/",
          bitrate: 0,
        });
        created = true;
      }
      if (method === "DELETE") created = false;
      return route.fulfill({
        json: {
          streamKey: key,
          rtmpUrl: "rtmps://live.example/live",
          channel: created
            ? {
                ...ch,
                name: "My live",
                description: "Live session",
                comment: "リクエスト歓迎",
                bitrate: 3000,
              }
            : null,
        },
      });
    }
    return route.abort();
  });
  await page.goto("/");
  await page.getByRole("button", { name: "メニュー", exact: true }).click();
  await page.getByRole("link", { name: "配信する", exact: true }).click();
  await expect(page).toHaveURL("/broadcast");
  await page
    .getByRole("button", { name: "配信キーを発行", exact: true })
    .click();
  await expect(page.getByText("my-secret-key", { exact: true })).toHaveCount(0);
  await page
    .getByRole("textbox", { name: "配信名", exact: true })
    .fill("My live");
  await page
    .getByRole("textbox", { name: "ジャンル", exact: true })
    .fill("Music");
  await page
    .getByRole("textbox", { name: "説明", exact: true })
    .fill("Live session");
  await page
    .getByRole("textbox", { name: "コメント", exact: true })
    .fill("リクエスト歓迎");
  await page
    .getByRole("textbox", { name: "コンタクトURL", exact: true })
    .fill("https://bbs.jpnkn.com/board/");
  await expect(page.getByRole("spinbutton", { name: "ビットレート (kbps)" })).toHaveCount(0);
  await page.getByRole("button", { name: "配信枠を作成", exact: true }).click();
  await expect(page.getByRole("heading", { name: "My live" })).toBeVisible();
  await expect(
    page.getByRole("status").filter({ hasText: "OBS接続待ち" }),
  ).toBeVisible();
  await expect(page.getByText("3000 kbps", { exact: true })).toBeVisible();
  page.once("dialog", (d) => d.dismiss());
  await page.getByRole("button", { name: "自分の配信を停止" }).click();
  await expect(page.getByRole("heading", { name: "My live" })).toBeVisible();
  page.on("dialog", (d) => d.accept());
  await page.getByRole("button", { name: "自分の配信を停止" }).click();
  await expect(
    page.getByRole("textbox", { name: "配信名", exact: true }),
  ).toHaveValue("My live");
  await expect(
    page.getByRole("textbox", { name: "コメント", exact: true }),
  ).toHaveValue("リクエスト歓迎");
  await expect(
    page.getByRole("textbox", { name: "コンタクトURL", exact: true }),
  ).toHaveValue("https://bbs.jpnkn.com/board/");
  await expect(
    page.getByRole("spinbutton", { name: "ビットレート (kbps)" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("textbox", { name: "配信名", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "メニュー", exact: true }).click();
  await page.getByRole("button", { name: "ログアウト", exact: true }).click();
  await expect(page.getByRole("link", { name: "X でログイン" })).toBeVisible();
});
