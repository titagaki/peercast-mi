import App from "./App";
import { setAdminCSRF } from "./api";
import { useResource } from "./hooks";
import { siteAPI, type SiteSession } from "./site-api";
import { siteURL } from "./site-path";

async function loadAdminSession(signal?: AbortSignal) {
  const session = await siteAPI<SiteSession>("me", { signal });
  setAdminCSRF(session.admin ? (session.csrf ?? "") : "");
  return session;
}

export default function SiteAdmin() {
  const session = useResource(loadAdminSession, 30000);
  if (!session.error && session.data?.admin) return <App auditAvailable />;
  return (
    <main className="site-app">
      <h1>管理パネル</h1>
      {session.error ? (
        <>
          <p role="alert">{session.error}</p>
          <button onClick={session.reload}>再試行</button>
        </>
      ) : !session.data ? (
        <p role="status">管理権限を確認中…</p>
      ) : !session.data.user ? (
        <a
          href={siteURL(
            `/auth/x/start?next=${encodeURIComponent(siteURL("/admin"))}`,
          )}
        >
          X でログイン
        </a>
      ) : (
        <p role="alert">このアカウントには管理権限がありません。</p>
      )}
      <p>
        <a href={siteURL("/")}>視聴サイトへ戻る</a>
      </p>
    </main>
  );
}
