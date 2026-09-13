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
- アルゴリズムの移植は peercast-yt と一致する (下記「検証」)。ただし peercast-yt / PeerCastStation と同じ ChannelID にはならない
  - peercast-yt は `encode(nullptr, name, genre, bitrate)` (`servhs.cpp` `setBroadcastIdChannelId`) または `encode(nullptr, name, loginMount, bitrate)` (HTTP push, `createChannelInfo`) を呼ぶ。peercast-mi は `name` に `"\x00" + StreamKey` を連結するので、StreamKey が空でない限り入力が異なる。C 側は `salt1[s1]` が NUL で止まるため `"\x00"` 以降を見ないが、Go 側は NUL を XOR (no-op) してキーを続けて混ぜる
  - PeerCastStation は `BroadcastChannel.CreateChannelID` で SHA512(bcid) + name + genre + source を MD5 する別方式なので、一致することはない

## 検証 (2026-09-13)

`gnuid.cpp` の `GnuID::encode` ループを C++ でそのまま写した実行ファイルと `id.ChannelID` に同じ (broadcastID, name, genre, bitrate) を与えて比較し、9 ベクトル (空文字列、16/17 バイト、マルチバイト、16 バイト超の長文、bitrate 0/255/256/1000/2000) すべて一致した。16 バイト境界でのループ回数 `(max/16+1)*16` と文字列末尾でのリセット (その回は XOR しない) が正しく移植されている。期待値は `internal/id/id_test.go` の `TestChannelID_PeercastYTVectors` に固定した。

peercast-yt から移行したときに同じ ChannelID を保ちたい場合は、StreamKey の連結をやめる設計変更が必要で、本 ADR の範囲外とする。

## 参照

- `internal/id/id.go`, `internal/channel/manager.go`
- [spec/overview.md §3.4](../spec/overview.md)
