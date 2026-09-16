# MariaDB監査記録の実装検証

2026-09-16。peercast-mi着手時 `d4ebe67`（clean）からの実装差分を検証。本番DB・本番サイトには接続・適用していない。

## 参照・範囲

- 0yp参照リポジトリ: `/home/megan/src/go/peercast-0yp`、`c278834849977b4c725eaad85626b75fe5bf7b51`、今回も作業ツリーcleanを確認。対象ファイル・関数は [設計時の比較](2026-09-15-audit-design-0yp.md) に記録。0ypは変更していない。
- 採用点: 明示メタデータ列、原記録と配信単位の集約、database/sql / go-sql-driver/mysql v1.9.3。意図的差異: UTC、ライフサイクル通知、JSONL再送、旧bootだけの中断照合、専用DBと明示migration。[ADR 0032](../decisions/0032-site-audit-mariadb.md)。
- `internal/audit/mysql.go`: rawとprojectionの同一transaction、revisionによる更新、明示migration、復旧・保持。
- `internal/audit/recorder.go`: 上限・再送・破損隔離・停止時同期。`run.go`: 枠と実受信区間。
- `internal/site`・`internal/jsonrpc`・`internal/channel`・`internal/rtmp`: 認証・操作者・所有者・作成確定・停止の通知。
- 既存互換性ノート、2026-09-14比較のS04/S05、プロトコル参照を確認。新しい相互接続一致の主張はしていない。旧枠のエンコーダー切断で再作成済み枠を停止しない変更は設計で合意した差異。FLV制御パケットの監査分類はメディア輸送のバイト列を変更しない。

## 通常の回帰・静的検査

- `gofmt -w`（変更したGoファイル）: 完了。
- `go mod tidy`: 完了。
- `go vet ./...`: 成功。
- `go test ./...`: 成功。
- `PEERCAST_AUDIT_TEST_ADDR=127.0.0.1:<一時ポート> go test -race ./...`: 成功。最終のRTMPメディア分類・DB未設定テスト追加後もaudit / rtmp / siteのrace検査成功。
- yayaue.meのComposeはダミー環境変数と `.env.example` で `docker compose config -q`: 成功。Ansible本番実行・イメージの本番ビルドは実施していない。

## 追加した再現チェック

### 使い捨てMariaDB

Docker公式MariaDB 11.4イメージ、実行時 `11.4.13-MariaDB-ubu2404`。image digest `sha256:65ad406b90f2d23a0d75d8cdadb075f9c421f70d8c93f020b9513e9ab9b29a78`。loopbackのランダムポート、固定の専用テストDB `peercast_mi_audit_test` を使用。本番.envは読み込んでいない。

`internal/audit/mysql_test.go: TestMariaDB`:

- 4テーブル作成と再実行、checksum一致確認・不一致拒否。
- 日本語・絵文字・IPv6・NULL終了時刻の保存。
- 同じイベントの再送で原記録が重複しない。
- 終了→古い開始の逆順投入で終了状態とrevisionが戻らない。
- projectionの文字数制約違反で原記録もrollbackする。
- 旧bootのliveをinterruptedにして実終了時刻はNULL。現bootのwaitingは維持。
- 期限切れの原イベントを復活させず集約だけを処理し、古い終了済み集約を削除。

このテストは明示的な `PEERCAST_AUDIT_TEST_ADDR` がない通常テストではskipする。DB資格情報はテスト内で専用値へ上書きする。

### キュー・スプール

`recorder_test.go`:

- DB停止を模擬したbackendで退避し、再起動して再送。
- 取込済みのファイルコピーを残して再送しても重複しない。
- 未完了.tmpの回収、過大・破損行を挟んだ後続イベント取込、.bad隔離と劣化表示。
- キュー満杯、1 byteのスプール予算、同一ディレクトリの排他失敗で欠落を可視化。
- 失敗20件の集約、期限切れライフサイクルだけの照合。
- 1回のDB期限で全バッチを処理できなくても取込位置が進む。
- 接続環境変数未設定相当（nil MySQL）でpanicせず耐久ファイルを残す。

実ディスクを満杯にした試験・電源断試験ではない。commit直後のTCP遮断を実DBに注入した試験でもない。重複再送のSQL検証と、backend／ファイルによる失敗再現を分けて評価している。

### 操作・受信経路

- `run_test.go`: 作成確定の重複防止、メディア開始・進捗・終了、作成時メタデータ保持、snapshot不変、JSON保存取消、同ChannelID別履歴。
- `site/audit_test.go`: ログインの独立session_ref、サイト入力genreと公開genre、管理者の停止と所有者の分離、ログアウト、キー・Cookie・CSRF・OAuth秘密の不混入、状態APIの権限、JSON履歴書込失敗の単一create failure、検証済みログインキャンセル。
- `rtmp/audit_test.go`: AVC/AACヘッダー・AVC終了制御を開始に数えない、実データの開始、旧接続が新枠を停止しない、同接続の枠切替で別input、server_shutdownの区別。
- `config/audit_test.go`: node_id・保持日数の設定検証。

## 未確認・運用上の残り

本番MariaDBのバージョン、mi用DB・資格情報・UFW、実行用/移行用ロールの本番権限、バックアップ復元、長時間負荷、実OBS・他PCP実装との相互接続は今回未確認。記録の閲覧UIは次段階。具体的な本番手順はyayaue.meの `docs/mi-audit.md` にまとめた。

## 本番バージョン確認後の追加検証

ユーザーのVPS実行結果で `10.11.16-MariaDB-deb12` を確認した。上記の「本番バージョン未確認」は初回検証時点の記録。

同じMariaDB 10.11.16の使い捨てDocker環境で `TestMariaDB` を再実行し成功（0.19秒）。検証したサーバー文字列は `10.11.16-MariaDB-ubu2204`、image digestは `sha256:8a99982dced50264560fd8ada91aa34c276636bc02713f01f252c540930f9915`。4テーブルのmigration・再実行・checksum、保存・重複再送・逆順投入・transaction rollback・復旧・期限削除を確認した。

本番とDBのバージョンは同じだがOSパッケージは異なる。本番へのDDL実行、専用ユーザーの権限、コンテナからの疎通はまだ確認していない。

## 自動削除の無効化

ユーザーの指定で保持期間の既定と本番用設定を無期限（retention_days=0）へ変更。`TestUnlimitedRetentionKeepsOldEventsAndNeverPrunes`で1年前の原イベントが通常保存され、期限削除が一度も呼ばれないことを確認。正の日数を指定した期限処理の既存テストも成功。`go vet ./...`、`go test ./...`成功。SQL・スキーマ変更はなく、今回の変更ではMariaDB実機テストを再実行していない。本番反映はアプリ更新と設定配置が必要。
