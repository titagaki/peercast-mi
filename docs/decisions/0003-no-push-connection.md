# 0003: push (GIV) 接続は対象外とする

- 状態: 遡及記録 (決定は 2026-03 頃、記録 2026-09-13)

## 背景

peercast-yt には、ポートを開放できない (firewalled) リレーノードに対して上流側からアウトバウンド接続する GIV プロトコルがある。peercast-mi でこれをサポートするかどうか。

## 決定

対象外とする。peercast-mi はポート開放済みのノードとして動作する前提で、下流ノードは常に peercast-mi 側に接続してくる。

代わりに、リレー枠が満杯になったときは firewalled な下流ノードを優先して切断し (`Channel.MakeRelayable`)、ポート開放済みのノードに枠を譲る (PeerCastStation と同じ)。

## 結果・影響

- firewalled な環境で peercast-mi を動かしても、下流にはリレーできない (視聴・自分の配信は可能)
- 実装が単純になる。GIV のためのアウトバウンド接続管理・push 要求の受付が不要

## 参照

- `internal/channel/channel.go` (`MakeRelayable`)
- [peercaststation-compat.md](peercaststation-compat.md) 「劣勢リレー接続の強制切断」
