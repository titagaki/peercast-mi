# 0001: ChannelID は peercast-yt 互換の XOR アルゴリズムで生成する

- 状態: 遡及記録 (決定は 2026-03 頃、記録 2026-09-13)

## 背景

ChannelID はチャンネルを一意に識別する 16 バイトの GnuID で、同じ配信を再開したときに同じ ID になることが望ましい (YP の掲載や視聴 URL が安定する)。peercast-yt には `BroadcastID` とチャンネル情報から決定論的に導出する実装 (`gnuid.cpp:24`, `servhs.cpp:2440`) があり、SHA512+MD5 を使う別方式も存在する。

## 決定

peercast-yt の XOR ベースアルゴリズムを採用し、`internal/id.ChannelID(broadcastID, name, genre, bitrate)` として実装する。

- 入力: `BroadcastID`, `Name`, `Genre`, `Bitrate` (下位 8 bit)
- peercast-mi では `Name` に `"\x00" + StreamKey` を連結して渡す (`channel.channelIDForBroadcast`)。同じ名前で別アカウントが配信しても衝突しないようにするため
- SHA512+MD5 方式は既存ノードと非互換のため不採用
- peercast-yt の `randomizeBroadcastingChannelID` フラグは実装しない

## 結果・影響

- 同じ `streamKey` / `name` / `genre` / `bitrate` で `broadcastChannel` を呼び直すと同じ ChannelID になる
- `bitrate` が変わると ID も変わる。RTMP の onMetaData で後からビットレートを更新しても ID は変えない (API 呼び出し時点の値で確定)
- 既存実装との互換性 (同じ入力で同じ ID になるか) は未検証 → [tasks.md](../tasks.md)

## 参照

- `internal/id/id.go`, `internal/channel/manager.go`
- [spec/overview.md §3.4](../spec/overview.md)
