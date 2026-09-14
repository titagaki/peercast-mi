import { useEffect, useRef, useState } from "react";
import { Notice, Secret } from "./components";
import { useAction, useResource } from "./hooks";
import {
  siteAPI,
  type SiteSession,
  type SiteDirectory,
  type OwnBroadcast,
} from "./site-api";
import { SitePlayer } from "./SitePlayer";
import { SiteComments } from "./SiteComments";
import { SiteChannelInfo } from "./SiteChannelInfo";
import { channelExplanation } from "./site-channel-text";
import "./App.css";
import "./SiteApp.css";

const loadSession = (signal: AbortSignal) =>
  siteAPI<SiteSession>("me", { signal });
const loadChannels = (signal: AbortSignal) =>
  siteAPI<SiteDirectory>("directory", { signal });
const loadBroadcast = (signal: AbortSignal) =>
  siteAPI<OwnBroadcast>("broadcast", { signal });

export default function SiteApp() {
  const session = useResource(loadSession, 30000);
  const loginAction = useAction();
  return (
    <div className="app site-app">
      <a className="skip-link" href="#main">
        メインコンテンツへ
      </a>
      <header className="app-header">
        <div className="brand">
          <span className="brand-mark" aria-hidden="true">
            mi
          </span>
          <div>
            <h1>
              <a className="site-brand" href="/">
                peercast-mi live
              </a>
            </h1>
            <span className="muted">見つける、観る、配信する</span>
          </div>
        </div>
        {session.data?.user && (
          <SiteMenu session={session.data} onLogout={session.reload} />
        )}
      </header>
      <main id="main">
        <Notice error={session.error} />
        <Notice error={loginAction.error} />
        {session.data?.devLogin && (
          <p role="status">
            ローカル開発モード：X
            認証を省略しています。配信・視聴操作は実際のノードに反映されます。
          </p>
        )}
        {session.data?.user ? (
          <SignedIn key={session.data.user.id} session={session.data} />
        ) : (
          <section className="panel site-welcome">
            <h2>PeerCast の配信を、ここから。</h2>
            <p>
              {session.data?.devLogin
                ? "開発用ユーザーで視聴・配信を確認できます。"
                : "このサイトで視聴・配信するには X ログインが必要です。"}
            </p>
            <p className="muted">
              サイトの帯域を共同で利用するための認証です。PeerCast
              の公開中継は制限しません。
            </p>
            {session.loading ? (
              <p role="status">ログイン状態を確認中…</p>
            ) : session.error ? (
              <p>
                サイト接続を確認してから「ログイン状態を更新」を押してください。
              </p>
            ) : session.data?.devLogin ? (
              <button
                disabled={loginAction.busy}
                onClick={() =>
                  void loginAction.run(async () => {
                    await siteAPI("dev-login", { method: "POST" });
                    session.reload();
                  }, "")
                }
              >
                開発用ユーザーでログイン
              </button>
            ) : (
              <a
                className="site-login"
                href={`/auth/x/start?next=${encodeURIComponent(window.location.pathname)}`}
              >
                X でログイン
              </a>
            )}
            <button onClick={session.reload} disabled={session.loading}>
              ログイン状態を更新
            </button>
          </section>
        )}
      </main>
      <footer className="site-footer">
        <span>peercast-mi</span>
        <a href="/admin">管理パネル</a>
      </footer>
    </div>
  );
}

function SiteMenu({
  session,
  onLogout,
}: {
  session: SiteSession;
  onLogout: () => void;
}) {
  const action = useAction();
  const [open, setOpen] = useState(false);
  const container = useRef<HTMLDivElement>(null);
  const toggle = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (!open) return;
    const dismiss = (event: PointerEvent) => {
      if (!container.current?.contains(event.target as Node)) setOpen(false);
    };
    const escape = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        setOpen(false);
        toggle.current?.focus();
      }
    };
    document.addEventListener("pointerdown", dismiss);
    document.addEventListener("keydown", escape);
    return () => {
      document.removeEventListener("pointerdown", dismiss);
      document.removeEventListener("keydown", escape);
    };
  }, [open]);
  return (
    <div
      className="site-menu"
      ref={container}
      onBlur={(event) => {
        if (!event.currentTarget.contains(event.relatedTarget)) setOpen(false);
      }}
    >
      <button
        ref={toggle}
        className="site-menu-toggle"
        aria-label="メニュー"
        aria-expanded={open}
        aria-controls="site-navigation"
        onClick={() => setOpen(!open)}
      >
        <svg
          width="22"
          height="22"
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          strokeWidth="2"
          aria-hidden="true"
        >
          {open ? (
            <path d="m6 6 12 12M6 18 18 6" />
          ) : (
            <path d="M3 6h18M3 12h18M3 18h18" />
          )}
        </svg>
      </button>
      <nav
        id="site-navigation"
        className="site-menu-panel"
        aria-label="サイトナビゲーション"
        hidden={!open}
      >
        <p>{session.user?.name} さん</p>
        <a
          href="/"
          aria-current={window.location.pathname === "/" ? "page" : undefined}
        >
          チャンネル一覧
        </a>
        <a
          href="/broadcast"
          aria-current={
            window.location.pathname === "/broadcast" ? "page" : undefined
          }
        >
          配信する
        </a>
        <button
          disabled={action.busy}
          onClick={() =>
            void action.run(async () => {
              await siteAPI("logout", { method: "POST", csrf: session.csrf });
              onLogout();
            }, "")
          }
        >
          ログアウト
        </button>
        <Notice error={action.error} />
      </nav>
    </div>
  );
}

function SignedIn({ session }: { session: SiteSession }) {
  const path = window.location.pathname;
  const match = /^\/channels\/([a-fA-F0-9]{32})\/?$/.exec(path);
  return (
    <>
      {path === "/broadcast" ? (
        <Broadcast csrf={session.csrf ?? ""} />
      ) : match ? (
        <WatchPage id={match[1].toLowerCase()} />
      ) : path === "/" || path === "/watch" ? (
        <ChannelList />
      ) : (
        <p>
          ページが見つかりません。<a href="/">チャンネル一覧へ</a>
        </p>
      )}
    </>
  );
}

function ChannelList() {
  const channels = useResource(loadChannels, 10000);
  const [search, setSearch] = useState("");
  const rows = channels.data?.channels.filter((c) =>
    `${c.name} ${channelExplanation(c)}`
      .toLowerCase()
      .includes(search.toLowerCase()),
  );
  return (
    <>
      <Notice error={channels.error} />
      <div className="section-heading">
        <h2>チャンネル一覧</h2>
        <button onClick={channels.reload} disabled={channels.loading}>
          一覧を更新
        </button>
      </div>
      <label className="site-search">
        チャンネルを検索
        <input
          type="search"
          value={search}
          onChange={(e) => setSearch(e.target.value)}
          placeholder="名前・ジャンル・説明"
        />
      </label>
      {channels.loading && !channels.data && (
        <p role="status">配信一覧を読み込み中…</p>
      )}
      {rows?.length === 0 && (
        <p>
          {search
            ? "検索条件に合うチャンネルはありません。"
            : "現在表示できるチャンネルはありません。"}
        </p>
      )}
      <div className="site-channels">
        {rows?.map((c) => (
          <a
            className="panel site-channel-card"
            key={c.id}
            href={`/channels/${c.id}`}
            aria-label={c.name || "名前なし"}
          >
            <SiteChannelInfo channel={c} />
          </a>
        ))}
      </div>
    </>
  );
}

function WatchPage({ id }: { id: string }) {
  const channels = useResource(loadChannels, 10000);
  const current = channels.data?.channels.find((c) => c.id === id);
  return (
    <>
      <a className="site-back" href="/">
        ← チャンネル一覧
      </a>
      <Notice error={channels.error} />
      {current ? (
        <div className="site-watch-layout">
          <SitePlayer key={current.id} channel={current} />
          <SiteComments
            key={current.id + current.contactUrl}
            channelId={current.id}
          />
        </div>
      ) : (
        <p role="status">
          {channels.loading && !channels.data
            ? "チャンネルを読み込み中…"
            : channels.error
              ? "チャンネル情報を取得できません。"
              : "この配信は終了したか、一覧にありません。"}
        </p>
      )}
    </>
  );
}

function Broadcast({ csrf }: { csrf: string }) {
  const own = useResource(loadBroadcast);
  const action = useAction();
  const [name, setName] = useState("");
  const [genre, setGenre] = useState("");
  const [description, setDescription] = useState("");
  return (
    <section className="panel">
      <h2>配信する</h2>
      <Notice error={own.error || action.error} message={action.message} />
      <button onClick={own.reload} disabled={own.loading || action.busy}>
        配信状態を更新
      </button>
      {own.data && (
        <>
          <p>
            OBS
            等の配信ソフトに設定してください。先に配信枠を作成し、その後ソフトで配信を開始します。
          </p>
          <p>
            配信先: <code>{own.data.rtmpUrl}</code>
          </p>
          {own.data.streamKey && (
            <>
              <h3>あなたの配信キー</h3>
              <Secret key={own.data.streamKey} value={own.data.streamKey} />
              <p className="muted">
                他の人には渡さないでください。ログアウトしても配信キーは有効です。
              </p>
            </>
          )}
          <button
            disabled={action.busy || own.loading || !!own.data.channel}
            onClick={() => {
              if (
                own.data?.streamKey &&
                !window.confirm(
                  "以前のキーは使えなくなります。再発行しますか？",
                )
              )
                return;
              void action.run(async () => {
                await siteAPI("key", { method: "POST", csrf });
                own.reload();
              }, "配信キーを発行しました。");
            }}
          >
            {own.data.streamKey ? "配信キーを再発行" : "配信キーを発行"}
          </button>
          {own.data.channel ? (
            <>
              <h3>{own.data.channel.name}</h3>
              <p>配信枠が作成されています。</p>
              <button
                disabled={action.busy}
                onClick={() => {
                  if (!window.confirm("自分の配信を停止しますか？")) return;
                  void action.run(async () => {
                    await siteAPI("broadcast", { method: "DELETE", csrf });
                    own.reload();
                  }, "配信を停止しました。");
                }}
              >
                自分の配信を停止
              </button>
            </>
          ) : (
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void action.run(async () => {
                  await siteAPI("broadcast", {
                    method: "POST",
                    csrf,
                    body: { name, genre, description },
                  });
                  own.reload();
                }, "配信枠を作成しました。配信ソフトで送信を開始してください。");
              }}
            >
              <fieldset
                disabled={action.busy || own.loading || !own.data.streamKey}
              >
                <legend>新しい配信枠</legend>
                <div className="form-grid">
                  <label>
                    配信名
                    <input
                      required
                      maxLength={80}
                      value={name}
                      onChange={(e) => setName(e.target.value)}
                    />
                  </label>
                  <label>
                    ジャンル
                    <input
                      maxLength={80}
                      value={genre}
                      onChange={(e) => setGenre(e.target.value)}
                    />
                  </label>
                  <label>
                    説明
                    <textarea
                      maxLength={600}
                      value={description}
                      onChange={(e) => setDescription(e.target.value)}
                    />
                  </label>
                </div>
                <button type="submit">配信枠を作成</button>
              </fieldset>
            </form>
          )}
        </>
      )}
    </section>
  );
}
