import {
  useCallback,
  useState,
  useEffect,
  useRef,
  type ReactNode,
  type FormEvent,
} from "react";
import { Notice, TableArea } from "./components";
import { useResource } from "./hooks";
import { siteAPI } from "./site-api";
import {
  auditTime,
  eventNames,
  outcomeNames,
  reasonName,
  statusNames,
  type AuditPage,
  type AuditEvent,
  type AuditBroadcast,
  type AuditInput,
  type AuditStatus,
  type AuditSettings,
} from "./audit-api";
import "./AuditLogPage.css";

type Kind = "events" | "broadcasts";
type Search = {
  from: string;
  until: string;
  account: string;
  type: string;
  outcome: string;
  status: string;
  ip: string;
  broadcastId: string;
};
function initialSearch(): Search {
  const date = new Date();
  date.setDate(date.getDate() - 7);
  date.setHours(0, 0, 0, 0);
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000)
    .toISOString()
    .slice(0, 16);
  return {
    from: local,
    until: "",
    account: "",
    type: "",
    outcome: "",
    status: "",
    ip: "",
    broadcastId: "",
  };
}
const loadStatus = (signal: AbortSignal) =>
  siteAPI<AuditStatus>("audit/status", { signal });

export default function AuditLogPage() {
  const status = useResource(loadStatus, 30000);
  return (
    <section className="audit-page" aria-labelledby="audit-title">
      <div className="page-header">
        <div>
          <h2 id="audit-title">ログ</h2>
          <p>ログイン・操作と、このノードで作成した配信の記録</p>
        </div>
      </div>
      <Notice error={status.error} />
      {status.data && (
        <div className="audit-health" role="status">
          <strong>
            {!status.data.enabled
              ? "記録は無効です"
              : status.data.degraded
                ? "記録に問題があります"
                : status.data.lastSuccess
                  ? "記録は正常です"
                  : "記録の開始を待っています"}
          </strong>
          {status.data.enabled && (
            <span>
              最終DB処理成功：{auditTime(status.data.lastSuccess)} ／
              未送信ファイル：{status.data.pendingFiles} ／ 待機：
              {status.data.queued}件 ／ 欠落：{status.data.dropped}件 ／ 隔離：
              {status.data.quarantinedFiles}件
            </span>
          )}
        </div>
      )}
      {status.loading && !status.data && (
        <p role="status">記録の状態を確認中…</p>
      )}
      {status.error && <button onClick={status.reload}>状態を再取得</button>}
      {status.data?.enabled && <LogBrowser />}
    </section>
  );
}
function LogBrowser() {
  const [kind, setKind] = useState<Kind>("events");
  const [draft, setDraft] = useState<Search>(initialSearch);
  const [applied, setApplied] = useState<Search>(draft);
  const [revision, setRevision] = useState(0);
  const update = (field: keyof Search, value: string) =>
    setDraft((old) => ({ ...old, [field]: value }));
  const params = new URLSearchParams({ limit: "50" });
  if (applied.from) params.set("from", new Date(applied.from).toISOString());
  if (applied.until) params.set("until", new Date(applied.until).toISOString());
  const account = applied.account.trim();
  if (account)
    params.set(
      kind === "events" ? "actor" : "owner",
      /^\d+$/.test(account) ? `site:x:${account}` : account,
    );
  if (kind === "events") {
    for (const field of ["type", "outcome", "ip", "broadcastId"] as const)
      if (applied[field]) params.set(field, applied[field]);
  } else if (applied.status) params.set("status", applied.status);
  const path = `audit/${kind}?${params}`;
  const search = (e: FormEvent) => {
    e.preventDefault();
    setApplied({ ...draft });
    setRevision((n) => n + 1);
  };
  const related = (id: string) => {
    const next = { ...initialSearch(), from: "", broadcastId: id };
    setKind("events");
    setDraft(next);
    setApplied(next);
    setRevision((n) => n + 1);
  };
  return (
    <>
      <nav aria-label="ログの種類">
        <button
          aria-current={kind === "events" ? "page" : undefined}
          onClick={() => setKind("events")}
        >
          操作ログ
        </button>
        <button
          aria-current={kind === "broadcasts" ? "page" : undefined}
          onClick={() => setKind("broadcasts")}
        >
          配信履歴
        </button>
      </nav>
      <form className="audit-filters" onSubmit={search}>
        <label>
          開始日時
          <input
            type="datetime-local"
            value={draft.from}
            onChange={(e) => update("from", e.target.value)}
          />
        </label>
        <label>
          終了日時（この時刻より前）
          <input
            type="datetime-local"
            value={draft.until}
            onChange={(e) => update("until", e.target.value)}
          />
        </label>
        <label>
          {kind === "events" ? "操作したアカウント" : "配信者のアカウント"}
          <input
            value={draft.account}
            placeholder="X ID または site:x:123"
            maxLength={255}
            onChange={(e) => update("account", e.target.value)}
          />
        </label>
        {kind === "events" ? (
          <>
            <label>
              イベント
              <select
                value={draft.type}
                onChange={(e) => update("type", e.target.value)}
              >
                <option value="">すべて</option>
                {Object.entries(eventNames).map(([v, n]) => (
                  <option key={v} value={v}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <label>
              結果
              <select
                value={draft.outcome}
                onChange={(e) => update("outcome", e.target.value)}
              >
                <option value="">すべて</option>
                {Object.entries(outcomeNames).map(([v, n]) => (
                  <option key={v} value={v}>
                    {n}
                  </option>
                ))}
              </select>
            </label>
            <label>
              接続元IP
              <input
                value={draft.ip}
                maxLength={45}
                onChange={(e) => update("ip", e.target.value)}
              />
            </label>
            <label>
              配信履歴ID
              <input
                value={draft.broadcastId}
                maxLength={32}
                onChange={(e) => update("broadcastId", e.target.value)}
              />
            </label>
          </>
        ) : (
          <label>
            配信状態
            <select
              value={draft.status}
              onChange={(e) => update("status", e.target.value)}
            >
              <option value="">すべて</option>
              {Object.entries(statusNames).map(([v, n]) => (
                <option key={v} value={v}>
                  {n}
                </option>
              ))}
            </select>
          </label>
        )}
        <div className="actions">
          <button className="primary" type="submit">
            検索
          </button>
          <button
            type="button"
            onClick={() => {
              const next = { ...initialSearch(), from: "" };
              setDraft(next);
              setApplied(next);
              setRevision((n) => n + 1);
            }}
          >
            条件をクリア
          </button>
          <button
            type="button"
            onClick={() => {
              setRevision((n) => n + 1);
            }}
          >
            最新を取得
          </button>
        </div>
      </form>
      <p className="muted">
        日時は{Intl.DateTimeFormat().resolvedOptions().timeZone}
        で表示します。初期表示は過去7日間です。
        {kind === "events" &&
          "IP検索は31日以内の開始・終了日時を指定してください。"}
      </p>
      {kind === "events" ? (
        <Pages<AuditEvent>
          key={`${path}:${revision}`}
          path={path}
          label="操作ログ"
          render={(items) => <EventTable items={items} />}
        />
      ) : (
        <Pages<AuditBroadcast>
          key={`${path}:${revision}`}
          path={path}
          label="配信履歴"
          render={(items) => <BroadcastTable items={items} related={related} />}
        />
      )}
    </>
  );
}
function Pages<T>({
  path,
  label,
  render,
}: {
  path: string;
  label: string;
  render: (items: T[]) => ReactNode;
}) {
  const [cursors, setCursors] = useState([""]);
  const token = cursors[cursors.length - 1];
  const url = `${path}${path.includes("?") ? "&" : "?"}cursor=${encodeURIComponent(token)}`;
  return (
    <DataPage<T>
      key={url}
      path={url}
      label={label}
      render={render}
      number={cursors.length}
      previous={
        cursors.length > 1
          ? () => setCursors((old) => old.slice(0, -1))
          : undefined
      }
      next={(cursor) => setCursors((old) => [...old, cursor])}
    />
  );
}
function DataPage<T>({
  path,
  label,
  render,
  number,
  previous,
  next,
}: {
  path: string;
  label: string;
  render: (items: T[]) => ReactNode;
  number: number;
  previous?: () => void;
  next: (cursor: string) => void;
}) {
  const load = useCallback(
    (signal: AbortSignal) => siteAPI<AuditPage<T>>(path, { signal }),
    [path],
  );
  const data = useResource(load);
  return (
    <div className="audit-results" aria-label={`${label}の検索結果`}>
      <Notice error={data.error} />
      {data.loading ? (
        <p role="status">読み込み中…</p>
      ) : data.error ? (
        <button onClick={data.reload}>再試行</button>
      ) : (
        data.data &&
        (data.data.items.length ? (
          render(data.data.items)
        ) : (
          <p role="status">該当する記録はありません。</p>
        ))
      )}
      <div className="audit-pagination">
        <button disabled={!previous || data.loading} onClick={previous}>
          前のページ
        </button>
        <span>
          {number}ページ目
          {data.data && !data.loading && !data.error
            ? `（${data.data.items.length}件）`
            : ""}
        </span>
        <button
          disabled={!data.data?.nextCursor || data.loading || !!data.error}
          onClick={() => {
            if (data.data?.nextCursor) next(data.data.nextCursor);
          }}
        >
          次のページ
        </button>
      </div>
    </div>
  );
}
function EventTable({ items }: { items: AuditEvent[] }) {
  return (
    <TableArea label="操作ログ一覧">
      <table>
        <thead>
          <tr>
            <th>日時</th>
            <th>イベント</th>
            <th>操作者</th>
            <th>結果</th>
            <th>接続元IP</th>
            <th>理由</th>
            <th>詳細</th>
          </tr>
        </thead>
        <tbody>
          {items.map((e) => (
            <tr key={e.id}>
              <td className="audit-time">{auditTime(e.at)}</td>
              <td>
                {eventNames[e.type] ?? e.type}
                {!!e.payload.count && <small>{e.payload.count}件を記録</small>}
              </td>
              <td>
                {e.actor.name || "—"}
                <small>{e.actor.account || "システム・未認証"}</small>
              </td>
              <td>{outcomeNames[e.outcome] ?? e.outcome}</td>
              <td>{e.actor.ip || "—"}</td>
              <td>{reasonName(e.reason)}</td>
              <td>
                <details>
                  <summary>詳細</summary>
                  <dl className="audit-detail-fields">
                    <dt>イベントID</dt>
                    <dd>{e.id}</dd>
                    <dt>DB保存日時</dt>
                    <dd>{auditTime(e.recordedAt)}</dd>
                    <dt>所有者</dt>
                    <dd>{e.owner || "—"}</dd>
                    <dt>発生元</dt>
                    <dd>{e.actor.source}</dd>
                    <dt>ノード</dt>
                    <dd>{e.node}</dd>
                    <dt>配信履歴ID</dt>
                    <dd>{e.broadcastId || "—"}</dd>
                    <dt>チャンネルID</dt>
                    <dd>{e.channelId || "—"}</dd>
                    <dt>接続ID</dt>
                    <dd>{e.connectionId || "—"}</dd>
                    <dt>受信区間ID</dt>
                    <dd>{e.inputId || "—"}</dd>
                    <dt>起動識別ID</dt>
                    <dd>{e.boot}</dd>
                    <dt>起動内の順番</dt>
                    <dd>{e.seq}</dd>
                    <dt>ログイン識別ID</dt>
                    <dd>{e.actor.session || "—"}</dd>
                    {e.payload.firstAt && (
                      <>
                        <dt>集約開始</dt>
                        <dd>{auditTime(e.payload.firstAt)}</dd>
                        <dt>集約終了</dt>
                        <dd>{auditTime(e.payload.lastAt)}</dd>
                      </>
                    )}
                  </dl>
                  {e.payload.before && e.payload.after && (
                    <SettingsDiff
                      before={e.payload.before}
                      after={e.payload.after}
                    />
                  )}
                  <details>
                    <summary>記録の補足データ</summary>
                    <pre>{JSON.stringify(e.payload, null, 2)}</pre>
                  </details>
                </details>
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableArea>
  );
}
function SettingsDiff({
  before,
  after,
}: {
  before: AuditSettings;
  after: AuditSettings;
}) {
  return (
    <table>
      <caption>配信情報の変更</caption>
      <thead>
        <tr>
          <th>項目</th>
          <th>変更前</th>
          <th>変更後</th>
        </tr>
      </thead>
      <tbody>
        {settingFields
          .filter(([k]) => before[k] !== after[k])
          .map(([k, n]) => (
            <tr key={k}>
              <th>{n}</th>
              <td>{before[k] ?? "—"}</td>
              <td>{after[k] ?? "—"}</td>
            </tr>
          ))}
      </tbody>
    </table>
  );
}
const settingFields: [keyof AuditSettings, string][] = [
  ["name", "チャンネル名"],
  ["inputGenre", "入力ジャンル"],
  ["genre", "公開ジャンル"],
  ["description", "詳細"],
  ["comment", "コメント"],
  ["contactUrl", "コンタクトURL"],
  ["bitrate", "ビットレート（kbps）"],
  ["contentType", "形式"],
];
function endTime(record: { ended?: string; interrupted?: string }) {
  return record.ended
    ? auditTime(record.ended)
    : record.interrupted
      ? "不明（中断）"
      : "未終了";
}
function BroadcastTable({
  items,
  related,
}: {
  items: AuditBroadcast[];
  related: (id: string) => void;
}) {
  const [selected, setSelected] = useState<AuditBroadcast | null>(null);
  return (
    <>
      <TableArea label="配信履歴一覧">
        <table>
          <thead>
            <tr>
              <th>枠作成</th>
              <th>チャンネル</th>
              <th>配信者</th>
              <th>受信開始</th>
              <th>終了</th>
              <th>状態</th>
              <th>終了理由</th>
              <th>詳細</th>
            </tr>
          </thead>
          <tbody>
            {items.map((b) => (
              <BroadcastRow
                key={b.id}
                item={b}
                open={selected?.id === b.id}
                select={() => setSelected(selected?.id === b.id ? null : b)}
              />
            ))}
          </tbody>
        </table>
      </TableArea>
      {selected && (
        <BroadcastDetail key={selected.id} item={selected} related={related} />
      )}
    </>
  );
}
function BroadcastRow({
  item: b,
  open,
  select,
}: {
  item: AuditBroadcast;
  open: boolean;
  select: () => void;
}) {
  return (
    <>
      <tr>
        <td className="audit-time">{auditTime(b.created)}</td>
        <td>{b.settings.name}</td>
        <td>
          {b.ownerName || "—"}
          <small>{b.owner || "所有者不明"}</small>
        </td>
        <td className="audit-time">{auditTime(b.firstMedia)}</td>
        <td className="audit-time">{endTime(b)}</td>
        <td>
          {statusNames[b.status] ?? b.status}
          {b.incomplete && <small>記録に不明・欠落あり</small>}
        </td>
        <td>{reasonName(b.reason)}</td>
        <td>
          <button aria-expanded={open} onClick={select}>
            {open ? "閉じる" : "詳細"}
          </button>
        </td>
      </tr>
    </>
  );
}
function BroadcastDetail({
  item: b,
  related,
}: {
  item: AuditBroadcast;
  related: (id: string) => void;
}) {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    heading.current?.focus();
  }, []);
  return (
    <section
      className="audit-broadcast-detail"
      aria-label={`${b.settings.name}の詳細`}
    >
      <h3 ref={heading} tabIndex={-1}>
        {b.settings.name}の配信記録
      </h3>
      <dl className="audit-detail-fields">
        <dt>配信履歴ID</dt>
        <dd>{b.id}</dd>
        <dt>チャンネルID</dt>
        <dd>{b.channelId}</dd>
        <dt>作成したアカウント</dt>
        <dd>{b.actor.account || "—"}</dd>
        <dt>作成元IP</dt>
        <dd>{b.actor.ip || "—"}</dd>
        <dt>最終メディア受信</dt>
        <dd>{auditTime(b.lastMedia)}</dd>
        <dt>中断を検知した日時</dt>
        <dd>{auditTime(b.interrupted)}</dd>
        {settingFields.map(([k, n]) => (
          <div key={k}>
            <dt>{n}</dt>
            <dd>{b.settings[k] ?? "—"}</dd>
          </div>
        ))}
      </dl>
      <button onClick={() => related(b.id)}>この配信の操作ログ</button>
      <h4>RTMP受信区間</h4>
      <p className="muted">
        枠作成とメディア受信の開始は別の時刻です。同時に複数の入力がある場合、受信区間は重複します。
      </p>
      <Pages<AuditInput>
        path={`audit/broadcasts/${b.id}/inputs?limit=50`}
        label="受信区間"
        render={(items) => <InputTable items={items} />}
      />
    </section>
  );
}
function InputTable({ items }: { items: AuditInput[] }) {
  return (
    <TableArea label="受信区間一覧">
      <table>
        <thead>
          <tr>
            <th>接続元IP</th>
            <th>受信開始</th>
            <th>最終受信</th>
            <th>終了</th>
            <th>理由</th>
            <th>記録状態</th>
          </tr>
        </thead>
        <tbody>
          {items.map((i) => (
            <tr key={i.id}>
              <td>
                {i.remoteIp}
                <small>受信区間ID：{i.id}</small>
                <small>接続ID：{i.connectionId}</small>
              </td>
              <td>{auditTime(i.started)}</td>
              <td>{auditTime(i.lastMedia)}</td>
              <td>{endTime(i)}</td>
              <td>{reasonName(i.reason)}</td>
              <td>{i.incomplete ? "不明・欠落あり" : "欠落の検知なし"}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </TableArea>
  );
}
