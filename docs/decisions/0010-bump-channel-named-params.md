# 0010: `bumpChannel` だけ名前指定 params も受け付ける

- 状態: 採用
- 日付: 2026-09-13

## 背景

0009 の方針で JSON-RPC は位置指定配列のみとしていたが、peca-live (外部の視聴補助ツール) は PeerCastStation の `bumpChannel` を `{"channelId": "..."}` の名前指定形式で呼ぶ。peca-live から peercast-mi をそのまま使えるようにしたい。

## 決定

`bumpChannel` に限り、位置指定配列 `[channelId]` と名前指定オブジェクト `{channelId}` の両方を受け付ける。

- 既存の配列形式はそのまま (後方互換)
- 名前指定で `channelId` が無い・`null`・文字列以外 → `-32602`
- 存在しない ID → 従来どおり `-32603`
- 他のメソッド (`issueStreamKey` 等の独自 API を含む) は変更しない。YP への通知処理も変更しない

実装は `bumpChannelWithParams` で `params` の先頭が `{` かどうかで分岐し、配列なら既存の `withChannel` にそのまま委譲する。

## 結果・影響

- peca-live からの `bumpChannel` が動く
- 「位置指定のみ」の原則に例外ができた。他メソッドにも名前指定を広げるかどうかは、必要になった時点で改めて判断する

## 参照

- `internal/jsonrpc/handler_channel.go`
- [spec/api/jsonrpc.md](../spec/api/jsonrpc.md) `bumpChannel`
