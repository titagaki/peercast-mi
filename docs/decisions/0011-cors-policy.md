# 0011: JSON-RPC の CORS はループバック + 許可リストのみ

- 状態: 採用
- 日付: 2026-09-13

## 背景

JSON-RPC はループバック (127.0.0.1 等) からの要求を認証なしで通す。同梱の Web UI を別ポートの dev サーバー (`localhost:5173`) から使うため、当初は `Access-Control-Allow-Origin: *` を返していた。

しかしこの組み合わせでは、利用者がブラウザで開いた任意の Web ページが `fetch("http://127.0.0.1:7144/api/1", …)` で API を呼べてしまう。preflight (`OPTIONS`) が `*` で通り、実際の `POST` はループバック発なので無認証で実行される。`issueStreamKey` や `stopChannel` が第三者のページから叩ける状態だった。

## 決定

`Origin` ヘッダー付きの要求はオリジンを検査し、ワイルドカードは使わない。

- `Origin` なし (curl・同一オリジン): CORS ヘッダーを付けずに処理
- ループバックのオリジン (`localhost` / `127.0.0.1` / `[::1]`、任意ポート): 許可
- `allowed_origins` (config.toml) に列挙されたオリジン: 許可
- それ以外: `OPTIONS` / `POST` とも 403

許可時は要求されたオリジンをそのまま `Access-Control-Allow-Origin` に返し、`Vary: Origin` を付ける。

## 却下した案

- **ループバックでも常に Basic 認証を要求する**: 同梱 UI と pcgw-0yp の使い勝手が落ちる。Origin 検査で十分防げる
- **`Origin` 付き要求だけ認証を要求する**: 認証情報をブラウザ側 (UI) に持たせることになり、別の問題が増える

## 結果・影響

- Web UI を LAN 上の別ホストから開く場合は `allowed_origins` への追加が必要になった (README に記載)
- ループバック上で動く悪意あるプロセスからは防げない (その場合は既にローカル実行権限がある)

## 参照

- `internal/jsonrpc/server.go` (`isAllowedOrigin`), `internal/config/config.go`
- [spec/components.md 4.10](../spec/components.md)
