# ログイン・配信履歴

`internal/audit` が MariaDB に操作履歴を保存する。既定では無効。配信キー・過去10件のフォーム設定のJSON保存、認証セッションの保持方法は従来どおり。

## 設定・導入

```toml
[audit]
enabled = true
node_id = "mi-production"
# 省略時はconfig.tomlと同じディレクトリのsite-data/audit
spool_dir = "/config/site-data/audit"
retention_days = 0 # 無期限
```

`node_id` は再起動後も同じ、1〜64文字のASCII英数字・`.`・`_`・`-`。各稼働ノードで異なる値を使い、同じIDを別ディレクトリの複数プロセスに設定しない。`retention_days` は省略または0で無期限（自動削除なし）、1〜36500で指定日数。無効値は設定読込エラー。

接続資格情報は `PEERCAST_AUDIT_DB_HOST`、`PORT`（省略時3306）、`NAME`、`USER`、`PASSWORD`。DB名・ユーザー・ホストが未設定なら記録はスプールに滞留し、設定後の再起動で送信する。接続障害・スキーマ未適用でもログイン・配信は継続する。

0ypとは別のDB・実行ユーザーを用意する。実行時の権限は専用DBのSELECT/INSERT/UPDATE/DELETE。DDLは別の資格情報で明示実行する。

```sh
# 手元のWSL: 開発用DBの環境変数を設定したシェル
# 本番の資格情報をこのシェルに転記しない。
go run ./cmd/audit-migrate
```

Dockerイメージには `/app/audit-migrate` を同梱する。`--entrypoint /app/audit-migrate` で実行できる。通常起動はDDLを実行せず、version/checksumだけを確認する。移行コマンドはDB単位の名前付きロックを取り、適用済みchecksumが異なれば停止。同じ版の再実行は成功扱い。DDL途中失敗は未適用として残り、`CREATE TABLE IF NOT EXISTS` で再開する。既存の同名テーブルの構造までは自動修復しないので、初回は空の専用DBを使用する。

## テーブルと日時

正式DDLは [001_initial.sql](../../internal/audit/migrations/001_initial.sql)。全日時はUTCのDATETIME(6)、DB接続のtime_zoneも`+00:00`。

| テーブル | 内容 |
| --- | --- |
| `audit_events` | 変更しない原記録。発生・取込時刻、操作者、所有者、IP、成否、固定理由、相関ID、版付きJSON |
| `broadcasts` | 配信枠インスタンスごとの集約。作成時設定、最初・最後のメディア時刻、終了・中断時刻と状態 |
| `broadcast_inputs` | 配信枠×RTMP接続ごとのメディア受信区間 |
| `schema_migrations` | 適用したDDLの版、SHA-256 checksum、適用時刻 |

`broadcast_id` は今回の履歴用IDで、PCPのBroadcastIDとは別。同じChannelIDで再作成しても別履歴になる。`event_id`、`boot_id`、`session_ref`、`connection_id`、`input_id` もそれぞれ独立したランダム128bitのhex32文字。実セッションCookieとログ専用session_refは別物。boot_seqは起動内の採番。

`broadcasts` の名前・ジャンル等は作成時の値。input_genreはサイト入力、genreは公開値。変更後の値はイベントpayloadのbefore/afterに残す。文字列はUTF-8を保ち、名前・ジャンル256 bytes、詳細・コメント・コンタクトURL2048 bytes、content type32 bytesに制限する。JSON-RPCの受理する値そのものはこの記録上限で変更しない。

状態はwaiting（メディア未受信）、live（一度以上受信し枠が存続）、ended（終了を観測）、interrupted（終了時刻不明）。liveは現在も映像が到着している保証ではない。中断時にended_atは埋めず、interruption_detected_atを設定する。

## 記録するイベント

| 種類 | 発生点 |
| --- | --- |
| `auth.login` / `auth.logout` | X交換・セッション発行の成功、callback失敗・検証済みキャンセル、開発ログイン、明示ログアウト |
| `key.issue` / `key.rotate` / `key.revoke` | サイトの発行・再発行、管理APIの発行・上書き・失効 |
| `broadcast.create` | 枠作成の成功・失敗。サイトでは履歴JSON保存まで成功してから確定。取消はsetup_rollback |
| `broadcast.metadata` | ChannelInfoの変更。名前・ジャンル・詳細・コメント・URL・bitrate・content typeの旧新値 |
| `broadcast.end` | 本人停止user_stop、管理停止admin_stop、RTMP切断encoder_disconnect、正常終了server_shutdown |
| `rtmp.publish` | 発行済みキーの承認、unknown_key / rate_limited拒否。承認と映像開始は別 |
| `input.start` / `input.progress` / `input.end` | RTMPが配信チャンネルにデータを書いた区間。AVC/AACシーケンスヘッダー・AVC終了制御・メタデータは開始に数えない |
| `system.start` / `system.stop` | 記録機能の起動・正常停止 |
| `broadcast.interrupted` | 同一ノードの旧bootで未終了の枠を復旧時に照合 |
| `audit.gap` | 記録を破棄した件数。復元を意味しない |

上流PCPのリレー、個々の視聴、セッション自然失効、TrackInfoだけの変更は対象外。自動削除はリレーに限るためbroadcast.endのidle_cleanupは発生しない。認証・CSRF等で操作ハンドラーに到達しない拒否を全HTTPアクセスログとして収集する機能ではない。

操作者と所有者は別カラム。サイト管理者のID・解決済みHTTP IP・session_refは、検証済みのリクエストから同一プロセス内のcontextでJSON-RPCへ渡す。HTTPヘッダーによる身元指定は受け付けない。直接Basic認証の操作は`admin:basic`、直接loopbackは操作者不明・source=admin。HTTP IPはsite.trusted_proxiesの設定で解決し、RTMP IPはソケット由来。

同一RTMP接続が新しい枠に移れば別inputになる。旧枠に結び付いたエンコーダーの切断で新しい枠を停止しない。最終メディア時刻は到着時にメモリー更新し、次の到着時に前回通知から60秒以上ならprogressを送る。正常終了時は最後の観測値を送る。無到着時のタイマー更新はしない。複数入力の区間が重なる場合、時間の単純合計は配信時間にならない。

OAuthのcode/token、Cookie、CSRF、配信キーやそのハッシュ、RTMP PublishingName、未加工HTTPヘッダーはイベントに含めない。自由入力のうち記録するのは上記の許可項目のみ。

## 保存・再送・欠落

- 送信元は非ブロッキングの4096件キューに入れる。イベントは64KiB以下。
- 単一writerが250msごとに最大256件をまとめ、0600の一時JSONLをsync→rename→ディレクトリsync。ディレクトリは0700で作成しflockで排他する。作成済みのディレクトリ権限は運用側で管理する。
- 合計上限512MiB。1ファイルは1バッチ（最大約16MiB）。隔離ファイル・未完了一時ファイルも容量に含める。キュー満杯・ディスク書込失敗・二重起動時は新しい記録を破棄し、通常ログと状態APIで通知する。
- イベントINSERTと2集約テーブルの更新は同一トランザクション。commit済みIDは再適用しない。集約のrevisionが新しい場合だけ更新し、incompleteは解除しない。
- ファイル内の取込位置はメモリーで進め、全件取込後にファイルを削除する。再起動時は先頭から再送しても同じIDで重複しない。途中のDBタイムアウトでも次回は未取込行から進む。
- 接続タイムアウト2秒、DB処理1回の期限3秒、再試行1〜60秒。DB処理中は同じwriterのファイル同期も待つため、250msは障害時の耐久保証ではない。
- 未完了の.tmpは次の起動で回収。破損・過大行・未知版・恒久的なデータ制約エラーはスキップして正しい後続行を取り込み、元ファイルを.badとして隔離する。隔離が残れば再起動後も劣化表示する。
- ログイン・Publish失敗は種類/IP/理由ごとに1分集約。最大1024組、超過分は欠落計上。成功は集約しない。
- 欠落後のスナップショット、開始イベントより先に届いた集約、旧boot中断はincompleteになる。gapの対象bootにある集約も保守的にincompleteにする。強制終了で失った件数を正確に復元できる保証はない。
- DB復旧後、スプールを取り込んでから同じnode_idの旧bootのwaiting/liveをinterruptedにする。現在のbootは終了扱いにしない。
- 正常終了ではサイトの停止・RTMP/チャンネルの終了後、記録キューを最大5秒で同期する。DBの復旧までは待たない。

## 保持と状態確認

保持期間が0（既定）の場合、DBの期限削除と古い再送イベントの期限判定を行わず、原記録・配信履歴・受信区間を無期限に保存する。

保持期間を正の日数に設定した場合は、1分ごとに最大1000件の期限切れイベントと最大100件の終了済み枠を削除する。終了済み枠のinputを先に削除する。イベントはoccurred_at、枠はended_at / interruption_detected_atが基準。大量の期限切れデータは複数回で削除する。

保持期間を正の日数に設定した場合、期限を超えた再送イベントの原記録は取り込まない。ライフサイクルのスナップショットだけは集約・復旧照合に使い、古い終了済み集約は期限削除する。未終了枠は保持期間だけでは削除しない。.badは自動削除しない。管理者が内容・原因を確認して別保存・除去する。

管理者限定 `GET {base_path}/site/api/audit/status` はenabled / degraded / lastSuccess / pendingBytes / pendingFiles / queued / dropped / quarantinedFiles / reasonを返す。droppedは起動中のカウント、quarantinedFilesはディスク上の隔離ファイル数。履歴本文・資格情報は返さない。未認証401、一般ユーザー・開発ログイン403。履歴自体はSQLで参照する。

SQL例は [設計時の参照例](../design/site-audit.md#9-sql参照例)。DBとスプールは別々にバックアップする。保持期限で原記録が消えた後の全履歴の再生成は保証しない。
