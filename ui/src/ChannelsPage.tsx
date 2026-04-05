import { useCallback, useEffect, useState } from "react";
import {
  bumpChannel,
  getChannelConnections,
  getChannels,
  stopChannel,
  stopChannelConnection,
  type ChannelConnection,
  type ChannelEntry,
} from "./api";

function formatUptime(seconds: number): string {
  if (!seconds) return "-";
  const h = Math.floor(seconds / 3600);
  const m = Math.floor((seconds % 3600) / 60);
  const s = seconds % 60;
  if (h > 0) return `${h}h${m}m${s}s`;
  if (m > 0) return `${m}m${s}s`;
  return `${s}s`;
}

function formatRate(bytesPerSec: number): string {
  if (!bytesPerSec) return "-";
  const kbps = (bytesPerSec * 8) / 1000;
  if (kbps >= 1000) return `${(kbps / 1000).toFixed(2)} Mbps`;
  return `${kbps.toFixed(1)} kbps`;
}

export function ChannelsPage() {
  const [entries, setEntries] = useState<ChannelEntry[]>([]);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [connections, setConnections] = useState<ChannelConnection[]>([]);
  const [connError, setConnError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setLoading(true);
    setError(null);
    try {
      setEntries(await getChannels());
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void reload();
    const timer = setInterval(() => void reload(), 30000);
    return () => clearInterval(timer);
  }, [reload]);

  const selected = entries.find((e) => e.channelId === selectedId) ?? null;

  const reloadConnections = useCallback(async (channelId: string) => {
    setConnError(null);
    try {
      setConnections(await getChannelConnections(channelId));
    } catch (e) {
      setConnError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    if (!selectedId) {
      setConnections([]);
      setConnError(null);
      return;
    }
    void reloadConnections(selectedId);
    const timer = setInterval(() => void reloadConnections(selectedId), 30000);
    return () => clearInterval(timer);
  }, [selectedId, reloadConnections]);

  const onStopConnection = async (connectionId: number) => {
    if (!selectedId) return;
    if (!confirm("Disconnect this connection?")) return;
    try {
      await stopChannelConnection(selectedId, connectionId);
      await reloadConnections(selectedId);
    } catch (e) {
      setConnError(e instanceof Error ? e.message : String(e));
    }
  };

  const onStop = async (channelId: string) => {
    if (!confirm("Stop this channel?")) return;
    try {
      await stopChannel(channelId);
      await reload();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  const onBump = async (channelId: string) => {
    try {
      await bumpChannel(channelId);
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    }
  };

  return (
    <section>
      <header className="page-header">
        <h2>Channels</h2>
        <button onClick={reload} disabled={loading}>
          {loading ? "Loading..." : "Reload"}
        </button>
      </header>

      {error && <div className="error">{error}</div>}

      <table className="data-table">
        <thead>
          <tr>
            <th>Name</th>
            <th>Status</th>
            <th>Type</th>
            <th>Bitrate</th>
            <th>Listeners</th>
            <th>Relays</th>
            <th>Uptime</th>
            <th></th>
          </tr>
        </thead>
        <tbody>
          {entries.length === 0 && !loading && (
            <tr>
              <td colSpan={8} className="empty">
                No channels.
              </td>
            </tr>
          )}
          {entries.map((c) => (
            <tr
              key={c.channelId}
              onClick={() => setSelectedId(c.channelId)}
              className={c.channelId === selectedId ? "selected" : ""}
            >
              <td>{c.info.name || "(unnamed)"}</td>
              <td>
                {c.status.status}
                {c.status.isBroadcasting ? " / broadcasting" : ""}
              </td>
              <td>{c.info.contentType}</td>
              <td>{c.info.bitrate} kbps</td>
              <td>
                {c.status.localDirects} / {c.status.totalDirects}
              </td>
              <td>
                {c.status.localRelays} / {c.status.totalRelays}
              </td>
              <td>{formatUptime(c.status.uptime)}</td>
              <td onClick={(e) => e.stopPropagation()}>
                <button onClick={() => onBump(c.channelId)}>Bump</button>{" "}
                <button onClick={() => onStop(c.channelId)}>Stop</button>
              </td>
            </tr>
          ))}
        </tbody>
      </table>

      {selected && (
        <div className="detail-panel">
          <h3>Detail</h3>
          <dl>
            <dt>Channel ID</dt>
            <dd className="mono">{selected.channelId}</dd>
            <dt>Source</dt>
            <dd className="mono">{selected.status.source}</dd>
            <dt>Genre</dt>
            <dd>{selected.info.genre || "-"}</dd>
            <dt>Description</dt>
            <dd>{selected.info.desc || "-"}</dd>
            <dt>Comment</dt>
            <dd>{selected.info.comment || "-"}</dd>
            <dt>URL</dt>
            <dd>{selected.info.url || "-"}</dd>
            <dt>Track</dt>
            <dd>
              {selected.track.creator || selected.track.title
                ? `${selected.track.creator} - ${selected.track.title}`
                : "-"}
            </dd>
            <dt>Flags</dt>
            <dd>
              {selected.status.isReceiving ? "receiving " : ""}
              {selected.status.isRelayFull ? "relay-full " : ""}
              {selected.status.isDirectFull ? "direct-full " : ""}
            </dd>
          </dl>

          <h4>Connections</h4>
          {connError && <div className="error">{connError}</div>}
          <table className="data-table">
            <thead>
              <tr>
                <th>ID</th>
                <th>Type</th>
                <th>Protocol</th>
                <th>Status</th>
                <th>Remote</th>
                <th>Send</th>
                <th></th>
              </tr>
            </thead>
            <tbody>
              {connections.length === 0 && (
                <tr>
                  <td colSpan={7} className="empty">
                    No connections.
                  </td>
                </tr>
              )}
              {connections.map((c) => (
                <tr key={`${c.type}-${c.connectionId}`}>
                  <td>{c.connectionId < 0 ? "-" : c.connectionId}</td>
                  <td>{c.type}</td>
                  <td>{c.protocolName}</td>
                  <td>{c.status}</td>
                  <td className="mono">{c.remoteEndPoint ?? "-"}</td>
                  <td>{formatRate(c.sendRate)}</td>
                  <td>
                    {c.type === "relay" && (
                      <button onClick={() => onStopConnection(c.connectionId)}>
                        Disconnect
                      </button>
                    )}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
