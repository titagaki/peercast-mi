# 0016: オンデマンドリレーの開始要求は送信元で制限し、接続先では制限しない

- 状態: 採用
- 日付: 2026-09-14

## 背景

`/pls/` `/stream/` は未登録チャンネルを要求されると `tip` で指定された任意の host:port へ PCP 接続を開始する ([0014](0014-stream-on-demand-relay.md))。Listener には送信元 IP の判定がなく、インターネット側からの GET でもリレーを開始できた。リレーチャンネル数にも上限がなかった。

参照実装はどちらも「接続先」ではなく「誰がリレー開始を要求できるか」を制限している。

- PeerCastStation: 既定のリスナーは `LocalAccepts = All`、`GlobalAccepts = Metadata | Relay` (`AppBase.cs`)。ループバックと site-local 以外からの `/pls/` `/stream/` は `AuthMiddleware` で 403。tip は `ParseEndPoint` で形式検証するだけで allowlist はなく、リレーチャンネル数の上限もない
- peercast-yt: `/pls/` `/stream/` とも `getChannel(..., relay = isPrivate() || hasValidAuthToken())` (`servhs.cpp`)。リレー開始はプライベートホストか auth token 付きのみ。視聴自体は `ALLOW_DIRECT` フィルタで別に制御

## 決定

- `relay_request_from` を追加する。`"private"` (既定) はループバック・プライベート (RFC 1918, fc00::/7)・リンクローカルの送信元だけがリレー開始を要求でき、それ以外は 403。`"any"` は制限なし
- 制限は「未登録チャンネルのリレー開始」だけに適用し、登録済みチャンネルの視聴と `/pls/` の応答には適用しない。peercast-mi は公開ノードとして HTTP 視聴を受ける使い方があり、既存の利用範囲を変えないため
- `max_relay_channels` (既定 0 = 無制限) を追加し、`Manager.StartRelay` で判定する。上限なら `ErrRelayChannelLimit` → 503
- tip の接続先 allowlist は設けない

## 却下した案

- **tip の接続先 allowlist / プライベートアドレス禁止**: 参照実装にもなく、tracker は任意の公開ホストなので実用的な allowlist が作れない。LAN 内の tracker を指す正当な使い方も塞ぐ
- **PeerCastStation のように視聴も含めて送信元で 403 にする**: 公開ノードとしての HTTP 視聴を潰す。必要なら別途 `max_listeners` 等で対処
- **auth token (peercast-yt の `hasValidAuthToken`)**: peca-live のような呼び出し側に token を渡す経路がない。必要になれば追加

## 結果・影響

- 既定でインターネット側からはリレーを開始できなくなる。`/pls/?tip=` `/stream/?tip=` をグローバルアドレスから使っていた場合は `relay_request_from = "any"` が必要
- `Listener.RelayRequestFromAny` は既定 false (private のみ)。テストでは net.Pipe の remote がプライベート判定できないので true にする
- `lookupChannel` が失敗理由をステータス行で返すようになり、403 / 404 / 503 を呼び出し側で区別できる

## 参照

- `internal/config/config.go` (`RelayRequestFrom`, `MaxRelayChannels`)、`internal/servent/listener.go` (`lookupChannel`, `isPrivateAddr`)、`internal/channel/manager.go` (`StartRelay`)
- [spec/components.md 4.7 視聴要求](../spec/components.md)
- PeerCastStation `PeerCastStation.App/AppBase.cs` (`StartListen` の既定 accepts)、`PeerCastStation.Core/Http/AuthMiddleware.cs`; peercast-yt `core/common/servhs.cpp` (`/pls/`, `/stream/`)、`servent.cpp` (`isPrivate`)
- [0014](0014-stream-on-demand-relay.md)
