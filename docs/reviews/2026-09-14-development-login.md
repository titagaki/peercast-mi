# ローカル開発用ログイン

2026-09-14。X 登録前にローカルで確認するという追加依頼に対応。[ADR 0020](../decisions/0020-local-development-login.md)、[利用手順](../spec/site.md#開発用ログイン) を参照。

## 実装

`site.dev_login = true` の場合のみ固定の開発ユーザーでログインできる。Go の X 資格情報は不要。通常セッションを発行するので、その後の CSRF・所有権・視聴数制限・HTTP 映像認証は同じ実装を通る。通常モードでは開発 endpoint 自体を登録しない。

起動時の HTTP loopback origin / loopback bind 検査に加え、開発モードの全要求で実 TCP 接続元と Host を確認する。ログインは同一 Origin の POST に限定する。画面と起動ログに開発モードを表示し、X ID と開発 ID の保存名前空間を分離した。

既存 `config.toml` の `[site]` に `dev_login = true` を追加。既存の `http://localhost:5173` origin と `127.0.0.1:8080` listen は維持した。未追跡 `.env` の内容を読んだり変更したりせず、ルート `.gitignore` に `/.env` を追加した。既存の開発プロキシ修正も保持している。

## 検証

- Go テスト: X 資格情報なしの起動・ログイン、未認証 API の拒否、Cookie / CSRF、実 Manager へのキー発行、ログアウト、外部 peer / Host / Origin / Origin 欠如の拒否、転送ヘッダーを信用しないこと、GET ログインの拒否、通常モードに開発 endpoint がないこと、公開設定の起動拒否を確認。
- `go vet ./...` / `go test ./...`: 成功。
- `go test -race ./internal/site ./internal/channel ./internal/servent`: 成功。`npm run lint` / `npm run build` も成功。
- 生成 FLV テストを含むブラウザーテスト全 48 ケース成功（desktop / mobile 幅 / dark）。
- ブラウザーに開発ユーザーのログイン・注意表示・ログアウトの回帰テストを追加。API は模擬応答であり実 X ログインの試験ではない。

実際の X 認証往復、実エンコーダー → Go → ブラウザーの長時間視聴は今回未確認。Go の稼働プロセスは起動・停止していない。操作は実際のノードや公開 PCP に反映されるため、開発モードをトンネルや外部プロキシで公開しない。
