import { useEffect, useRef, useState } from "react";
import mpegts from "mpegts.js";
import type { SiteChannel } from "./site-api";
import { Notice } from "./components";

export function SitePlayer({ channel }: { channel: SiteChannel }) {
  const video = useRef<HTMLVideoElement>(null);
  const [attempt, setAttempt] = useState(0);
  const [started, setStarted] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("再生ボタンを押してください。");
  const supported =
    mpegts.isSupported() && channel.contentType.toUpperCase() === "FLV";
  useEffect(() => {
    const element = video.current;
    if (!element || !supported || !started) return;
    let active = true;
    const player = mpegts.createPlayer(
      {
        type: "flv",
        isLive: true,
        url: `/site/stream/${channel.id}`,
        withCredentials: true,
      },
      { enableWorker: false },
    );
    player.on(mpegts.Events.ERROR, () => {
      if (active) {
        setError(
          "再生できません。ログイン状態・同時視聴数・配信の状態を確認して再試行してください。",
        );
        player.unload();
      }
    });
    player.attachMediaElement(element);
    player.load();
    void player.play()?.catch(() => {
      if (active) setMessage("映像の再生ボタンを押してください。");
    });
    return () => {
      active = false;
      player.destroy();
    };
  }, [channel.id, supported, attempt, started]);
  return (
    <section className="panel site-player" aria-label="視聴プレイヤー">
      <h2>{channel.name}</h2>
      {supported ? (
        <video
          ref={video}
          controls
          playsInline
          aria-label={`${channel.name} の映像`}
          onPlaying={() => setMessage("再生中")}
          onWaiting={() => setMessage("映像を待っています…")}
          onEnded={() => setMessage("配信が終了しました。")}
        />
      ) : (
        <p role="status">
          このブラウザーまたは配信形式では再生できません。現在は FLV /
          MediaSource 対応環境が対象です。
        </p>
      )}
      <Notice error={error} />
      <p role="status">{supported && message}</p>
      <div className="actions">
        <button
          disabled={!supported}
          onClick={() => {
            setError("");
            setMessage("映像に接続しています…");
            setStarted(true);
            setAttempt((n) => n + 1);
          }}
        >
          {started ? "映像に再接続" : "再生を開始"}
        </button>
        <button
          disabled={!supported}
          onClick={() => {
            void video.current
              ?.requestFullscreen()
              .catch(() => setError("全画面表示に切り替えられませんでした。"));
          }}
        >
          全画面
        </button>
        {document.pictureInPictureEnabled && (
          <button
            disabled={!supported}
            onClick={() => {
              void video.current
                ?.requestPictureInPicture()
                .catch(() =>
                  setError("ミニプレイヤーに切り替えられませんでした。"),
                );
            }}
          >
            ミニプレイヤー
          </button>
        )}
      </div>
      {(channel.genre || channel.description) && (
        <p>
          {[channel.genre, channel.description].filter(Boolean).join(" · ")}
        </p>
      )}
      {/^https?:\/\//i.test(channel.contactUrl) && (
        <a href={channel.contactUrl} target="_blank" rel="noopener noreferrer">
          配信者の掲示板・連絡先を開く
        </a>
      )}
    </section>
  );
}
