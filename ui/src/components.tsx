import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import { errorMessage } from "./hooks";

export function Notice({
  error,
  message,
}: {
  error?: string | null;
  message?: string | null;
}) {
  return (
    <>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
      {message && (
        <div className="notice success" role="status">
          {message}
        </div>
      )}
    </>
  );
}

export function TableArea({
  label,
  children,
}: {
  label: string;
  children: ReactNode;
}) {
  return (
    <div className="table-scroll" role="region" aria-label={label} tabIndex={0}>
      {children}
    </div>
  );
}

export function Refresh({
  loading,
  updated,
  onClick,
}: {
  loading: boolean;
  updated: Date | null;
  onClick: () => void;
}) {
  return (
    <div className="refresh">
      <span className="muted" role="status">
        {loading
          ? "読み込み中…"
          : updated
            ? `更新 ${updated.toLocaleTimeString("ja-JP")}`
            : "未取得"}
      </span>
      <button onClick={onClick} disabled={loading}>
        更新
      </button>
    </div>
  );
}

export function Modal({
  title,
  busy,
  onClose,
  children,
}: {
  title: string;
  busy: boolean;
  onClose: () => void;
  children: ReactNode;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const id = useId();
  useEffect(() => {
    const dialog = ref.current;
    const trigger = document.activeElement;
    dialog?.showModal();
    return () => {
      dialog?.close();
      // React may remove the dialog before native focus restoration runs.
      if (trigger instanceof HTMLElement && trigger.isConnected)
        trigger.focus();
    };
  }, []);
  return (
    <dialog
      ref={ref}
      aria-labelledby={id}
      onCancel={(event) => {
        event.preventDefault();
        if (!busy) onClose();
      }}
    >
      <header className="section-heading">
        <h2 id={id}>{title}</h2>
        <button
          type="button"
          aria-label="ダイアログを閉じる"
          disabled={busy}
          onClick={onClose}
        >
          閉じる
        </button>
      </header>
      {children}
    </dialog>
  );
}

export function Secret({ value }: { value: string }) {
  const [visible, setVisible] = useState(false);
  const [message, setMessage] = useState("");
  return (
    <div className="secret">
      <code>{visible ? value : "••••••••••••••••"}</code>
      <div className="actions">
        <button
          type="button"
          aria-pressed={visible}
          onClick={() => setVisible(!visible)}
        >
          {visible ? "隠す" : "表示"}
        </button>
        <button
          type="button"
          onClick={async () => {
            try {
              await navigator.clipboard.writeText(value);
              setMessage("コピーしました");
            } catch (error) {
              setMessage(
                `コピーできません。表示して手動でコピーしてください。${errorMessage(error)}`,
              );
            }
          }}
        >
          コピー
        </button>
      </div>
      <span className="muted" role="status">
        {message}
      </span>
    </div>
  );
}
