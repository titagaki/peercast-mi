# `/mi/` 配置とコンテナ導入設定の検証

日付: 2026-09-14。ローカル作業ツリーの変更を対象とする。本番適用・参照実装との比較調査は行っていない。

## 変更

- `site.base_path` と Vite の `PEERCAST_SITE_BASE_PATH` を追加。ページ・API・映像・OAuth callback・cookie Path を同じ公開パスに合わせる。空文字のルート配置を維持する。
- Dockerfile に UI ビルドを含め、UID/GID 10001 で実行する。`/config` に設定・ノード ID・配信キーを保持する。
- ローカルの `~/src/yayaue.me` に mi の Compose サービス、Caddy `/mi`・`/mi/*` 転送、設定コピーとビルド・再起動の Ansible タスクを追加。PCP は7154、サイト8080はコンテナ内部、RTMP1945はホストloopback限定。配信入力は暫定でSSHトンネルを使う。

## 自動テスト

| 検証 | 結果 |
|---|---|
| `go vet ./...` | 成功 |
| `go test ./...` | 成功。サンドボックス内では既存のTCP待受テストが拒否されたため、外で実行 |
| `npm run lint` | 成功 |
| `npm run build` | ルート配置で成功 |
| `npm test` | desktop/mobile/dark の63件成功。生成FLVを使う既存再生テストを含む |
| `npm run test:proxy` | 既存開発プロキシの1件成功 |
| `npm test -- --config playwright.base-path.config.ts` | `/mi` 用の本番ビルドとブラウザーテスト1件成功 |

Go の新規検証では、公開接頭辞外の404、画面・アセット、末尾スラッシュ転送、サイトAPI・映像認証、CSRF、復帰先の制限、不正なbase_pathを確認した。既存OAuthテストをルートと `/mi` の両方で実行し、cookie Path、token交換時のcallback、元ページ復帰、フロー再利用拒否を確認した。Xプロバイダーはモック。

`/mi` のブラウザーテストはビルド済みUIをVite previewで配信し、APIと映像をモックする。直接視聴URLのログインリンク、一覧から視聴・配信への遷移、アセットと映像要求が `/mi/` 配下になることを検証する。実X認証・PCP中継・本番映像の検証とは区別する。

## ローカル構成・起動チェック

- ダミー認証情報で `docker compose --env-file .env.example config -q` 成功。
- ネットワーク無効の一時 Caddy コンテナで `caddy validate` 成功。差分上、既存HTTP `/yp/index.txt`、HTTPS `/yp`、その他の静的配信を維持。Caddy経由のHTTP/HTTPS機能テストは未実施。既存と同じ空白インデントへのフォーマット警告は残る。
- サンプルinventoryで `prepare.yml` と `deploy.yml` の `--syntax-check` 成功。VPS接続・checkモード・本番適用は未実施。
- `docker build --build-arg PEERCAST_SITE_BASE_PATH=/mi -t peercast-mi:mi-path-check .` 成功。
- このイメージをネットワークなしの一時コンテナとしてダミーX資格情報で起動。`/mi/` のHTML内のアセット接頭辞、`/mi/site/api/me` の匿名応答、UID10001、`/config/broadcast_id` の生成を確認。コンテナ再作成を挟んだ状態の引き継ぎは未実施。
- `git diff --check` は両リポジトリで成功。

Dockerの `npm ci` は既存と同数のaudit指摘9件を出した。依存バージョンは変更していない。既存の [保守タスク](../tasks.md#保守) を継続する。

## 未実施

本番のソース配置・X資格情報設定・callback登録・7154到達性・Ansible適用、実ログイン、OBS配信と他ノードとの相互接続。公開RTMPS終端は未構成。詳細な導入手順はインフラリポジトリの `ansible/README.md` に記載した。

判断は [ADR 0023](../decisions/0023-site-base-path.md)、動作は [サイト仕様](../spec/site.md#サブパスへの配置) を参照。

## 公開 RTMP への変更

2026-09-14 追記: ユーザーが OBS の入力を RTMP とストリームキー認証に指定したため、上記の暫定SSHトンネル構成を変更した。Compose のホスト公開を `1945:1945`、サイトの案内URLを `rtmp://yayaue.me:1945/live` に変更し、インフラの手順書にも反映した。RTMPS は今回の導入対象外。

認証は既存の `internal/rtmp/server.go` の `OnPublish` が `Manager.IsIssuedKey` で検証し、未発行のキーを拒否する。Go実装の変更はない。本番への適用・実OBS接続は未実施。

変更後の検証: `go test ./internal/rtmp -run '^TestOnPublish_'`、ダミー値によるComposeの `config -q`、両リポジトリの `git diff --check` が成功。Caddy・Ansibleの実装変更はなく、本番操作は行っていない。

## 本番RTMPポートの確定

ユーザー指定により本番は標準の1935/TCPを使う。上記1945での準備履歴に対して、Composeを `1935:1935`、本番TOMLを `rtmp_port = 1935`、OBSの案内URLを `rtmp://yayaue.me/live` に更新した。手元の開発用 `config.toml` は1945を維持する。本番への適用・待受確認は未実施。

1935への変更はダミー値のCompose `config -q` と両リポジトリの差分チェックで検証済み。Go実装の変更はなく、Goテスト・実OBS接続は今回追加実行していない。
