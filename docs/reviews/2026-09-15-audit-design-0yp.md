# ログイン・配信履歴の設計で参考にした0ypのDB実装

2026-09-15。設計のみ。mi実装・0yp実装・本番DBの変更は行わない。

2026-09-16追記: mi側の実装・MariaDB検証は [実装検証](2026-09-16-audit-implementation.md) に記録。以下は設計時点の調査として残す。

## 参照範囲

- peercast-0yp: `/home/megan/src/go/peercast-0yp`、コミット `c278834849977b4c725eaad85626b75fe5bf7b51`、作業ツリーclean。
- peercast-mi: `37b5f4a`、設計着手時clean。
- リモート最新版との一致・本番の適用スキーマ・DBバージョンは未確認。0ypの設計書だけでなく次のコードを照合した。

| ファイル・関数 | 確認できた内容 |
| --- | --- |
| `docker/db/schema.sql`, `docs/database.md` | channel_sessions / channel_snapshots。InnoDB、utf8mb4、DATETIME、tracker_ip VARCHAR(45)、チャンネルマスターなし |
| `internal/repository/session.go`: Insert / Close / List / ListIntervalsByName | INSERT時にDBの連番ID取得、終了時に終了日時と最終メタデータ更新。画面一覧は過去7日、区間照会は過去365日 |
| 同 CloseStaleSessions | ended_atがNULLの全行をNOW()で閉じる |
| `internal/repository/snapshot.go`: Insert | (recorded_at,name)の重複でupsertしsession_id等を更新。sessionごとの記録時刻UNIQUEではない |
| 同 ListByNameAndDate / ListByNameAndDateForPage | LAGで前行の名前・ジャンル・詳細・コメント・トラック等と比較。名前と日付から履歴表示 |
| `internal/archive/recorder.go`: Start / poll / closeAllSessions | 1秒ポーリングで掲載の出入りを検知。初期snapshotは現在の10分窓先頭へ丸める。その後10分間隔。開始時に古い未終了行を閉じ、正常停止でactiveを閉じる |
| 同 pollのDBエラー処理 | 作成INSERT失敗ならactiveに追加せず次回再試行。終了UPDATE失敗でもactiveから削除。snapshot失敗はログに残す。永続再送キューはこのrecorderにない |
| `internal/config/config.go`: Load | DB_*環境変数からDSNを構築、parseTime=true / loc=Local |
| `main.go` | database/sql、repository、archiveを組み立てる。sql.Openのエラーで終了するが、sql.OpenだけをDB接続確認済みとは扱わない |
| `go.mod` | go-sql-driver/mysql v1.9.3 |
| `docker-compose.dev.yml` | schema.sqlをDB初期化ディレクトリへ配置 |

参照したスキーマ・起動・repository・archiveでは、バージョン付きマイグレーション実行や期限DELETEは確認できなかった。7日・365日のSELECT条件を、物理データの保存期限と解釈しない。別の運用ジョブによる削除・DDL適用は未確認。

## mi設計への反映

| 論点 | 0ypの確認済み実装 | miの設計案 |
| --- | --- | --- |
| 責務 | repositoryにSQL、archiveに記録ループ | 同様にSQLと記録処理を分離し、配信・認証処理へDB依存を入れない |
| テーブル分割 | セッションと時系列snapshot | 配信枠の履歴、操作イベント、実メディア受信区間に分ける |
| メタデータ | 名前等を通常カラムへ直接保存 | 同じ方式。イベント固有データだけJSON。名前のマスターは作らない |
| 対象期間 | Store掲載の出現・消失 | 枠作成、初回メディア、終了を別々に記録 |
| 識別 | DB連番セッションID、snapshotの名前+時刻UNIQUE | DB停止中にも採番するランダムID。名前・ChannelID・時刻だけで同一配信と判定しない |
| IP | PCP trackerのGlobalAddr | HTTP作成元IPとRTMP接続元IPを別に保存 |
| 文字数 | メタデータの多くはVARCHAR(255) | miの256 bytes / 2048 bytes上限を収めるVARCHAR / TEXT |
| 時刻 | 設計書はJST、DSNはloc=Local、時刻はDATETIME | UTC、DATETIME(6)、発生時刻とDB取込時刻を区別 |
| 異常終了 | 未終了全行のended_atをNOWで更新 | 同じnodeの旧bootのみ照合、終了時刻不明と検知時刻を分ける |
| 保存失敗 | 通常ログ、操作によって次回再試行 | JSONL退避・event_idによる冪等再送。欠落し得る条件を明記 |
| 保持 | 一覧7日・区間照会365日、物理削除未確認 | 初期提案90日、物理削除・遅延再送時の期限も定義 |
| 人数推移 | 10分snapshot | 今回は対象外。mi配下人数を配信全体として記録しない |
| マイグレーション | 初期化SQLの配置を確認 | schema_migrationsを設け、本番起動とDDL適用を分離 |

差異は0ypの不具合修正依頼ではなく、ログインや実受信を記録するmiの目的と「DB障害で配信を止めない」方針に合わせた選択。0ypの変更は行わない。

## 結果と検証範囲

[設計本文](../design/site-audit.md) と [DDL案](../design/site-audit-schema.sql) に反映。ソース根拠・テーブルの対応・履歴ID・時刻・NULL・再送・保管期限を文書で照合した。DB実行・実機相互接続・既存データの移行テストは行っていない。SQLは未適用の設計案であり、実装時に実MariaDBバージョンで検証する。
