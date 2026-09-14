export type SiteUser = { id: string; name: string };
export type SiteSession = { user: SiteUser | null; csrf?: string };
export type SiteChannel = {
  id: string;
  name: string;
  genre: string;
  description: string;
  contactUrl: string;
  contentType: string;
  receiving: boolean;
  listeners: number;
};
export type OwnBroadcast = {
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
  const response = await fetch(`/site/api/${path}`, {
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
  });
  if (!response.ok)
    throw new Error(
      response.status === 401
        ? "X ログインの期限が切れました。ログインし直してください。"
        : (await response.text()).slice(0, 500),
    );
  return response.json() as Promise<T>;
}
