export type AuditPage<T> = { items: T[]; nextCursor?: string };
export type AuditActor = {
  account?: string;
  name?: string;
  ip?: string;
  session?: string;
  source: string;
};
export type AuditSettings = {
  name: string;
  inputGenre?: string;
  genre: string;
  description: string;
  comment: string;
  contactUrl: string;
  bitrate: number;
  contentType: string;
};
export type AuditBroadcast = {
  id: string;
  node?: string;
  channelId: string;
  owner?: string;
  ownerName?: string;
  actor: AuditActor;
  settings: AuditSettings;
  created: string;
  firstMedia?: string;
  lastMedia?: string;
  ended?: string;
  interrupted?: string;
  status: string;
  reason?: string;
  incomplete: boolean;
};
export type AuditInput = {
  id: string;
  broadcastId: string;
  connectionId: string;
  remoteIp: string;
  started: string;
  lastMedia: string;
  ended?: string;
  interrupted?: string;
  reason?: string;
  incomplete: boolean;
};
export type AuditEvent = {
  id: string;
  node: string;
  boot: string;
  seq: number;
  at: string;
  recordedAt: string;
  type: string;
  actor: AuditActor;
  outcome: string;
  owner?: string;
  broadcastId?: string;
  inputId?: string;
  connectionId?: string;
  channelId?: string;
  reason?: string;
  version: number;
  payload: {
    count?: number;
    firstAt?: string;
    lastAt?: string;
    before?: AuditSettings;
    after?: AuditSettings;
    broadcast?: AuditBroadcast;
    input?: AuditInput;
  };
};
export type AuditStatus = {
  enabled: boolean;
  degraded: boolean;
  lastSuccess?: string;
  pendingBytes: number;
  pendingFiles: number;
  quarantinedFiles: number;
  queued: number;
  dropped: number;
  reason?: string;
};

export const eventNames: Record<string, string> = {
  "auth.login": "ログイン",
  "auth.logout": "ログアウト",
  "key.issue": "配信キー発行",
  "key.rotate": "配信キー再発行",
  "key.revoke": "配信キー失効",
  "broadcast.create": "配信枠作成",
  "broadcast.metadata": "配信情報変更",
  "broadcast.end": "配信終了",
  "broadcast.interrupted": "配信中断を検知",
  "rtmp.publish": "RTMP接続要求",
  "input.start": "メディア受信開始",
  "input.progress": "メディア受信状況",
  "input.end": "メディア受信終了",
  "system.start": "ノード起動",
  "system.stop": "ノード停止",
  "audit.gap": "記録の欠落",
};
export const outcomeNames: Record<string, string> = {
  success: "成功",
  failure: "失敗",
  cancelled: "キャンセル",
  unknown: "不明",
};
export const statusNames: Record<string, string> = {
  waiting: "受信待ち",
  live: "未終了",
  ended: "終了",
  interrupted: "中断（終了時刻不明）",
};
const reasons: Record<string, string> = {
  encoder_disconnect: "エンコーダー切断",
  user_stop: "配信者が停止",
  admin_stop: "管理者が停止",
  server_shutdown: "ノード終了",
  target_changed: "配信枠の切り替え",
  setup_rollback: "設定保存失敗による取り消し",
  process_interrupted: "前回起動の終了を確認できず",
  unknown_key: "未発行の配信キー",
  rate_limited: "接続試行数の制限",
  operation_rejected: "操作を受け付けられず",
  invalid_callback: "ログイン情報が不正または期限切れ",
  oauth_exchange_failed: "X認証に失敗",
  user_cancelled: "ログインをキャンセル",
  session_capacity: "ログイン数の上限",
  events_dropped: "記録の欠落",
  development_login_rejected: "開発ログインを拒否",
};
export const reasonName = (reason?: string) =>
  reason ? (reasons[reason] ?? reason) : "—";
export function auditTime(value?: string) {
  if (!value) return "—";
  const date = new Date(value);
  return Number.isNaN(date.getTime())
    ? "不明"
    : date.toLocaleString("ja-JP", {
        year: "numeric",
        month: "2-digit",
        day: "2-digit",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        hour12: false,
      });
}
