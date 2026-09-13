# 0002: `x-peercast-pos` による途中参加の位置決定

- 状態: 遡及記録 (決定は 2026-03 頃、記録 2026-09-13)

## 背景

下流ノードがリレー接続するとき、`GET /channel/<id>` に `x-peercast-pos: <位置>` を付けて「どこから送ってほしいか」を伝えてくる (PeerCastStation は再接続時に `Channel.ContentPosition` を送る)。上流側でこれをどう扱うか決める必要があった。

## 決定

`PCPOutputStream.handshake()` でヘッダーを読み取り、`streamLoop()` の開始位置に使う。

- `reqPos == 0` (未指定): ヘッダー位置から開始
- `reqPos >= OldestPos()`: その位置から開始
- `reqPos < OldestPos()` (バッファから溢れている): `OldestPos()` から開始
- いずれの場合も最初のキーフレーム (`ContFlags == 0`) までは送らない

自ノードが上流に再接続するときも `Channel.ContentPosition()` を送る (peercaststation-compat.md 参照)。

## 結果・影響

- 再接続した下流に重複データを送らずに済む
- `reqPos == 0` を「未指定」と同一視しているので、本当に位置 0 を要求された場合と区別できない。ストリーム開始直後は `hpos == 0` なので実害はない
- 2026-09-14 検証済み (変更不要): PeerCastStation は未指定を -1、指定を整数値として区別して保持するが、使う側は `AddContentSink` の `header.Position >= requestPos || content.Position >= requestPos` だけで、位置は非負なので -1 でも 0 でも「ヘッダー以降を全部送る」になり挙動は同じ。しかも PeerCastStation の下流は常に `x-peercast-pos:{Channel.ContentPosition}` を送り、ヘッダー未受信時の `ContentPosition` は 0 なので初回接続で 0 を送ってくる。これを「持っているものを全部」と読むのが正しい。peercast-yt は `unsigned int reqPos = 0; if (reqPos)` で peercast-mi と同じ。位置が uint32 で一周してちょうど 0 になる瞬間だけ「再開」ではなく「ヘッダーから」になるが、重複が少し出る程度で実害はない

## 参照

- `internal/servent/pcp.go` (`handshake`, `streamLoop`)
- PeerCastStation `PeerCastStation.PCP/OwinContextExtensions.cs` (`GetPCPPos`), `PCPOutputStream.cs` (`requestPos`), `PeerCastStation.Core/Channel.cs` (`AddContentSink`, `ContentPosition`), `PCPSourceStream.cs` (relay request)
- peercast-yt `core/common/servent.cpp` (`handshakeStream`)
- [spec/components.md 4.8](../spec/components.md)
