import { useCallback, useEffect, useState } from "react";
import {
  getSettings,
  getVersionInfo,
  getYellowPages,
  type Settings,
  type VersionInfo,
  type YellowPage,
} from "./api";

export function StatusPage() {
  const [versionInfo, setVersionInfo] = useState<VersionInfo | null>(null);
  const [settings, setSettings] = useState<Settings | null>(null);
  const [yellowPages, setYellowPages] = useState<YellowPage[]>([]);
  const [error, setError] = useState<string | null>(null);

  const reload = useCallback(async () => {
    setError(null);
    try {
      const [v, s, yp] = await Promise.all([
        getVersionInfo(),
        getSettings(),
        getYellowPages(),
      ]);
      setVersionInfo(v);
      setSettings(s);
      setYellowPages(yp);
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void reload();
  }, [reload]);

  return (
    <section>
      <header className="page-header">
        <h2>Status</h2>
      </header>

      {error && <div className="error">{error}</div>}

      <h3>Version</h3>
      <table className="data-table kv-table">
        <thead>
          <tr>
            <th>Item</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Agent</td>
            <td>{versionInfo?.agentName ?? "-"}</td>
          </tr>
        </tbody>
      </table>

      <h3>Settings</h3>
      <table className="data-table kv-table">
        <thead>
          <tr>
            <th>Item</th>
            <th>Value</th>
          </tr>
        </thead>
        <tbody>
          <tr>
            <td>Server Port</td>
            <td>{settings?.serverPort ?? "-"}</td>
          </tr>
          <tr>
            <td>RTMP Port</td>
            <td>{settings?.rtmpPort ?? "-"}</td>
          </tr>
        </tbody>
      </table>

      <h3>Yellow Pages</h3>
      {yellowPages.length === 0 ? (
        <p>No yellow pages configured.</p>
      ) : (
        <table className="data-table">
          <thead>
            <tr>
              <th>ID</th>
              <th>Name</th>
              <th>URI</th>
              <th>Channels</th>
            </tr>
          </thead>
          <tbody>
            {yellowPages.map((yp) => (
              <tr key={yp.yellowPageId}>
                <td>{yp.yellowPageId}</td>
                <td>{yp.name}</td>
                <td className="mono">{yp.uri}</td>
                <td>{yp.channelCount}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </section>
  );
}
