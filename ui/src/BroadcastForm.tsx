import { useEffect, useRef, useState } from "react";
import { Notice } from "./components";
import { useAction, errorMessage } from "./hooks";
import {
  siteAPI,
  type BroadcastSettings,
  type BroadcastHistoryEntry,
} from "./site-api";

const emptySettings: BroadcastSettings = {
  name: "",
  genre: "",
  description: "",
  comment: "",
  contactUrl: "",
};
type BoardThread = { id: string; title: string; comments: number };
type BroadcastBoard = {
  supported: boolean;
  boardTitle: string;
  boardUrl: string;
  thread: BoardThread | null;
  threadUrl: string;
  latestThread: BoardThread | null;
  latestThreadUrl: string;
  threadError?: string;
};

// Keyed by URL: edits unmount the old checker and cancel its reads/actions.
function ContactBoard({
  url,
  onMove,
}: {
  url: string;
  onMove: (url: string) => void;
}) {
  const [data, setData] = useState<BroadcastBoard | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [revision, setRevision] = useState(0);
  const action = useAction();
  const moveController = useRef<AbortController | null>(null);
  useEffect(() => {
    const controller = new AbortController();
    const timer = setTimeout(() => {
      void siteAPI<BroadcastBoard>(
        `broadcast/board?url=${encodeURIComponent(url)}`,
        { signal: controller.signal },
      )
        .then((result) => {
          if (!controller.signal.aborted) {
            setData(result);
            setError(null);
          }
        })
        .catch((err: unknown) => {
          if (!controller.signal.aborted) setError(errorMessage(err));
        });
    }, 400);
    return () => {
      clearTimeout(timer);
      controller.abort();
      moveController.current?.abort();
    };
  }, [url, revision]);
  return (
    <div className="broadcast-board" aria-live="polite">
      <Notice error={error || action.error || data?.threadError} />
      {error ? (
        <button type="button" onClick={() => setRevision((n) => n + 1)}>
          掲示板情報を再取得
        </button>
      ) : !data ? (
        <span>掲示板を確認中…</span>
      ) : !data.supported ? (
        <span className="muted">
          このURLは掲示板情報の表示に対応していません。
        </span>
      ) : (
        <>
          <span>
            掲示板「
            <a href={data.boardUrl} target="_blank" rel="noreferrer">
              {data.boardTitle}
            </a>
            」
            {data.thread && (
              <>
                のスレ「
                <a href={data.threadUrl} target="_blank" rel="noreferrer">
                  {data.thread.title} ({data.thread.comments})
                </a>
                」
              </>
            )}
          </span>
          <button
            type="button"
            disabled={action.busy || !data.latestThread}
            onClick={() => {
              void action.run(async () => {
                const controller = new AbortController();
                moveController.current = controller;
                const latest = await siteAPI<BroadcastBoard>(
                  `broadcast/board?url=${encodeURIComponent(url)}`,
                  { signal: controller.signal },
                );
                if (controller.signal.aborted) return;
                if (!latest.latestThread)
                  throw new Error(
                    "この板には埋まっていないスレッドがありません。",
                  );
                onMove(latest.latestThreadUrl);
              }, "");
            }}
          >
            新スレに移動
          </button>
          {!data.latestThread && (
            <span className="muted">埋まっていないスレッドがありません。</span>
          )}
        </>
      )}
    </div>
  );
}

export function BroadcastForm({
  history,
  initial,
  disabled,
  onSubmit,
}: {
  history: BroadcastHistoryEntry[];
  initial?: BroadcastSettings;
  disabled: boolean;
  onSubmit: (settings: BroadcastSettings) => void;
}) {
  // Mount only after the owner response arrives. Polling must not replace edits.
  const [fields, setFields] = useState<BroadcastSettings>(
    () => initial ?? history[0] ?? emptySettings,
  );
  const set = (key: keyof BroadcastSettings, value: string) =>
    setFields((current) => ({ ...current, [key]: value }));
  return (
    <form
      className="broadcast-form"
      onSubmit={(e) => {
        e.preventDefault();
        onSubmit(fields);
      }}
    >
      <fieldset disabled={disabled} aria-label="チャンネル作成">
        <select
          aria-label="以前の設定を読み込む"
          value=""
          disabled={history.length === 0}
          onChange={(e) => {
            const entry = history[Number(e.target.value)];
            if (entry) setFields(entry);
          }}
        >
          <option value="" disabled>
            以前の設定を読み込む
          </option>
          {history.map((entry, i) => (
            <option key={`${entry.createdAt}-${i}`} value={i}>
              {entry.name} · {new Date(entry.createdAt).toLocaleString("ja-JP")}{" "}
              · {entry.description || entry.comment}
            </option>
          ))}
        </select>
        <div className="broadcast-fields">
          <label>
            チャンネル名
            <input
              required
              maxLength={256}
              value={fields.name}
              onChange={(e) => set("name", e.target.value)}
            />
          </label>
          <label>
            ジャンル
            <input
              maxLength={256}
              value={fields.genre}
              onChange={(e) => set("genre", e.target.value)}
            />
          </label>
          <label>
            詳細
            <input
              maxLength={2048}
              value={fields.description}
              onChange={(e) => set("description", e.target.value)}
            />
          </label>
          <label>
            コメント
            <input
              maxLength={2048}
              value={fields.comment}
              onChange={(e) => set("comment", e.target.value)}
            />
          </label>
          <label>
            コンタクトURL
            <input
              type="url"
              maxLength={2048}
              value={fields.contactUrl}
              onChange={(e) => set("contactUrl", e.target.value)}
              placeholder="https://…"
            />
          </label>
        </div>
        {fields.contactUrl.trim() && (
          <ContactBoard
            key={fields.contactUrl.trim()}
            url={fields.contactUrl.trim()}
            onMove={(url) => set("contactUrl", url)}
          />
        )}
        <button type="submit">配信枠を作成</button>
      </fieldset>
    </form>
  );
}
