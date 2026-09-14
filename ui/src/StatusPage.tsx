import { getSettings, getVersionInfo, getYellowPages } from "./api";
import { Notice, Refresh, TableArea } from "./components";
import { useResource } from "./hooks";

const loadStatus = async (signal: AbortSignal) => {
  const [version, settings, yellowPages] = await Promise.all([
    getVersionInfo(signal),
    getSettings(signal),
    getYellowPages(signal),
  ]);
  return { version, settings, yellowPages };
};

export function StatusPage() {
  const status = useResource(loadStatus, 30000);
  return (
    <section>
      <header className="page-header">
        <div>
          <span className="eyebrow">NODE INFORMATION</span>
          <h2>ノード情報</h2>
          <p className="muted">
            接続先ノードの設定と YP 登録情報です。30 秒ごとに自動更新。
          </p>
        </div>
        <Refresh
          loading={status.loading}
          updated={status.updated}
          onClick={status.reload}
        />
      </header>
      <Notice error={status.error} />
      <div className="summary-grid">
        <div>
          <span>エージェント</span>
          <strong className="agent-name">
            {status.data?.version.agentName ?? "—"}
          </strong>
        </div>
        <div>
          <span>PeerCast ポート</span>
          <strong>{status.data?.settings.serverPort ?? "—"}</strong>
        </div>
        <div>
          <span>RTMP ポート</span>
          <strong>{status.data?.settings.rtmpPort ?? "—"}</strong>
        </div>
      </div>
      <div className="section-heading">
        <h3>Yellow Pages</h3>
        <span className="muted">チャンネルの掲載先</span>
      </div>
      <TableArea label="Yellow Pages">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>名前</th>
              <th>URI</th>
              <th>配信チャンネル数</th>
            </tr>
          </thead>
          <tbody>
            {!status.data?.yellowPages.length && (
              <tr>
                <td colSpan={4} className="empty">
                  {status.loading
                    ? "YP を読み込み中…"
                    : status.error
                      ? "YP 情報を取得できませんでした。"
                      : "YP は設定されていません。"}
                </td>
              </tr>
            )}
            {status.data?.yellowPages.map((yp) => (
              <tr key={yp.yellowPageId}>
                <td>{yp.yellowPageId}</td>
                <td>{yp.name}</td>
                <td className="mono">{yp.uri}</td>
                <td>{yp.channelCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableArea>
    </section>
  );
}
