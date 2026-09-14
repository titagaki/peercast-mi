import { useId, type FormEvent } from "react";
import {
  broadcastChannel,
  listStreamKeys,
  setChannelInfo,
  type ChannelEntry,
} from "./api";
import { Modal, Notice } from "./components";
import { useAction, useResource } from "./hooks";

function InfoFields({ entry }: { entry?: ChannelEntry }) {
  const prefix = useId();
  const fields = [
    ["name", "チャンネル名", "text"],
    ["genre", "ジャンル", "text"],
    ["desc", "説明", "text"],
    ["comment", "コメント", "text"],
    ["url", "URL", "url"],
  ] as const;
  return (
    <div className="form-grid">
      {fields.map(([name, label, type]) => (
        <label key={name} htmlFor={`${prefix}-${name}`}>
          {label}
          {name === "name" && <span className="required">必須</span>}
          <input
            id={`${prefix}-${name}`}
            name={name}
            type={type}
            defaultValue={entry?.info[name] ?? ""}
            required={name === "name"}
          />
        </label>
      ))}
    </div>
  );
}

function infoFrom(form: HTMLFormElement) {
  const data = new FormData(form);
  const value = (name: string) => String(data.get(name) ?? "").trim();
  const name = value("name");
  if (!name) throw new Error("チャンネル名を入力してください。");
  return {
    name,
    genre: value("genre"),
    desc: value("desc"),
    comment: value("comment"),
    url: value("url"),
  };
}

export function BroadcastDialog({
  onClose,
  onCreated,
}: {
  onClose: () => void;
  onCreated: () => void;
}) {
  const keys = useResource(listStreamKeys);
  const action = useAction();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    void action.run(async () => {
      if (keys.loading || keys.error)
        throw new Error("ストリームキーを読み込み直してください。");
      const data = new FormData(form);
      const streamKey = String(data.get("streamKey") ?? "");
      if (!keys.data?.some((key) => key.streamKey === streamKey))
        throw new Error("有効なストリームキーを選択してください。");
      const input = String(data.get("bitrate") ?? "").trim();
      const bitrate = input === "" ? undefined : Number(input);
      if (
        bitrate !== undefined &&
        (!/^\d+$/.test(input) ||
          !Number.isSafeInteger(bitrate) ||
          bitrate <= 0 ||
          bitrate > 2147483647)
      )
        throw new Error(
          "ビットレートは正の整数で入力するか、自動のままにしてください。",
        );
      await broadcastChannel({
        streamKey,
        info: { ...infoFrom(form), bitrate },
      });
      onCreated();
      onClose();
    }, "配信を登録しました。");
  };
  return (
    <Modal title="配信を開始" busy={action.busy} onClose={onClose}>
      <p className="muted">
        発行済みのキーを選択し、エンコーダーから RTMP を送信してください。
      </p>
      <Notice error={keys.error} />
      {keys.loading && <p role="status">ストリームキーを読み込み中…</p>}
      {!keys.loading && keys.error && (
        <button onClick={keys.reload}>キーを再読み込み</button>
      )}
      {!keys.loading && !keys.error && keys.data?.length === 0 && (
        <p className="notice">
          キーがありません。「ストリームキー」画面で先に発行してください。
        </p>
      )}
      <form onSubmit={submit}>
        <fieldset disabled={action.busy}>
          <label>
            ストリームキー <span className="required">必須</span>
            <select
              name="streamKey"
              required
              disabled={keys.loading || !!keys.error}
              defaultValue=""
            >
              <option value="">アカウントを選択</option>
              {keys.data?.map((key) => (
                <option key={key.accountName} value={key.streamKey}>
                  {key.accountName}
                </option>
              ))}
            </select>
          </label>
          <InfoFields />
          <label>
            ビットレート (kbps)
            <input
              name="bitrate"
              type="number"
              min="1"
              max="2147483647"
              step="1"
              placeholder="自動"
            />
          </label>
          <Notice error={action.error} />
          <div className="form-actions">
            <button
              className="primary"
              type="submit"
              disabled={keys.loading || !!keys.error || !keys.data?.length}
            >
              {action.busy ? "登録中…" : "配信を開始"}
            </button>
            <button type="button" onClick={onClose}>
              キャンセル
            </button>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}

export function EditChannelDialog({
  entry,
  onClose,
  onUpdated,
}: {
  entry: ChannelEntry;
  onClose: () => void;
  onUpdated: () => void;
}) {
  const action = useAction();
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const form = event.currentTarget;
    void action.run(async () => {
      await setChannelInfo(entry.channelId, { info: infoFrom(form) });
      onUpdated();
      onClose();
    }, "チャンネル情報を保存しました。");
  };
  return (
    <Modal title="チャンネル情報を編集" busy={action.busy} onClose={onClose}>
      <p className="muted">トラック情報とビットレートは変更しません。</p>
      <form onSubmit={submit}>
        <fieldset disabled={action.busy}>
          <InfoFields entry={entry} />
          <Notice error={action.error} />
          <div className="form-actions">
            <button className="primary" type="submit">
              {action.busy ? "保存中…" : "保存"}
            </button>
            <button type="button" onClick={onClose}>
              キャンセル
            </button>
          </div>
        </fieldset>
      </form>
    </Modal>
  );
}
