import { useCallback, useEffect, useState } from "react";
import {
  broadcastChannel,
  bumpChannel,
  getChannelConnections,
  getChannels,
  listStreamKeys,
  stopChannel,
  stopChannelConnection,
  type ChannelConnection,
  type ChannelEntry,
  type StreamKeyEntry,
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

function BroadcastForm({
  onCreated,
}: {
  onCreated: () => void;
}) {
  const [open, setOpen] = useState(false);
  const [streamKeys, setStreamKeys] = useState<StreamKeyEntry[]>([]);
  const [streamKey, setStreamKey] = useState("");
  const [name, setName] = useState("");
  const [genre, setGenre] = useState("");
  const [desc, setDesc] = useState("");
  const [comment, setComment] = useState("");
  const [url, setUrl] = useState("");
  const [bitrateInput, setBitrateInput] = useState("自動");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (open) {
      void listStreamKeys().then(setStreamKeys);
    }
  }, [open]);

  const onSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    setError(null);
    setSubmitting(true);
    try {
      const bitrateNum = parseInt(bitrateInput, 10);
      await broadcastChannel({
        streamKey,
        info: {
          name,
          genre: genre || undefined,
          url: url || undefined,
          desc: desc || undefined,
          comment: comment || undefined,
          bitrate: Number.isFinite(bitrateNum) && bitrateNum > 0 ? bitrateNum : undefined,
        },
      });
      setOpen(false);
      setName("");
      setGenre("");
      setDesc("");
      setComment("");
      setUrl("");
      setBitrateInput("自動");
      onCreated();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSubmitting(false);
    }
  };

  if (!open) {
    return <button onClick={() => setOpen(true)}>Broadcast</button>;
  }

  return (
    <form className="broadcast-form" onSubmit={onSubmit}>
      <div className="broadcast-form-grid">
        <label>Stream Key</label>
        <select value={streamKey} onChange={(e) => setStreamKey(e.target.value)} required>
          <option value="">-- select --</option>
          {streamKeys.map((sk) => (
            <option key={sk.accountName} value={sk.streamKey}>
              {sk.accountName}
            </option>
          ))}
        </select>

        <label>Name</label>
        <input type="text" value={name} onChange={(e) => setName(e.target.value)} required />

        <label>Genre</label>
        <input type="text" value={genre} onChange={(e) => setGenre(e.target.value)} />

        <label>Description</label>
        <input type="text" value={desc} onChange={(e) => setDesc(e.target.value)} />

        <label>Comment</label>
        <input type="text" value={comment} onChange={(e) => setComment(e.target.value)} />

        <label>URL</label>
        <input type="url" value={url} onChange={(e) => setUrl(e.target.value)} />

        <label>Bitrate (kbps)</label>
        <div>
          <input
            type="text"
            list="bitrate-presets"
            value={bitrateInput}
            onChange={(e) => setBitrateInput(e.target.value)}
          />
          <datalist id="bitrate-presets">
            <option value="自動" />
            <option value="500" />
            <option value="1000" />
            <option value="2000" />
            <option value="3000" />
            <option value="5000" />
          </datalist>
        </div>
      </div>

      {error && <div className="error">{error}</div>}

      <div className="broadcast-form-actions">
        <button type="submit" disabled={submitting}>
          {submitting ? "Starting..." : "Start Broadcast"}
        </button>
        <button type="button" onClick={() => setOpen(false)}>
          Cancel
        </button>
      </div>
    </form>
  );
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
        <div style={{ display: "flex", gap: "0.5rem" }}>
          <BroadcastForm onCreated={reload} />
          <button onClick={reload} disabled={loading}>
            {loading ? "Loading..." : "Reload"}
          </button>
        </div>
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
