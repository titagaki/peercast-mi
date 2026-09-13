# 0017: HTTP 視聴は ChannelInfo を待ってから 200 を返す

- 状態: 採用
- 日付: 2026-09-14

## 背景

[0014](0014-stream-on-demand-relay.md) では「200 を先に送り、データを最大 30 秒待つ」既存方式を維持した。その結果、リレー開始直後の視聴では `icy-name` が空、`Content-Type` が既定の `video/x-flv` になり、FLV 以外のチャンネルでは Content-Type が誤る。リレー開始に失敗しても 200 が返ってから切れるので、呼び出し側が失敗を区別できない。

参照実装はどちらも 2 段階で、「info を待つ → 200 → データを待つ」になっている。

- PeerCastStation: `GetChannelAsync` が `WaitForReadyContentTypeAsync` で ContentType が揃うまで最大 10 秒待ってから 200、時間切れは 504。データはその後届き次第 (`HTTPOutputStream.cs`)
- peercast-yt: `getChannel` が playing になるまで最大 10 秒待ってから `handshakeStream` で 200 + 正しい Content-Type を返し、`waitForChannelHeader` で最大 30 秒データを待つ (`servmgr.cpp`, `servent.cpp`)

## 決定

- `HTTPOutputStream.run()` は `ChannelInfo.Type` が空なら `infoCh` の通知を最大 10 秒 (`infoWaitTimeout`) 待ってから 200 を書く。時間切れは 504 Gateway Timeout
- 200 送信後の初回データ待機 (最大 30 秒、時間切れは切断のみ) は変えない
- 判定は PeerCastStation と同じく「ContentType があるか」。ブロードキャストチャンネルは `broadcastChannel` で必ず `Type = "FLV"` が入り、リレーチャンネルは上流の chan info アトム (データより先に届く) で入る

## 却下した案

- **データ到着まで待ってから 200 / 504**: info 待ちよりはるかに長くなりうる (リレー確立 + キーフレーム待ち)。応答を待つプレイヤーがタイムアウトしないよう 200 を先に返す [0014] の判断は維持する
- **200 を先に返したまま icy-* を諦める**: Content-Type が誤ると FLV 以外のチャンネルで demuxer の選択を誤る。参照実装が揃って待っているものを省く理由がない

## 結果・影響

- リレー開始直後でも `Content-Type` / `icy-name` などが実際のチャンネル情報になる
- tracker 不在などで info が届かない失敗は 504 として呼び出し側に見える
- 既存の登録済みチャンネル視聴には影響しない (info は最初からある)

## 参照

- `internal/servent/http.go` (`run`, `infoWaitTimeout`)
- [spec/components.md 4.9](../spec/components.md)
- PeerCastStation `PeerCastStation.HTTP/HTTPOutputStream.cs` (`GetChannelAsync`); peercast-yt `core/common/servmgr.cpp` (`getChannel`)、`servent.cpp` (`waitForChannelHeader`)
- [0014](0014-stream-on-demand-relay.md)
