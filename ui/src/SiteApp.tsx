import { siteURL, sitePath } from "./site-path";
import { useEffect, useRef, useState } from "react";
import { Notice, Secret } from "./components";
import { useAction, useResource } from "./hooks";
import {
  siteAPI,
  type SiteSession,
  type SiteDirectory,
  type OwnBroadcast,
  type BroadcastSettings,
} from "./site-api";
import { BroadcastForm } from "./BroadcastForm";
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
              <a className="site-brand" href={siteURL("/")}>
                PecaMI
              </a>
            </h1>
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
                href={siteURL(
                  `/auth/x/start?next=${encodeURIComponent(window.location.pathname)}`,
                )}
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
        <a href={siteURL("/admin")}>管理パネル</a>
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
          href={siteURL("/")}
          aria-current={sitePath() === "/" ? "page" : undefined}
        >
          チャンネル一覧
        </a>
        <a
          href={siteURL("/broadcast")}
          aria-current={sitePath() === "/broadcast" ? "page" : undefined}
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
  const path = sitePath();
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
          ページが見つかりません。<a href={siteURL("/")}>チャンネル一覧へ</a>
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
            href={siteURL(`/channels/${c.id}`)}
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
      <a className="site-back" href={siteURL("/")}>
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
  const own = useResource(loadBroadcast, 5000);
  const action = useAction();
  const [stopped, setStopped] = useState<BroadcastSettings>();
  return (
    <section className="panel">
      <h2>チャンネル作成</h2>
      <Notice error={own.error || action.error} message={action.message} />
      <button onClick={own.reload} disabled={own.loading || action.busy}>
        配信状態を更新
      </button>
      {own.data && (
        <>
          <section
            className="broadcast-connection"
            aria-label="配信ソフトの設定"
          >
            <p className="broadcast-connection-help">
              作成後、以下をOBSなどに設定して配信を開始してください。
            </p>
            <dl className="broadcast-connection-fields">
              <div>
                <dt>配信先</dt>
                <dd>
                  <code>{own.data.rtmpUrl}</code>
                </dd>
              </div>
              <div>
                <dt>配信キー</dt>
                <dd>
                  {own.data.streamKey ? (
                    <Secret
                      key={own.data.streamKey}
                      value={own.data.streamKey}
                      maskLength={own.data.streamKey.length}
                    />
                  ) : (
                    <span className="muted">未発行</span>
                  )}
                  <p className="muted">キーは他の人に教えないでください。</p>
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
                </dd>
              </div>
            </dl>
          </section>
          {!own.data.channel && (
            <BroadcastForm
              history={own.data.history ?? []}
              initial={stopped}
              disabled={action.busy || !own.data.streamKey || !!own.error}
              onSubmit={(settings) => {
                void action.run(async () => {
                  await siteAPI("broadcast", {
                    method: "POST",
                    csrf,
                    body: {
                      name: settings.name,
                      genre: settings.genre,
                      description: settings.description,
                      comment: settings.comment,
                      contactUrl: settings.contactUrl,
                      bitrate: 0,
                    },
                  });
                  setStopped(undefined);
                  own.reload();
                }, "配信枠を作成しました。配信ソフトで送信を開始してください。");
              }}
            />
          )}
          {own.data.channel ? (
            <>
              <h3>{own.data.channel.name}</h3>
              <p role="status">
                {own.data.channel.receiving ? "配信中" : "OBS接続待ち"}
              </p>
              <dl className="broadcast-details">
                <dt>ジャンル</dt>
                <dd>{own.data.channel.genre || "—"}</dd>
                <dt>説明</dt>
                <dd>{own.data.channel.description || "—"}</dd>
                <dt>コメント</dt>
                <dd>{own.data.channel.comment || "—"}</dd>
                <dt>コンタクトURL</dt>
                <dd>{own.data.channel.contactUrl || "—"}</dd>
                <dt>ビットレート</dt>
                <dd>
                  {own.data.channel.bitrate
                    ? `${own.data.channel.bitrate} kbps`
                    : "自動"}
                </dd>
              </dl>
              <p>
                <a href={siteURL(`/channels/${own.data.channel.id}`)}>
                  視聴ページを開く
                </a>
              </p>
              <p>
                配信停止後は、OBS側でも送信を停止してください。配信キーは引き続き使えます。
              </p>
              <button
                disabled={action.busy || own.loading || !!own.error}
                onClick={() => {
                  const current = own.data?.channel;
                  if (
                    !current ||
                    !window.confirm(`「${current.name}」の配信を停止しますか？`)
                  )
                    return;
                  void action.run(async () => {
                    await siteAPI("broadcast", { method: "DELETE", csrf });
                    setStopped(
                      own.data?.history?.[0] ?? {
                        name: current.name,
                        genre: current.genre,
                        description: current.description,
                        comment: current.comment ?? "",
                        contactUrl: current.contactUrl,
                      },
                    );
                    own.reload();
                  }, `「${current.name}」の配信を停止しました。入力内容を引き継いで再作成できます。`);
                }}
              >
                自分の配信を停止
              </button>
            </>
          ) : null}
        </>
      )}
    </section>
  );
}
