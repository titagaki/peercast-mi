# 視聴サイトの Vite 開発接続修正

2026-09-14。対象は `0c9f1ed` からの開発接続・UI エラー処理。開始時の作業ツリーはクリーン。

## 原因と修正

`http://localhost:5173/watch` は表示できたが、同一オリジンの `/site/api/me` が Go へ転送されず、Vite の SPA フォールバック HTML を JSON として解析していた。このため `Unexpected token '<'` になっていた。

- Vite の `/site/` と `/auth/` を既定 `http://127.0.0.1:8080` へ転送。接続先はサーバー専用の `PEERCAST_SITE_TARGET` で変更可能。
- 管理 `/api/1` は転送せず、サイトの Host / Origin / Cookie / CSRF を保持。Go や X の認証を省略する機能は追加しない。
- 開発ポートを 5173 / strictPort にし、OAuth callback と食い違う自動ポート変更を防ぐ。
- 接続失敗は 503 と設定確認案内。HTML / 不正 JSON / ネットワークエラーも画面上で案内し、接続確認できるまでログインリンクを出さない。再取得で復帰できる。
- Go の `site.origin` と X callback の設定手順を [UI README](../../ui/README.md) と [仕様](../spec/site.md) に追記。

## 検証

- `npm run test:proxy`: 成功。実 Vite と模擬 HTTP サーバーを OS 割当ポートで起動し、API・OAuth callback の query・redirect / cookie・メディアの転送、書込ヘッダーと body の保持、管理 API 非転送、バックエンド停止時の 503 を確認。
- ブラウザーテスト: HTML 応答の案内と復帰、503 の案内を追加し、生成 FLV テストを含む全 45 ケース成功（desktop / mobile 幅 / dark）。API 応答は模擬で、上記の実 Vite 転送テストとは分けて実行。
- `npm run lint` / `npm run build`: 成功。
- Go の実装は変更なし。Go のテスト・vet は今回は再実行していない。

転送テストの初回は Vite が port 0 を 5173 に読み替え、既存サーバーと衝突して失敗した。Vite の内部 HTTP サーバーに OS 割当ポートを指定する形に修正した（未提供の `init` メソッド呼出しも除去）。ユーザーの既存プロセスは停止していない。Node の転送テストを Playwright が収集しないよう、ブラウザーテストは `*.spec.ts` に限定した。

実 Go / 実 X アカウントでの認証往復は未確認。環境変数・既存 `config.toml`・`ui/.env.local`・公開環境は変更していない。Go サイトが無効または未設定なら、Vite の再起動だけではログイン・配信は利用できない。
