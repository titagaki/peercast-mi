import { useState } from "react";
import "./App.css";
import { ChannelsPage } from "./ChannelsPage";
import { StreamKeysPage } from "./StreamKeysPage";
import { StatusPage } from "./StatusPage";

type Page = "channels" | "keys" | "status";
export default function App() {
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
        ) : (
          <StatusPage />
        )}
      </main>
      <footer>
        peercast-mi <span>RTMP / PCP node management</span>
      </footer>
    </div>
  );
}
