# 0004: `helo.ping` によるファイアウォール疎通確認

- 状態: 遡及記録 (決定は 2026-03 頃、記録 2026-09-13)

## 背景

下流ノードがポートを開放しているかどうかは、リレー枠の管理 (0003) と YP の掲載情報 (firewalled 表示) に必要。PCP では `helo` に `ping` フィールドを入れると、相手が指定ポートに接続し直して `pcp\n` + helo / oleh を交換することで疎通を確認する仕組みがある。

## 決定

両方向で実装する。

- **自ノード → YP**: `yp/client.go` の `buildHelo()` に `ping = listenPort` を入れ、YP からの疎通確認 (`pcp\n` + helo) を `servent/listener.go` の `handlePing()` で受けて oleh + quit を返す
- **下流 → 自ノード**: `servent/pcp.go` のハンドシェイクで `helo.ping` があれば `pingHost()` で実際に接続して oleh の session ID が一致するか確認する。成功したときだけ下流の `remotePort` を設定する。`ping` がなく `port` だけあればその値を信用する
- 接続元がサイトローカルアドレスなら ping が成功しても `remotePort = 0` (外から届かないため。PeerCastStation 互換)

## 結果・影響

- `IsFirewalled()` が実測に基づくので、`MakeRelayable` の退出判定と代替ホスト案内が正確になる
- ハンドシェイク中に追加の TCP 接続 (最大 2 秒) が入る

## 参照

- `internal/servent/pcp.go` (`pingHost`, `isSiteLocal`), `internal/servent/listener.go` (`handlePing`), `internal/yp/client.go`
