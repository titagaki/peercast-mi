# スマホ向け配信キーの検証

日付: 2026-09-14。対象: 895510c からの変更。

## 確認した動作

- `internal/site/stream_key_test.go`: API発行形式、小文字英字2文字＋数字4桁、ユーザー間の一意性、再発行による旧キー失効、衝突再抽選、乱数エラー時の旧キー維持、衝突の試行上限、既存の長いキー維持。
- `internal/rtmp/publish_limiter_test.go`: 接続し直しても同じIPの制限を共有、未知キー拒否、窓リセット後の正規キー受理、全体上限、追跡IP数上限、旧形式への非適用。
- 保存時の他所有者との衝突判定は `StreamKeyStore.IssueStreamKey` のロック内で行う。サイトの事前検査後に衝突してもセンチネルエラーを検出して再抽選する。

## 実行結果

- `go vet ./...`: 成功。
- `go test ./...`: 成功。
- UI `npm run lint` / `npm run build`: 成功。既存の500 kB超チャンク警告あり。
- Playwright `--grep 'broadcast page retains'`: desktop / mobile / dark の3件成功。配信画面の既存ワークフローを確認。

## 未確認

本番への反映とスマホ実機からのRTMP送信は未実施。ブラウザテストはスマホの配信アプリとの接続確認ではない。外部ノードとの互換性比較は今回の対象外。
