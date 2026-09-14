import { listStreamKeys, revokeStreamKey } from "./api";
import { Notice, Refresh, Secret, TableArea } from "./components";
import { useAction, useResource } from "./hooks";

export function StreamKeysPage() {
  const keys = useResource(listStreamKeys);
  const action = useAction();
  return (
    <section>
      <header className="page-header">
        <div>
          <span className="eyebrow">BROADCAST ACCESS</span>
          <h2>ストリームキー</h2>
          <p className="muted">
            配信に使う認証情報です。第三者に共有しないでください。
          </p>
        </div>
        <Refresh
          loading={keys.loading}
          updated={keys.updated}
          onClick={keys.reload}
        />
      </header>
      <Notice error={keys.error} />
      <Notice error={action.error} message={action.message} />
      <div className="section-heading">
        <h3>発行済みのキー</h3>
        <p className="muted">失効させても、現在の配信は停止しません。</p>
      </div>
      <TableArea label="発行済みストリームキー">
        <table>
          <thead>
            <tr>
              <th>アカウント</th>
              <th>ストリームキー</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {!keys.data?.length && (
              <tr>
                <td colSpan={3} className="empty">
                  {keys.loading
                    ? "キーを読み込み中…"
                    : keys.error
                      ? "キーを取得できませんでした。"
                      : "発行済みのキーはありません。"}
                </td>
              </tr>
            )}
            {keys.data?.map((entry) => (
              <tr key={entry.accountName}>
                <td>{entry.accountName}</td>
                <td>
                  <Secret value={entry.streamKey} />
                </td>
                <td>
                  <button
                    className="danger"
                    disabled={action.busy || keys.loading || !!keys.error}
                    onClick={() => {
                      if (
                        !confirm(
                          "「" +
                            entry.accountName +
                            "」のキーを失効させますか？ 次回の配信接続から使えなくなります。",
                        )
                      )
                        return;
                      void action.run(async () => {
                        await revokeStreamKey(entry.accountName);
                        keys.reload();
                      }, "キーを失効させました。");
                    }}
                  >
                    失効
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableArea>
    </section>
  );
}
