import { lazy, Suspense } from "react";
import { usesSiteAdmin } from "./api";
import { sitePath } from "./site-path";
import SiteApp from "./SiteApp";

const App = lazy(() => import("./App"));
const SiteAdmin = lazy(() => import("./SiteAdmin"));

export default function SiteRoot() {
  return (
    <Suspense fallback={<p role="status">画面を読み込み中…</p>}>
      {/^\/admin\/?$/.test(sitePath()) ? (
        usesSiteAdmin ? (
          <SiteAdmin />
        ) : (
          <App />
        )
      ) : (
        <SiteApp />
      )}
    </Suspense>
  );
}
