# 0009: JSON-RPC API は互換性より peercast-mi 独自の使いやすさを優先

- 状態: 採用
- 日付: 2026-03-30 (API 実装時の方針)

## 背景

PeerCastStation / peercast-yt にも JSON-RPC API があり、フィールド名やメソッド構成を真似れば既存クライアントを流用できる可能性がある。一方、それらには peercast-mi に存在しない概念 (yellowPageId、ソースストリームの種別など) が多く、互換性を意識すると不要なフィールドや不自然な構造が残る。

## 決定

JSON-RPC API は peercast-mi 独自に定義する。既存実装は参考程度にとどめ、フィールド名・構造・レスポンス形式は peercast-mi の実装と主要クライアント (pcgw-0yp、同梱 Web UI) の都合を優先する。

- パラメータは原則として位置指定配列のみ
- `issueStreamKey` / `revokeStreamKey` / `listStreamKeys` / `broadcastChannel` は peercast-mi 独自
  - 2026-09-14 補足: `broadcastChannel` という名前は PeerCastStation にも存在する。ここで独自なのは StreamKey を使う引数・開始モデルであり、メソッド名が独自という意味ではない ([実装比較 I03](../reviews/2026-09-14-implementation-comparison.md))
- `getChannelInfo` の返却は `{info, track}` のみ (yellowPages は返さない)

例外的に互換性を持たせる場合は個別に記録する (→ 0010)。

## 結果・影響

- PeerCastStation 向けの既存クライアントはそのままでは使えない
- API の変更が自由にできる。変更時は [spec/api/jsonrpc.md](../spec/api/jsonrpc.md) を同時に更新する

## 参照

- [spec/api/jsonrpc.md](../spec/api/jsonrpc.md)
