import { lazy, Suspense, useState } from "react";
const AuditLogPage = lazy(() => import("./AuditLogPage"));
import "./App.css";
import { ChannelsPage } from "./ChannelsPage";
import { StreamKeysPage } from "./StreamKeysPage";
import { StatusPage } from "./StatusPage";

type Page = "channels" | "keys" | "status" | "audit";
export default function App({
  auditAvailable = false,
}: {
  auditAvailable?: boolean;
}) {
  const [page, setPage] = useState<Page>("channels");
  return (
    <div className="app">
      <a className="skip-link" href="#main">
        メインコンテンツへ
      </a>
      <header className="app-header">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            mi
          </span>
          <div>
            <h1>peercast-mi</h1>
            <span className="muted">Node console</span>
          </div>
        </div>
        <nav aria-label="メインナビゲーション">
          {(
            [
              ["channels", "チャンネル"],
              ["keys", "ストリームキー"],
              ["status", "ノード情報"],
              ...(auditAvailable ? [["audit", "ログ"] as const] : []),
            ] as const
          ).map(([id, label]) => (
            <button
              key={id}
              aria-current={page === id ? "page" : undefined}
              onClick={() => setPage(id)}
            >
              {label}
            </button>
          ))}
        </nav>
      </header>
      <main id="main" tabIndex={-1}>
        {page === "channels" ? (
          <ChannelsPage />
        ) : page === "keys" ? (
          <StreamKeysPage />
        ) : page === "audit" ? (
          <Suspense fallback={<p role="status">ログ画面を読み込み中…</p>}>
            <AuditLogPage />
          </Suspense>
        ) : (
          <StatusPage />
        )}
      </main>
      <footer>
        PecaMI <span>RTMP / PCP node management</span>
      </footer>
    </div>
  );
}
