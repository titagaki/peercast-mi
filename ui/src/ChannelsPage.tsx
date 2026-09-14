import { useCallback, useState } from "react";
import {
  bumpChannel,
  getChannelConnections,
  getChannelRelayTree,
  getChannels,
  stopChannel,
  stopChannelConnection,
  type ChannelEntry,
  type RelayTreeNode,
} from "./api";
import { BroadcastDialog, EditChannelDialog } from "./ChannelDialogs";
import { Notice, Refresh, Secret, TableArea } from "./components";
import { useAction, useResource } from "./hooks";

function uptime(seconds: number) {
  return [
    Math.floor(seconds / 3600),
    Math.floor((seconds % 3600) / 60),
    Math.floor(seconds % 60),
  ]
    .map((value) => value.toString().padStart(2, "0"))
    .join(":");
}

function endpoint(address: string, port: number) {
  return (
    (address.includes(":") ? "[" + address + "]" : address || "未取得") +
    ":" +
    port
  );
}

function TreeNode({
  node,
  depth = 0,
}: {
  node: RelayTreeNode;
  depth?: number;
}) {
  const flags = [
    node.isTracker && "配信元",
    node.isFirewalled && "疎通未確認 / 閉鎖",
    node.isReceiving && "受信中",
    node.isRelayFull && "リレー満杯",
    node.isDirectFull && "視聴満杯",
  ]
    .filter(Boolean)
    .join("・");
  return (
    <>
      <tr>
        <td style={{ paddingLeft: Math.min(depth, 8) * 1.25 + 1 + "rem" }}>
          <span aria-hidden="true">{depth > 0 ? "└ " : ""}</span>
          <span className="mono">{endpoint(node.address, node.port)}</span>
        </td>
        <td>{node.localDirects}</td>
        <td>{node.localRelays}</td>
        <td>{node.versionString || "—"}</td>
        <td>{flags || "—"}</td>
      </tr>
      {node.children.map((child, index) => (
        <TreeNode
          key={child.sessionId || index}
          node={child}
          depth={depth + 1}
        />
      ))}
    </>
  );
}

// Remount by channelId: details and actions never belong to another selection.
function ChannelDetail({
  entry,
  onUpdated,
  onClose,
}: {
  entry: ChannelEntry;
  onUpdated: () => void;
  onClose: () => void;
}) {
  const loadConnections = useCallback(
    (signal: AbortSignal) => getChannelConnections(entry.channelId, signal),
    [entry.channelId],
  );
  const loadTree = useCallback(
    (signal: AbortSignal) => getChannelRelayTree(entry.channelId, signal),
    [entry.channelId],
  );
  const connections = useResource(loadConnections, 30000);
  const tree = useResource(loadTree, 30000);
  const action = useAction();
  const [editing, setEditing] = useState(false);
  return (
    <section className="detail-panel" aria-label="選択チャンネルの詳細">
      <header className="section-heading">
        <div>
          <span className="eyebrow">CHANNEL DETAIL</span>
          <h3>{entry.info.name || "名前なし"}</h3>
        </div>
        <div className="actions">
          {entry.status.isBroadcasting && (
            <button onClick={() => setEditing(true)}>情報を編集</button>
          )}
          <button onClick={onClose}>詳細を閉じる</button>
        </div>
      </header>
      <dl className="details">
        <dt>チャンネル ID</dt>
        <dd className="mono">{entry.channelId}</dd>
        <dt>ソース</dt>
        <dd>
          {entry.status.isBroadcasting ? (
            <Secret value={entry.status.source} />
          ) : (
            <code>{entry.status.source || "—"}</code>
          )}
        </dd>
        <dt>ジャンル</dt>
        <dd>{entry.info.genre || "—"}</dd>
        <dt>説明</dt>
        <dd>{entry.info.desc || "—"}</dd>
        <dt>コメント</dt>
        <dd>{entry.info.comment || "—"}</dd>
        <dt>URL</dt>
        <dd>{entry.info.url || "—"}</dd>
        <dt>トラック</dt>
        <dd>
          {[entry.track.creator, entry.track.title]
            .filter(Boolean)
            .join(" — ") || "—"}
        </dd>
        <dt>空き枠</dt>
        <dd>
          リレー: {entry.status.isRelayFull ? "満杯" : "空きあり"} / 視聴:{" "}
          {entry.status.isDirectFull ? "満杯" : "空きあり"}
        </dd>
      </dl>
      <div className="section-heading">
        <h4>接続一覧</h4>
        <Refresh
          loading={connections.loading}
          updated={connections.updated}
          onClick={connections.reload}
        />
      </div>
      <Notice error={connections.error} />
      <Notice error={action.error} message={action.message} />
      <TableArea label="接続一覧">
        <table>
          <thead>
            <tr>
              <th>ID</th>
              <th>種類</th>
              <th>プロトコル</th>
              <th>状態</th>
              <th>接続先</th>
              <th>送信</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {!connections.data?.length && (
              <tr>
                <td colSpan={7} className="empty">
                  {connections.loading
                    ? "接続を読み込み中…"
                    : connections.error
                      ? "接続を取得できませんでした。"
                      : "接続はありません。"}
                </td>
              </tr>
            )}
            {connections.data?.map((connection) => (
              <tr key={connection.type + "-" + connection.connectionId}>
                <td>
                  {connection.connectionId < 0 ? "—" : connection.connectionId}
                </td>
                <td>
                  {connection.type === "relay"
                    ? "リレー"
                    : connection.type === "direct"
                      ? "視聴"
                      : "ソース"}
                </td>
                <td>{connection.protocolName}</td>
                <td>{connection.status}</td>
                <td className="mono">{connection.remoteEndPoint || "—"}</td>
                <td className="nowrap">
                  {((connection.sendRate * 8) / 1000).toFixed(1)} kbps
                </td>
                <td>
                  {connection.type === "relay" && (
                    <button
                      className="danger"
                      disabled={
                        action.busy ||
                        connections.loading ||
                        !!connections.error
                      }
                      onClick={() => {
                        if (
                          !confirm(
                            "「" +
                              entry.info.name +
                              "」の接続 " +
                              (connection.remoteEndPoint ||
                                connection.connectionId) +
                              " を切断しますか？",
                          )
                        )
                          return;
                        void action.run(async () => {
                          const stopped = await stopChannelConnection(
                            entry.channelId,
                            connection.connectionId,
                          );
                          connections.reload();
                          tree.reload();
                          onUpdated();
                          if (!stopped)
                            throw new Error(
                              "対象の接続は既に終了しています。一覧を更新しました。",
                            );
                        }, "接続を切断しました。");
                      }}
                    >
                      切断
                    </button>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </TableArea>
      <div className="section-heading">
        <h4>リレーツリー</h4>
        <Refresh
          loading={tree.loading}
          updated={tree.updated}
          onClick={tree.reload}
        />
      </div>
      <Notice error={tree.error} />
      <TableArea label="リレーツリー">
        <table>
          <thead>
            <tr>
              <th>アドレス</th>
              <th>視聴数</th>
              <th>リレー数</th>
              <th>エージェント</th>
              <th>状態</th>
            </tr>
          </thead>
          <tbody>
            {!tree.data?.length && (
              <tr>
                <td colSpan={5} className="empty">
                  {tree.loading
                    ? "ツリーを読み込み中…"
                    : tree.error
                      ? "ツリーを取得できませんでした。"
                      : "ツリー情報はありません。"}
                </td>
              </tr>
            )}
            {tree.data?.map((node, index) => (
              <TreeNode key={node.sessionId || index} node={node} />
            ))}
          </tbody>
        </table>
      </TableArea>
      {editing && (
        <EditChannelDialog
          entry={entry}
          onClose={() => setEditing(false)}
          onUpdated={onUpdated}
        />
      )}
    </section>
  );
}

export function ChannelsPage() {
  const channels = useResource(getChannels, 30000);
  const entries = channels.data ?? [];
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [broadcasting, setBroadcasting] = useState(false);
  const action = useAction();
  const selected = entries.find((entry) => entry.channelId === selectedId);
  const disabled = action.busy || channels.loading || !!channels.error;
  return (
    <section>
      <header className="page-header">
        <div>
          <span className="eyebrow">LIVE NETWORK</span>
          <h2>チャンネル</h2>
          <p className="muted">
            配信とリレーを管理します。30 秒ごとに自動更新。
          </p>
        </div>
        <button className="primary" onClick={() => setBroadcasting(true)}>
          ＋ 配信を開始
        </button>
      </header>
      <div className="summary-grid">
        <div>
          <span>配信チャンネル</span>
          <strong>
            {channels.data
              ? entries.filter((entry) => entry.status.isBroadcasting).length
              : "—"}
          </strong>
        </div>
        <div>
          <span>リレーチャンネル</span>
          <strong>
            {channels.data
              ? entries.filter((entry) => !entry.status.isBroadcasting).length
              : "—"}
          </strong>
        </div>
        <div>
          <span>ローカル視聴接続</span>
          <strong>
            {channels.data
              ? entries.reduce(
                  (sum, entry) => sum + entry.status.localDirects,
                  0,
                )
              : "—"}
          </strong>
        </div>
      </div>
      <div className="section-heading">
        <p className="muted">チャンネル名を選択すると詳細を表示します。</p>
        <Refresh
          loading={channels.loading}
          updated={channels.updated}
          onClick={channels.reload}
        />
      </div>
      <Notice error={channels.error} />
      <Notice error={action.error} message={action.message} />
      <TableArea label="チャンネル一覧">
        <table className="channel-table">
          <thead>
            <tr>
              <th>チャンネル</th>
              <th>状態</th>
              <th>形式 / ビットレート</th>
              <th>
                視聴
                <br />
                <small>ローカル / 全体</small>
              </th>
              <th>
                リレー
                <br />
                <small>ローカル / 全体</small>
              </th>
              <th>稼働時間</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            {!entries.length && (
              <tr>
                <td colSpan={7} className="empty">
                  {channels.loading
                    ? "チャンネルを読み込み中…"
                    : channels.error
                      ? "チャンネルを取得できませんでした。「更新」で再試行できます。"
                      : "チャンネルはありません。配信を開始すると、ここに表示されます。"}
                </td>
              </tr>
            )}
            {[...entries]
              .sort(
                (a, b) =>
                  Number(b.status.isBroadcasting) -
                  Number(a.status.isBroadcasting),
              )
              .map((entry) => (
                <tr
                  key={entry.channelId}
                  className={selectedId === entry.channelId ? "selected" : ""}
                >
                  <td>
                    <button
                      className="channel-name"
                      aria-pressed={selectedId === entry.channelId}
                      onClick={() => setSelectedId(entry.channelId)}
                    >
                      {entry.info.name || "名前なし"}
                    </button>
                    <span className="kind">
                      {entry.status.isBroadcasting ? "配信" : "リレー"}
                    </span>
                  </td>
                  <td>
                    <span
                      className={
                        "status-pill " +
                        (entry.status.isReceiving ? "live" : "idle")
                      }
                    >
                      {entry.status.isReceiving ? "受信中" : "待機中"}
                    </span>
                  </td>
                  <td>
                    {entry.info.contentType || "未取得"}
                    <span className="kind">
                      {entry.info.bitrate
                        ? entry.info.bitrate + " kbps"
                        : "自動 / 未取得"}
                    </span>
                  </td>
                  <td>
                    {entry.status.localDirects} / {entry.status.totalDirects}
                  </td>
                  <td>
                    {entry.status.localRelays} / {entry.status.totalRelays}
                  </td>
                  <td className="mono nowrap">{uptime(entry.status.uptime)}</td>
                  <td>
                    <div className="actions">
                      <button
                        disabled={disabled}
                        title={
                          entry.status.isBroadcasting
                            ? "配信中の全チャンネルを YP に再通知"
                            : "上流への再接続を要求。下流接続は維持"
                        }
                        onClick={() =>
                          void action.run(
                            async () => {
                              await bumpChannel(entry.channelId);
                              channels.reload();
                            },
                            entry.status.isBroadcasting
                              ? "YP への再通知を要求しました（YP 未設定時は何も行いません）。"
                              : "再接続を要求しました。成立状況は受信状態で確認してください。",
                          )
                        }
                      >
                        {entry.status.isBroadcasting ? "YP 再通知" : "再接続"}
                      </button>
                      <button
                        className="danger"
                        disabled={disabled}
                        onClick={() => {
                          if (
                            !confirm(
                              "「" +
                                (entry.info.name || entry.channelId) +
                                "」を停止しますか？ 視聴・リレー接続も終了します。",
                            )
                          )
                            return;
                          void action.run(async () => {
                            await stopChannel(entry.channelId);
                            setSelectedId((current) =>
                              current === entry.channelId ? null : current,
                            );
                            channels.reload();
                          }, "チャンネルを停止しました。");
                        }}
                      >
                        停止
                      </button>
                    </div>
                  </td>
                </tr>
              ))}
          </tbody>
        </table>
      </TableArea>
      {selected && (
        <ChannelDetail
          key={selected.channelId}
          entry={selected}
          onUpdated={channels.reload}
          onClose={() => setSelectedId(null)}
        />
      )}
      {broadcasting && (
        <BroadcastDialog
          onClose={() => setBroadcasting(false)}
          onCreated={channels.reload}
        />
      )}
    </section>
  );
}
