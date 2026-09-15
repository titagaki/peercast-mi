import { siteURL } from "./site-path";
export type SiteUser = { id: string; name: string };
export type SiteSession = {
  user: SiteUser | null;
  csrf?: string;
  admin?: boolean;
  devLogin?: boolean;
};
export type SiteChannel = {
  id: string;
  name: string;
  genre: string;
  description: string;
  comment?: string;
  uptime?: number;
  contactUrl: string;
  contentType: string;
  bitrate?: number;
  receiving: boolean;
  listeners: number;
  yellowPage?: string;
  playable?: boolean;
};
export type SiteDirectory = {
  channels: SiteChannel[];
  sources: {
    name: string;
    configured: boolean;
    error?: string;
    stale: boolean;
    updatedAt?: string;
  }[];
};
export type BroadcastSettings = {
  name: string;
  genre: string;
  description: string;
  comment: string;
  contactUrl: string;
};
export type BroadcastHistoryEntry = BroadcastSettings & { createdAt: string };
export type OwnBroadcast = {
  history: BroadcastHistoryEntry[];
  streamKey: string;
  rtmpUrl: string;
  channel: SiteChannel | null;
};

export async function siteAPI<T>(
  path: string,
  options: {
    method?: string;
    csrf?: string;
    body?: unknown;
    signal?: AbortSignal;
  } = {},
): Promise<T> {
  const response = await fetch(siteURL(`/site/api/${path}`), {
    method: options.method ?? "GET",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(options.csrf ? { "X-CSRF-Token": options.csrf } : {}),
    },
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
    signal: options.signal
      ? AbortSignal.any([options.signal, AbortSignal.timeout(15000)])
      : AbortSignal.timeout(15000),
  }).catch((error: unknown) => {
    if (options.signal?.aborted) throw error;
    throw new Error(
      "サイト API に接続できないか、応答がタイムアウトしました。Go のサイト機能と開発サーバーの接続設定を確認してください。操作後の場合は反映状況を更新して確認してください。",
    );
  });
  const text = await response.text();
  if (
    response.headers.get("Content-Type")?.includes("text/html") ||
    /^\s*<(?:!doctype|html)/i.test(text)
  ) {
    throw new Error(
      "サイト API から JSON ではなく HTML が返されました。開発サーバーを再起動し、Go の site.enabled と PEERCAST_SITE_TARGET の接続先を確認してください。",
    );
  }
  if (!response.ok)
    throw new Error(
      response.status === 401
        ? "X ログインの期限が切れました。ログインし直してください。"
        : text.slice(0, 500) ||
            `サイト API がエラーを返しました（HTTP ${response.status}）。`,
    );
  try {
    return JSON.parse(text) as T;
  } catch {
    throw new Error(
      "サイト API の応答形式が不正です。Go のサイトサーバーへの接続設定を確認してください。",
    );
  }
}
