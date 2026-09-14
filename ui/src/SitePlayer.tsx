import { siteURL } from "./site-path";
import { useEffect, useRef, useState } from "react";
import mpegts from "mpegts.js";
import type { SiteChannel } from "./site-api";
import { Notice } from "./components";
import { SiteChannelInfo } from "./SiteChannelInfo";

export function SitePlayer({ channel }: { channel: SiteChannel }) {
  const video = useRef<HTMLVideoElement>(null);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("映像に接続しています…");
  const supported =
    mpegts.isSupported() &&
    channel.contentType.toUpperCase() === "FLV" &&
    channel.playable !== false;
  useEffect(() => {
    const element = video.current;
    if (!element || !supported) return;
    let active = true;
    const player = mpegts.createPlayer(
      {
        type: "flv",
        isLive: true,
        url: siteURL(`/site/stream/${channel.id}`),
        withCredentials: true,
      },
      { enableWorker: false },
    );
    player.on(mpegts.Events.ERROR, () => {
      if (active) {
        setError(
          "映像に接続できません。配信・ログイン状態を確認し、ページを再読み込みしてください。",
        );
        player.unload();
      }
    });
    player.attachMediaElement(element);
    player.load();
    void player.play()?.catch(async (reason: unknown) => {
      if (!active) return;
      if (reason instanceof DOMException && reason.name === "NotAllowedError") {
        element.muted = true;
        try {
          await player.play();
        } catch {
          if (active) setMessage("プレイヤー内の再生ボタンを押してください。");
        }
      } else {
        setMessage("プレイヤー内の再生ボタンを押してください。");
      }
    });
    return () => {
      active = false;
      player.destroy();
    };
  }, [channel.id, supported]);
  return (
    <section className="panel site-player" aria-label="視聴プレイヤー">
      {supported && <span className="site-live-label">ライブ配信</span>}
      {supported ? (
        <video
          ref={video}
          controls
          autoPlay
          playsInline
          aria-label={`${channel.name} の映像`}
          onPlaying={(e) =>
            setMessage(
              e.currentTarget.muted
                ? "ミュートで再生中。音量はプレイヤー内で変更できます。"
                : "",
            )
          }
          onVolumeChange={(e) =>
            setMessage(e.currentTarget.muted ? "ミュート中" : "")
          }
          onPause={() => setMessage("一時停止中")}
          onWaiting={() => setMessage("映像を待っています…")}
          onEnded={() => setMessage("配信が終了しました。")}
        />
      ) : (
        <p role="status">
          {channel.playable === false
            ? "現在この配信に接続できません。"
            : "このブラウザーまたは配信形式では再生できません。FLV / MediaSource 対応環境が対象です。"}
        </p>
      )}
      <SiteChannelInfo channel={channel} detail />
      <Notice error={error} />
      {supported && message && <p role="status">{message}</p>}
      {/^https?:\/\//i.test(channel.contactUrl) && (
        <a href={channel.contactUrl} target="_blank" rel="noopener noreferrer">
          掲示板・連絡先を開く
        </a>
      )}
    </section>
  );
}
