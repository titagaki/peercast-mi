# 0005: 視聴者数・リレー数は実接続数と下流報告値の合算

- 状態: 遡及記録 (決定は 2026-03〜04、記録 2026-09-13)

## 背景

YP への bcst や下流への host アトムには `numl` (視聴者数) / `numr` (リレー数) を載せる。初期実装では固定値だったが、YP の表示やリレー先選択のスコアリングに使われるため実際の値が必要。また PeerCastStation は自ノード直下だけでなく、下流ノードが BCST HOST で報告してきた数も合算して `TotalDirects` / `TotalRelays` として扱う。

## 決定

- `Channel` が `numListeners` / `numRelays` を `AddOutput` / `RemoveOutput` で増減させ、直下の接続数として `NumListeners()` / `NumRelays()` を返す
- 下流ノードが送ってきた BCST HOST の `numl` / `numr` を session ID ごとに `nodeTable` に記録し、`TotalListeners()` / `TotalRelays()` で直下の数と合算する。下流が切断したらその分は削除する
- YP bcst と下流への host アトムには `Total*` を、自ノードの枠判定 (`IsRelayFull` 等) と Cleaner のアイドル判定には `Num*` を使う

## 結果・影響

- JSON-RPC の `localRelays` / `totalRelays` (`localDirects` / `totalDirects`) が PeerCastStation と同じ意味になる
- 下流が報告をやめても切断するまで古い値が残る。2026-09-14 再検証: 以前の「PeerCastStation も同様」という説明は今回の参照版では成立しない。PCS は `Channel.HostsView` で 180 秒を超えた報告を集計から除く。mi の集計の期限・子孫の扱いは [実装比較 D06](../reviews/2026-09-14-implementation-comparison.md) を参照

## 参照

- `internal/channel/channel.go`, `internal/channel/nodes.go`, `internal/servent/pcp.go` (`extractNodeStats`)
