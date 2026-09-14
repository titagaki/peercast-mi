import { useState, type FormEvent } from "react";
import { issueStreamKey, listStreamKeys, revokeStreamKey } from "./api";
import { Notice, Refresh, Secret, TableArea } from "./components";
import { useAction, useResource } from "./hooks";

function generateStreamKey() {
  return (
    "sk_" +
    Array.from(crypto.getRandomValues(new Uint8Array(16)), (byte) =>
      byte.toString(16).padStart(2, "0"),
    ).join("")
  );
}

export function StreamKeysPage() {
  const keys = useResource(listStreamKeys);
  const action = useAction();
  const [account, setAccount] = useState("");
  const [key, setKey] = useState(generateStreamKey);
  const [visible, setVisible] = useState(false);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (!account.trim() || !key.trim()) return;
    void action.run(async () => {
      await issueStreamKey(account.trim(), key.trim());
      setAccount("");
      setKey(generateStreamKey());
      keys.reload();
    }, "ストリームキーを発行しました。");
  };
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
      <section className="panel">
        <h3>キーを発行</h3>
        <form onSubmit={submit}>
          <fieldset disabled={action.busy}>
            <div className="issue-grid">
              <label>
                アカウント名
                <input
                  autoComplete="off"
                  value={account}
                  onChange={(event) => setAccount(event.target.value)}
                  placeholder="例: my-broadcast"
                  required
                />
              </label>
              <label>
                ストリームキー
                <input
                  type={visible ? "text" : "password"}
                  autoComplete="off"
                  spellCheck={false}
                  value={key}
                  onChange={(event) => setKey(event.target.value)}
                  required
                />
              </label>
            </div>
            <div className="form-actions">
              <button
                type="button"
                aria-pressed={visible}
                onClick={() => setVisible(!visible)}
              >
                {visible ? "キーを隠す" : "キーを表示"}
              </button>
              <button type="button" onClick={() => setKey(generateStreamKey())}>
                キーを再生成
              </button>
              <button
                className="primary"
                type="submit"
                disabled={!account.trim() || !key.trim()}
              >
                {action.busy ? "処理中…" : "発行"}
              </button>
            </div>
          </fieldset>
        </form>
      </section>
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
