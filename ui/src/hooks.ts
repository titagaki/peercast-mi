import { useCallback, useEffect, useRef, useState } from "react";

export function errorMessage(error: unknown): string {
  if (error instanceof TypeError && /fetch|network/i.test(error.message)) {
    return "API に接続できません。peercast-mi の起動状態と接続先・CORS 設定を確認してください。";
  }
  if (error instanceof Error && error.name === "TimeoutError") {
    return "応答がタイムアウトしました。操作が反映されているか更新して確認してください。";
  }
  return error instanceof Error ? error.message : String(error);
}

// Each effect owns one request. Cleanup aborts it and rejects late responses,
// including transports which have already completed when cancellation occurs.
export function useResource<T>(
  fetcher: (signal: AbortSignal) => Promise<T>,
  interval = 0,
) {
  const [revision, setRevision] = useState(0);
  const [result, setResult] = useState<{
    revision: number;
    data: T | null;
    error: string | null;
    updated: Date | null;
  }>({ revision: -1, data: null, error: null, updated: null });
  const reload = useCallback(() => setRevision((value) => value + 1), []);
  useEffect(() => {
    const controller = new AbortController();
    let active = true;
    void fetcher(controller.signal)
      .then((data) => {
        if (active)
          setResult({ revision, data, error: null, updated: new Date() });
      })
      .catch((error) => {
        if (active)
          setResult((previous) => ({
            ...previous,
            revision,
            error: errorMessage(error),
          }));
      });
    return () => {
      active = false;
      controller.abort();
    };
  }, [fetcher, revision]);
  useEffect(() => {
    if (!interval) return;
    const timer = setInterval(reload, interval);
    return () => clearInterval(timer);
  }, [interval, reload]);
  return { ...result, loading: result.revision !== revision, reload };
}

export function useAction() {
  const locked = useRef(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [message, setMessage] = useState<string | null>(null);
  const run = async (task: () => Promise<void>, success: string) => {
    if (locked.current) return;
    locked.current = true;
    setBusy(true);
    setError(null);
    setMessage(null);
    try {
      await task();
      setMessage(success);
    } catch (error) {
      setError(errorMessage(error));
    } finally {
      locked.current = false;
      setBusy(false);
    }
  };
  return { busy, error, message, run };
}
