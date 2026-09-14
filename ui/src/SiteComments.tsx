import { useCallback, useState } from "react";
import { useResource } from "./hooks";
import { Notice } from "./components";
import { siteAPI } from "./site-api";

type Board = {
  supported: boolean;
  threadId: string;
  threadTitle: string;
  commentCount: number;
  threads: { id: string; title: string; comments: number }[];
  comments: { no: number; name: string; date: string; body: string }[];
};
export function SiteComments({ channelId }: { channelId: string }) {
  const [thread, setThread] = useState("");
  return (
    <section className="panel site-comments" aria-label="掲示板コメント">
      <h2>掲示板</h2>
      <CommentThread
        key={thread}
        channelId={channelId}
        thread={thread}
        selectThread={setThread}
      />
    </section>
  );
}
function CommentThread({
  channelId,
  thread,
  selectThread,
}: {
  channelId: string;
  thread: string;
  selectThread: (id: string) => void;
}) {
  const load = useCallback(
    (signal: AbortSignal) =>
      siteAPI<Board>(
        `channels/${channelId}/comments${thread ? `?thread=${encodeURIComponent(thread)}` : ""}`,
        { signal },
      ),
    [channelId, thread],
  );
  const result = useResource(load, 10000);
  const data = result.data;
  return (
    <>
      <Notice error={result.error} />
      {result.error && (
        <button onClick={result.reload} disabled={result.loading}>
          コメントを再取得
        </button>
      )}
      {!data && result.loading && <p role="status">コメントを読み込み中…</p>}
      {data && !data.supported && (
        <p>
          この掲示板のコメント表示には対応していません。配信者の連絡先から確認してください。
        </p>
      )}
      {data?.supported && (
        <>
          <label>
            スレッド
            <select
              value={data.threadId}
              onChange={(e) => selectThread(e.target.value)}
            >
              <option value="">スレッドを選択してください</option>
              {data.threadId &&
                !data.threads.some((t) => t.id === data.threadId) && (
                  <option value={data.threadId}>
                    {data.threadTitle || data.threadId}
                  </option>
                )}
              {data.threads.map((t) => (
                <option key={t.id} value={t.id}>
                  {t.title} ({t.comments})
                </option>
              ))}
            </select>
          </label>
          {data.threadId && (
            <p className="muted">
              {data.threadTitle} · {data.commentCount} レス · 最新 30 件
            </p>
          )}
          <div className="site-comment-list" aria-label="コメント一覧">
            {data.comments.map((c) => (
              <article key={c.no}>
                <div className="muted">
                  {c.no} · {c.name} <time>{c.date}</time>
                </div>
                <p>{c.body}</p>
              </article>
            ))}
            {data.threadId && data.comments.length === 0 && (
              <p>まだコメントがありません。</p>
            )}
          </div>
        </>
      )}
    </>
  );
}
