import type { SiteChannel } from "./site-api";
import spIcon from "./assets/yp-sp.png";
import tpIcon from "./assets/yp-tp.png?no-inline";
import defaultIcon from "./assets/mouneyou.png";

import { channelExplanation } from "./site-channel-text";

function elapsed(seconds: number) {
  const minutes = Math.floor(seconds / 60);
  if (minutes < 1) return "配信開始直後";
  if (minutes < 60) return `${minutes}分前から配信`;
  const hours = Math.floor(minutes / 60);
  if (hours < 24)
    return `${hours}時間${minutes % 60 ? `${minutes % 60}分` : ""}前から配信`;
  return `${Math.floor(hours / 24)}日前から配信`;
}

export function SiteChannelInfo({
  channel,
  detail = false,
}: {
  channel: SiteChannel;
  detail?: boolean;
}) {
  const name = channel.name || "名前なし";
  const icon =
    channel.yellowPage === "SP"
      ? spIcon
      : channel.yellowPage === "TP"
        ? tpIcon
        : defaultIcon;
  const explanation = channelExplanation(channel);
  const title = detail ? <h2>{name}</h2> : <h3>{name}</h3>;
  return (
    <div className={`site-channel-info${detail ? " site-channel-detail" : ""}`}>
      <img
        className="site-channel-icon"
        src={icon}
        alt=""
        width="48"
        height="48"
        loading={detail ? "eager" : "lazy"}
      />
      <div className="site-channel-copy">
        {title}
        {explanation && (
          <p className="site-channel-description">{explanation}</p>
        )}
        <p className="site-channel-stats">
          {channel.listeners >= 0 && <span>{channel.listeners}人が視聴中</span>}
          {channel.uptime != null &&
            Number.isFinite(channel.uptime) &&
            channel.uptime >= 0 && <span>{elapsed(channel.uptime)}</span>}
        </p>
      </div>
    </div>
  );
}
