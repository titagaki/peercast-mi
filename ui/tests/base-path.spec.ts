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
