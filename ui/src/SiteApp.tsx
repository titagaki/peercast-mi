import { useState } from "react";
import { Notice, Secret } from "./components";
import { useAction, useResource } from "./hooks";
import {
  siteAPI,
  type SiteSession,
  type SiteChannel,
  type OwnBroadcast,
} from "./site-api";
import { SitePlayer } from "./SitePlayer";
import "./App.css";
import "./SiteApp.css";

const loadSession = (signal: AbortSignal) =>
  siteAPI<SiteSession>("me", { signal });
const loadChannels = (signal: AbortSignal) =>
  siteAPI<SiteChannel[]>("channels", { signal });
const loadBroadcast = (signal: AbortSignal) =>
  siteAPI<OwnBroadcast>("broadcast", { signal });

export default function SiteApp() {
  const session = useResource(loadSession, 30000);
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
            <h1>peercast-mi live</h1>
            <span className="muted">見つける、観る、配信する</span>
          </div>
        </div>
      </header>
      <main id="main">
        <Notice error={session.error} />
        {session.data?.user ? (
          <SignedIn
            key={session.data.user.id}
            session={session.data}
            onLogout={session.reload}
          />
        ) : (
          <section className="panel site-welcome">
            <h2>PeerCast の配信を、ここから。</h2>
            <p>このサイトで視聴・配信するには X ログインが必要です。</p>
            <p className="muted">
              サイトの帯域を共同で利用するための認証です。PeerCast
              の公開中継は制限しません。
            </p>
            {session.loading ? (
              <p role="status">ログイン状態を確認中…</p>
            ) : (
              <a className="site-login" href="/auth/x/start">
                X でログイン
              </a>
            )}
            <button onClick={session.reload} disabled={session.loading}>
              ログイン状態を更新
            </button>
          </section>
        )}
      </main>
      <footer>
        peercast-mi <span>公開 PCP 中継 / 認証付きサイト視聴</span>
      </footer>
    </div>
  );
}

function SignedIn({
  session,
  onLogout,
}: {
  session: SiteSession;
  onLogout: () => void;
}) {
  const channels = useResource(loadChannels, 10000);
  const action = useAction();
  const [selected, setSelected] = useState("");
  const [search, setSearch] = useState("");
  const [page, setPage] = useState<"watch" | "broadcast">("watch");
  const current = channels.data?.find((c) => c.id === selected);
  return (
    <>
      <div className="section-heading">
        <p>{session.user?.name} さん</p>
        <div className="actions">
          <button
            aria-pressed={page === "watch"}
            onClick={() => setPage("watch")}
          >
            視聴する
          </button>
          <button
            aria-pressed={page === "broadcast"}
            onClick={() => setPage("broadcast")}
          >
            自分の配信
          </button>
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
        </div>
      </div>
      <Notice error={action.error} />
      {page === "broadcast" ? (
        <Broadcast csrf={session.csrf ?? ""} />
      ) : (
        <>
          <Notice error={channels.error} />
          <div className="section-heading">
            <h2>配信中のチャンネル</h2>
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
              placeholder="名前・ジャンル"
            />
          </label>
          {current ? (
            <>
              <SitePlayer key={current.id} channel={current} />
              <button onClick={() => setSelected("")}>視聴を閉じる</button>
            </>
          ) : (
            selected && <p role="status">選択した配信は終了しました。</p>
          )}
          {channels.loading && !channels.data && (
            <p role="status">配信一覧を読み込み中…</p>
          )}
          {channels.data?.length === 0 && (
            <p>現在このノードにチャンネルはありません。</p>
          )}
          <div className="site-channels">
            {channels.data
              ?.filter((c) =>
                `${c.name} ${c.genre}`
                  .toLowerCase()
                  .includes(search.toLowerCase()),
              )
              .map((c) => (
                <article className="panel" key={c.id}>
                  <span className="muted">
                    {c.receiving ? "LIVE" : "接続待ち"} · {c.contentType}
                  </span>
                  <h3>{c.name}</h3>
                  <p>{c.genre}</p>
                  <p>{c.description}</p>
                  <button
                    onClick={() => setSelected(c.id)}
                    aria-pressed={selected === c.id}
                  >
                    「{c.name}」を視聴
                  </button>
                </article>
              ))}
          </div>
        </>
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
      <h2>自分の配信</h2>
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
