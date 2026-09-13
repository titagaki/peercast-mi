# 0014: `/stream/` も `/pls/` と同じ規則でオンデマンドリレーを開始する

- 状態: 採用
- 日付: 2026-09-14

## 背景

オンデマンドリレーは `/pls/<id>?tip=host:port` だけが入口で、`/stream/` は「プレイリストが指す先 = チャンネルは既にある」前提だった。プレイヤーが `/pls/` を開いてから M3U 経由で `/stream/` を開く二段構えを想定していたためで、`/stream/` はチャンネル未登録ならレスポンスも返さず切断していた。

peca-live のプレイヤー (flv.js) は `/stream/<id>.flv?tip=host:port` を直接開く。PeerCastStation (`HTTPDirectOwinApp.StreamHandler` → `RequestChannel(channelId, tip, request_relay: true)`) と peercast-yt (`/stream/` → `triggerChannel` → `findAndRelay`) はどちらも `/stream/` でリレーを開始するので、peercast-mi だけが想定外だった。

実装上は、`/stream/` が Listener の 64 バイト peek から channelId だけを切り出し、HTTP リクエスト本体は `HTTPOutputStream.run()` が読んでいたため、クエリ (`tip`) を見る場所がなかった。

## 決定

- `/pls/` と `/stream/` は同じ `parseViewerRequest` でリクエストを 1 回だけ読み、同じ `lookupChannel` でチャンネルを解決する。未登録なら `OnDemandRelay` (→ `Manager.StartRelay`) を呼ぶ。`HTTPOutputStream` はリクエストを読まない
- `/stream/` の tip なしは `/pls/` と同じく YP への tracker 問い合わせ (`relay.FindTracker`) に進む。「tip なしなら 404」にはしない。入口によって挙動が変わる理由がなく、PeerCastStation も同じだから
- 登録済みチャンネルへの要求では tip を無視する。接続先の変更やリレーの作り直しはしない (PeerCastStation の `Reconnect` 相当は、peercast-mi では停止したリレーチャンネルが Manager から消えるので不要)
- tip は `host:port` (port 1..65535) のみ受理し、それ以外は 400。登録済みチャンネルへの要求でも検証する。接続先の allowlist などの制限は新たに設けない (`/pls/` と同じ露出範囲)
- パスは `<32 hex>[.ext]` のみ受理し、余分なパス要素は 400 (PeerCastStation の `ChannelIdPattern` と同じ)
- `/stream/` の失敗は body 送信前の HTTP ステータス (400 / 404 / 503) で返す。200 送信後の初回データ待機の時間切れは切断のみで、別のエラーは書かない
- 初回データ待機は既存の `HTTPOutputStream.run()` (200 を先に送り、最大 30 秒待つ) をそのまま使う。PeerCastStation のように「データ到着まで待ってから 200 / 504」にはしない

## 却下した案

- **`/stream/` の tip なしは 404**: `/pls/` との差を新たに作るだけで利点がない
- **ヘッダー送信前にデータ到着を待って 504 を返す (PeerCastStation 方式)**: エラーが正確になるが、リレー確立中にプレイヤーが応答待ちでタイムアウトしないよう 200 を先に返す既存の判断を覆す変更になる。peca-live からは「200 が返って body が空のまま切れる」失敗として見えることは受け入れる
- **リレー生成を HTTP ハンドラーに複製する**: [0012](0012-relay-lifecycle-in-manager.md) に反する

## 結果・影響

- `OnDemandRelayFunc` の戻り値が `error` から `(*channel.Channel, error)` になり、開始後の再検索がなくなった
- `/pls/` も 400 の対象が広がった (不正な tip、`<id>/...` のような余分なパス)。以前は不正な tip はそのまま Dial に渡って失敗し 404 になっていた
- `/stream/` が未登録・上限超過のときにレスポンスなしで切断していたのが 404 / 503 になった
- リレーの最初のヘッダーより前に接続した視聴者にヘッダーが 2 回送られていたバグを修正した (自動リレーのテストで発覚。`/pls/` → `/stream/` の流れでも起きていた)
- 残るリスク: `/pls/` と同様、`/stream/` への GET だけで任意の host:port へ PCP 接続を開始でき、リレーチャンネル数に上限はない。`<video src>` などからも起動できるようになった。制限が必要なら別途判断する
- `max_upstream_kbps` が満杯でもリレーを起動してから視聴者を拒否するので、視聴者ゼロのリレーが Cleaner の掃除 (`channel_cleanup_minutes`) まで残る。`/pls/` と同じ

## 参照

- `internal/servent/listener.go` (`parseViewerRequest`, `lookupChannel`, `handleHTTPStream`), `internal/servent/http.go`, `main.go`
- [spec/components.md 4.7 視聴要求](../spec/components.md), [spec/overview.md 4.3](../spec/overview.md)
- PeerCastStation `PeerCastStation.HTTP/HTTPOutputStream.cs` (`GetChannelAsync`, `StreamHandler`), peercast-yt `core/common/servhs.cpp` (`/stream/`)
- [0012](0012-relay-lifecycle-in-manager.md)
