import { expect, test, type Page } from "@playwright/test";

const track = {
  title: "Keep this title",
  creator: "DJ",
  album: "Album",
  url: "https://example.test/track",
  genre: "",
};
const channel = (id: string, name: string, broadcast: boolean) => ({
  channelId: id,
  info: {
    name,
    genre: "Music",
    desc: "配信テスト",
    comment: "Hello",
    url: "https://example.test",
    bitrate: 1500,
    contentType: "FLV",
    mimeType: "video/x-flv",
  },
  track,
  status: {
    isBroadcasting: broadcast,
    isReceiving: true,
    status: "Receiving",
    source: broadcast
      ? "rtmp://127.0.0.1/live/sk_secret"
      : "[2001:db8::1]:7144",
    uptime: 1234,
    localDirects: 2,
    totalDirects: 4,
    localRelays: 1,
    totalRelays: 3,
    isRelayFull: false,
    isDirectFull: false,
  },
});
const entries = [
  channel("a", "Morning Radio", true),
  channel("b", "Night Relay", false),
];
const tree = [
  {
    sessionId: "node",
    address: "2001:db8::1",
    port: 7144,
    localDirects: 2,
    localRelays: 1,
    isTracker: true,
    isReceiving: true,
    isRelayFull: false,
    isDirectFull: false,
    isFirewalled: false,
    isControlFull: false,
    version: 1218,
    versionString: "PeerCast-MI",
    children: [],
  },
];
type Request = { method: string; params: unknown[]; id: number };
type Reply = {
  result?: unknown;
  error?: { code: number; message: string };
  delay?: number;
};

async function mockAPI(
  page: Page,
  override?: (request: Request) => Reply | undefined,
) {
  const calls: Request[] = [];
  await page.route("**/api/1", async (route) => {
    const request = route.request().postDataJSON() as Request;
    calls.push(request);
    const defaults: Record<string, unknown> = {
      getChannels: entries,
      getChannelInfo: {
        info: entries[0].info,
        track: { ...track, title: "Latest track" },
      },
      listStreamKeys: [{ accountName: "radio", streamKey: "sk_secret" }],
      getChannelConnections: [
        {
          connectionId: 7,
          type: "relay",
          protocolName: "PCP",
          status: "Connected",
          remoteEndPoint:
            request.params[0] === "a" ? "a.example:7144" : "b.example:7144",
          sendRate: 128000,
          recvRate: 0,
        },
      ],
      getChannelRelayTree: tree,
      getVersionInfo: { agentName: "PeerCast-MI/0.1.0" },
      getSettings: { serverPort: 7144, rtmpPort: 1935 },
      getYellowPages: [
        {
          yellowPageId: 1,
          name: "Test YP",
          uri: "pcp://yp.example:7144",
          channelCount: 1,
        },
      ],
      stopChannelConnection: true,
    };
    const reply = override?.(request) ?? {
      result: defaults[request.method] ?? null,
    };
    if (reply.delay)
      await new Promise((resolve) => setTimeout(resolve, reply.delay));
    await route.fulfill({
      json: {
        jsonrpc: "2.0",
        id: request.id,
        ...(reply.error
          ? { error: reply.error }
          : { result: reply.result ?? null }),
      },
    });
  });
  return calls;
}

test("info editing preserves the latest track", async ({ page }) => {
  const calls = await mockAPI(page);
  await page.goto("/");
  await page
    .getByRole("button", { name: "Morning Radio", exact: true })
    .click();
  await page.getByRole("button", { name: "情報を編集" }).click();
  await page.getByLabel("チャンネル名").fill("Updated radio");
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const write = calls.find((call) => call.method === "setChannelInfo");
  expect(write?.params[0]).toBe("a");
  expect(write?.params[1]).toMatchObject({ name: "Updated radio" });
  expect(write?.params[2]).toEqual({ ...track, title: "Latest track" });
});

test("failed track read does not submit a destructive empty track", async ({
  page,
}) => {
  const calls = await mockAPI(page, (request) =>
    request.method === "getChannelInfo"
      ? { error: { code: -32603, message: "track unavailable" } }
      : undefined,
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Morning Radio", exact: true })
    .click();
  await page.getByRole("button", { name: "情報を編集" }).click();
  await page.getByRole("button", { name: "保存", exact: true }).click();
  await expect(page.getByRole("alert")).toContainText("track unavailable");
  expect(calls.some((call) => call.method === "setChannelInfo")).toBe(false);
});

test("channel switch discards old detail responses and disconnects correct channel", async ({
  page,
}) => {
  const calls = await mockAPI(page, (request) =>
    request.method === "getChannelConnections" && request.params[0] === "a"
      ? {
          result: [
            {
              connectionId: 99,
              type: "relay",
              remoteEndPoint: "old-a.example",
              sendRate: 0,
            },
          ],
          delay: 600,
        }
      : undefined,
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Morning Radio", exact: true })
    .click();
  await expect
    .poll(() =>
      calls.some(
        (call) =>
          call.method === "getChannelConnections" && call.params[0] === "a",
      ),
    )
    .toBe(true);
  await page.getByRole("button", { name: "Night Relay", exact: true }).click();
  await expect(page.getByText("b.example:7144", { exact: true })).toBeVisible();
  await page.waitForTimeout(750); // Explicitly release the delayed, obsolete response.
  await expect(page.getByText("old-a.example", { exact: true })).toHaveCount(0);
  page.on("dialog", (dialog) => dialog.accept());
  await page.getByRole("button", { name: "切断", exact: true }).click();
  await expect
    .poll(
      () =>
        calls.find((call) => call.method === "stopChannelConnection")?.params,
    )
    .toEqual(["b", 7]);
});

test("broadcast key failure is visible, retryable and blocks submission", async ({
  page,
}) => {
  let fail = true;
  const calls = await mockAPI(page, (request) =>
    request.method === "listStreamKeys" && fail
      ? { error: { code: -32603, message: "keys unavailable" } }
      : undefined,
  );
  await page.goto("/");
  await page.getByRole("button", { name: "＋ 配信を開始" }).click();
  const dialog = page.getByRole("dialog");
  await expect(dialog.getByRole("alert")).toContainText("keys unavailable");
  await expect(
    dialog.getByRole("button", { name: "配信を開始", exact: true }),
  ).toBeDisabled();
  fail = false;
  await dialog.getByRole("button", { name: "キーを再読み込み" }).click();
  await dialog.getByLabel("ストリームキー").selectOption("sk_secret");
  await dialog.getByLabel("チャンネル名").fill("New radio");
  await dialog.getByRole("button", { name: "配信を開始", exact: true }).click();
  await expect(dialog).toHaveCount(0);
  expect(
    calls.find((call) => call.method === "broadcastChannel")?.params[0],
  ).toMatchObject({ streamKey: "sk_secret", info: { name: "New radio" } });
});

test("empty keys explain the prerequisite", async ({ page }) => {
  await mockAPI(page, (request) =>
    request.method === "listStreamKeys" ? { result: [] } : undefined,
  );
  await page.goto("/");
  await page.getByRole("button", { name: "＋ 配信を開始" }).click();
  await expect(
    page.getByText("キーがありません。", { exact: false }),
  ).toBeVisible();
  await expect(
    page
      .getByRole("dialog")
      .getByRole("button", { name: "配信を開始", exact: true }),
  ).toBeDisabled();
});

test("issuing a key prevents duplicate submissions", async ({ page }) => {
  const calls = await mockAPI(page, (request) =>
    request.method === "issueStreamKey"
      ? { result: null, delay: 500 }
      : undefined,
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "ストリームキー", exact: true })
    .click();
  await page.getByLabel("アカウント名").fill("new-account");
  await page.locator("form").evaluate((form) => {
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
    form.dispatchEvent(
      new Event("submit", { bubbles: true, cancelable: true }),
    );
  });
  await expect(page.getByRole("button", { name: "処理中…" })).toBeDisabled();
  await expect(
    page.getByText("ストリームキーを発行しました。", { exact: true }),
  ).toBeVisible();
  expect(calls.filter((call) => call.method === "issueStreamKey")).toHaveLength(
    1,
  );
});

test("read failures are not displayed as empty success and can retry", async ({
  page,
}) => {
  let fail = true;
  await mockAPI(page, (request) =>
    request.method === "getChannels" && fail
      ? { error: { code: -32603, message: "offline" } }
      : undefined,
  );
  await page.goto("/");
  await expect(page.getByRole("alert")).toContainText("offline");
  await expect(
    page.getByText("チャンネルはありません。", { exact: false }),
  ).toHaveCount(0);
  fail = false;
  await page.getByRole("button", { name: "更新", exact: true }).click();
  await expect(
    page.getByRole("button", { name: "Morning Radio", exact: true }),
  ).toBeVisible();
  await expect(page.getByRole("alert")).toHaveCount(0);
});

test("keyboard selection, dialog escape and contextual bump labels", async ({
  page,
}) => {
  const calls = await mockAPI(page);
  await page.goto("/");
  const channelButton = page.getByRole("button", {
    name: "Morning Radio",
    exact: true,
  });
  await channelButton.focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("region", { name: "選択チャンネルの詳細" }),
  ).toBeVisible();
  await expect(
    page.getByText("[2001:db8::1]:7144", { exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: "情報を編集" }).click();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toHaveCount(0);
  await expect(page.getByRole("button", { name: "情報を編集" })).toBeFocused();
  await page.getByRole("button", { name: "再接続", exact: true }).click();
  await expect
    .poll(() => calls.find((call) => call.method === "bumpChannel")?.params)
    .toEqual(["b"]);
  await expect(
    page.getByRole("button", { name: "YP 再通知", exact: true }),
  ).toBeVisible();
});

test("responsive screens, secret masking and node information", async ({
  page,
}, testInfo) => {
  await mockAPI(page);
  await page.goto("/");
  await page
    .getByRole("button", { name: "Morning Radio", exact: true })
    .click();
  await expect(page.getByText("a.example:7144", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.screenshot({
    path: testInfo.outputPath("channels.png"),
    fullPage: true,
  });
  await page.getByRole("button", { name: "情報を編集" }).click();
  const box = await page.getByRole("dialog").boundingBox();
  expect(box!.x).toBeGreaterThanOrEqual(0);
  expect(box!.x + box!.width).toBeLessThanOrEqual(page.viewportSize()!.width);
  await page.screenshot({
    path: testInfo.outputPath("dialog.png"),
    fullPage: false,
  });
  await page.keyboard.press("Escape");
  await page
    .getByRole("button", { name: "ストリームキー", exact: true })
    .click();
  await expect(page.getByText("radio", { exact: true })).toBeVisible();
  await expect(page.getByText("sk_secret", { exact: true })).toHaveCount(0);
  await page.getByRole("button", { name: "表示", exact: true }).click();
  await expect(page.getByText("sk_secret", { exact: true })).toBeVisible();
  expect(
    await page.evaluate(
      () => document.documentElement.scrollWidth <= window.innerWidth,
    ),
  ).toBe(true);
  await page.getByRole("button", { name: "ノード情報", exact: true }).click();
  await expect(page.getByText("Test YP", { exact: true })).toBeVisible();
  await expect(
    page.getByText("PeerCast-MI/0.1.0", { exact: true }),
  ).toBeVisible();
});

test("pending save is locked and cannot be dismissed", async ({ page }) => {
  const calls = await mockAPI(page, (request) =>
    request.method === "setChannelInfo"
      ? { result: null, delay: 700 }
      : undefined,
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Morning Radio", exact: true })
    .click();
  await page.getByRole("button", { name: "情報を編集" }).click();
  await page
    .getByRole("dialog")
    .locator("form")
    .evaluate((form) => {
      form.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      );
      form.dispatchEvent(
        new Event("submit", { bubbles: true, cancelable: true }),
      );
    });
  await expect(page.getByRole("button", { name: "保存中…" })).toBeDisabled();
  await page.keyboard.press("Escape");
  await expect(page.getByRole("dialog")).toBeVisible();
  await expect(
    page.getByRole("button", { name: "キャンセル", exact: true }),
  ).toBeDisabled();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(calls.filter((call) => call.method === "setChannelInfo")).toHaveLength(
    1,
  );
});
