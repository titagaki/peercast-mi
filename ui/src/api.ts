import { siteURL } from "./site-path";
// Minimal JSON-RPC 2.0 client for peercast-mi.
//
// Vite uses a different origin. The server allows loopback origins by default;
// other UI origins must be explicitly allowed in the backend configuration.

let adminCSRF = "";
export const setAdminCSRF = (token: string) => {
  adminCSRF = token;
};

const SITE_ENDPOINT = siteURL("/admin/api/1");
const ENDPOINT =
  (import.meta.env.VITE_PEERCAST_ENDPOINT as string | undefined)?.trim() ||
  (import.meta.env.DEV ? "http://127.0.0.1:7144/api/1" : SITE_ENDPOINT);

export const usesSiteAdmin = ENDPOINT === SITE_ENDPOINT;

export class RpcError extends Error {
  code: number;
  constructor(code: number, message: string) {
    super(message);
    this.code = code;
  }
}

let nextId = 1;

export async function rpc<T = unknown>(
  method: string,
  params: unknown[] = [],
  signal?: AbortSignal,
): Promise<T> {
  const id = nextId++;
  const timeout = AbortSignal.timeout(15000);
  const res = await fetch(ENDPOINT, {
    method: "POST",
    credentials: "same-origin",
    headers: {
      "Content-Type": "application/json",
      ...(ENDPOINT === SITE_ENDPOINT ? { "X-CSRF-Token": adminCSRF } : {}),
    },
    body: JSON.stringify({ jsonrpc: "2.0", method, params, id }),
    signal: signal ? AbortSignal.any([signal, timeout]) : timeout,
  });
  if (!res.ok) {
    throw new Error(`HTTP ${res.status} ${res.statusText}`);
  }
  const body = await res.json();
  if (
    !body ||
    body.jsonrpc !== "2.0" ||
    body.id !== id ||
    (!Object.hasOwn(body, "result") && !body.error)
  ) {
    throw new Error("サーバーから不正な応答を受信しました。");
  }
  if (body.error) {
    throw new RpcError(body.error.code, body.error.message);
  }
  return body.result as T;
}

// ---------------------------------------------------------------------------
// Typed wrappers — one per method the UI uses.
// ---------------------------------------------------------------------------

export type StreamKeyEntry = {
  accountName: string;
  streamKey: string;
};

export type ChannelInfo = {
  name: string;
  url: string;
  genre: string;
  desc: string;
  comment: string;
  bitrate: number;
  contentType: string;
  mimeType: string;
};

export type TrackInfo = {
  title: string;
  genre: string;
  album: string;
  creator: string;
  url: string;
};

export type ChannelStatus = {
  status: string;
  source: string;
  uptime: number;
  localRelays: number;
  localDirects: number;
  totalRelays: number;
  totalDirects: number;
  isBroadcasting: boolean;
  isRelayFull: boolean;
  isDirectFull: boolean;
  isReceiving: boolean;
};

export type ChannelEntry = {
  channelId: string;
  status: ChannelStatus;
  info: ChannelInfo;
  track: TrackInfo;
};

export const listStreamKeys = (signal?: AbortSignal) =>
  rpc<StreamKeyEntry[]>("listStreamKeys", [], signal);
export const issueStreamKey = (accountName: string, streamKey: string) =>
  rpc<null>("issueStreamKey", [accountName, streamKey]);
export const revokeStreamKey = (accountName: string) =>
  rpc<null>("revokeStreamKey", [accountName]);
export type ChannelConnection = {
  connectionId: number;
  type: string; // "source" | "relay" | "direct"
  status: string;
  sendRate: number;
  recvRate: number;
  protocolName: string;
  remoteEndPoint: string | null;
};

export type BroadcastParam = {
  streamKey: string;
  info: {
    name: string;
    genre?: string;
    url?: string;
    desc?: string;
    comment?: string;
    bitrate?: number;
  };
  track?: {
    title?: string;
    creator?: string;
    album?: string;
    url?: string;
  };
};

export const broadcastChannel = (param: BroadcastParam) =>
  rpc<{ channelId: string }>("broadcastChannel", [param]);

export type RelayTreeNode = {
  sessionId: string;
  address: string;
  port: number;
  isFirewalled: boolean;
  localRelays: number;
  localDirects: number;
  isTracker: boolean;
  isRelayFull: boolean;
  isDirectFull: boolean;
  isReceiving: boolean;
  isControlFull: boolean;
  version: number;
  versionString: string;
  children: RelayTreeNode[];
};

export type VersionInfo = {
  agentName: string;
};

export type Settings = {
  serverPort: number;
  rtmpPort: number;
};

export type YellowPage = {
  yellowPageId: number;
  name: string;
  uri: string;
  announceUri: string;
  channelCount: number;
};

export const getVersionInfo = (signal?: AbortSignal) =>
  rpc<VersionInfo>("getVersionInfo", [], signal);
export const getSettings = (signal?: AbortSignal) =>
  rpc<Settings>("getSettings", [], signal);
export const getYellowPages = (signal?: AbortSignal) =>
  rpc<YellowPage[]>("getYellowPages", [], signal);
export const getChannels = (signal?: AbortSignal) =>
  rpc<ChannelEntry[]>("getChannels", [], signal);
export const getChannelInfo = (channelId: string) =>
  rpc<{ info: ChannelInfo; track: TrackInfo }>("getChannelInfo", [channelId]);
export const getChannelRelayTree = (channelId: string, signal?: AbortSignal) =>
  rpc<RelayTreeNode[]>("getChannelRelayTree", [channelId], signal);
export const stopChannel = (channelId: string) =>
  rpc<null>("stopChannel", [channelId]);
export const bumpChannel = (channelId: string) =>
  rpc<null>("bumpChannel", [channelId]);
export const getChannelConnections = (
  channelId: string,
  signal?: AbortSignal,
) => rpc<ChannelConnection[]>("getChannelConnections", [channelId], signal);
export const stopChannelConnection = (
  channelId: string,
  connectionId: number,
) => rpc<boolean>("stopChannelConnection", [channelId, connectionId]);

export type SetChannelInfoParam = {
  info: {
    name: string;
    genre?: string;
    url?: string;
    desc?: string;
    comment?: string;
    bitrate?: number;
  };
  track?: {
    title?: string;
    creator?: string;
    album?: string;
    url?: string;
  };
};

export const setChannelInfo = async (
  channelId: string,
  param: SetChannelInfoParam,
) => {
  // The backend replaces track, even when {} is passed. Preserve a fresh
  // snapshot when editing only channel info; never silently erase the track.
  const track = param.track ?? (await getChannelInfo(channelId)).track;
  return rpc<null>("setChannelInfo", [channelId, param.info, track]);
};
