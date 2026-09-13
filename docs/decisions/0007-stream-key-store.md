# 0007: ストリームキー管理を Manager から StreamKeyStore に分離

- 状態: 遡及記録 (決定は 2026-04 頃、記録 2026-09-13)

## 背景

ストリームキー (RTMP 認証トークン) の発行・失効・永続化は、チャンネルのライフサイクルとは独立している (キーはチャンネル停止後も残り、プロセス再起動をまたぐ)。当初は `Manager` が両方を持っていたが、ロックの粒度とファイル I/O の責務が混ざっていた。

## 決定

`channel.StreamKeyStore` に切り出す。`Manager` は `*StreamKeyStore` を保持し、`IssueStreamKey` / `RevokeStreamKey` / `IsIssuedKey` / `ListStreamKeys` を委譲する。`StreamKeyStore` は独自の `sync.RWMutex` を持ち、`stream_keys.json` への原子的な書き込み (tmp + rename) を担当する。

キーの値は呼び出し側 (pcgw-0yp 等) が生成して渡すものとし、peercast-mi 側では形式を検証しない。

## 結果・影響

- `Manager` のロックはチャンネル表だけを守ればよくなった
- `RevokeStreamKey` は放送中のチャンネルを止めない (キー失効と配信停止は別操作)

## 参照

- `internal/channel/stream_key_store.go`, `internal/channel/manager.go`
