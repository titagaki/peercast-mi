import { useId, type FormEvent } from "react";
import { setChannelInfo, type ChannelEntry } from "./api";
import { Modal, Notice } from "./components";
import { useAction } from "./hooks";

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
