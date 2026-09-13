# 0003: push (GIV) 接続は対象外とする

- 状態: 遡及記録 (決定は 2026-03 頃、記録 2026-09-13)

## 背景

peercast-yt には、ポートを開放できない (firewalled) リレーノードに対して上流側からアウトバウンド接続する GIV プロトコルがある。peercast-mi でこれをサポートするかどうか。

## 決定

対象外とする。peercast-mi はポート開放済みのノードとして動作する前提で、下流ノードは常に peercast-mi 側に接続してくる。

代わりに、リレー枠が満杯になったときは firewalled な下流ノードを優先して切断し (`Channel.MakeRelayable`)、ポート開放済みのノードに枠を譲る (PeerCastStation と同じ)。

## 再検討 (2026-09-14)

PeerCastStation も push (GIV / `PCP_PUSH`) を実装していない (該当コードは UI の表示だけ)。peercast-yt は `acceptGIV` / `givProc` で対応し、リレー自動管理では「ポート 0 で GIV 非対応」のノードを切断候補にする (`servent.cpp` `isTerminationCandidate`)。peercast-mi は PeerCastStation と同じ立場で、firewalled 下流の扱い (`MakeRelayable`) も揃えてあるため、対象外を維持する。peercast-mi 自身がポート開放済みで動く前提が変わる (firewalled 環境で下流にリレーしたくなる) ときに再度検討する。

## 結果・影響

- firewalled な環境で peercast-mi を動かしても、下流にはリレーできない (視聴・自分の配信は可能)
- 実装が単純になる。GIV のためのアウトバウンド接続管理・push 要求の受付が不要

## 参照

- `internal/channel/channel.go` (`MakeRelayable`)
- [peercaststation-compat.md](peercaststation-compat.md) 「劣勢リレー接続の強制切断」
