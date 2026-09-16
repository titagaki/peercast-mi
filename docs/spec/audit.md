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

管理者限定 `GET {base_path}/site/api/audit/status` はenabled / degraded / lastSuccess / pendingBytes / pendingFiles / queued / dropped / quarantinedFiles / reasonを返す。droppedは起動中のカウント、quarantinedFilesはディスク上の隔離ファイル数。履歴本文・資格情報は返さない。未認証401、一般ユーザー・開発ログイン403。履歴自体は管理パネルの「ログ」またはSQLで参照する。

SQL例は [設計時の参照例](../design/site-audit.md#9-sql参照例)。DBとスプールは別々にバックアップする。保持期限で原記録が消えた後の全履歴の再生成は保証しない。

## 管理パネルでの閲覧

サイト管理パネル `{base_path}/admin` の「ログ」で操作ログ・配信履歴を切り替える。X管理者のみ利用可能。直接ノードAPI向けの管理UIにはこの入口を表示しない。

- 操作ログ: 発生日時、イベントの日本語名、操作者の当時の表示名とアカウント、結果、IP、理由。詳細に記録ID・DB保存時刻・所有者・ノード・配信/接続/受信区間/ログインの識別ID・補足payloadを表示。配信情報の変更は旧新の値を比較する。
- 配信履歴: 枠作成日時、名前、所有者、初回メディア受信、終了、状態、終了理由。詳細に作成時の設定、作成者・IP、最終受信・中断検知時刻とRTMP受信区間を表示。「この配信の操作ログ」で同じ履歴IDに絞り込む。
- `live` は「未終了」、`waiting` は「受信待ち」。ended_atがNULLかつ中断検知済みなら「不明（中断）」とし、終了時刻を推測しない。incompleteは「不明・欠落あり」と表示する。
- 日時は端末のタイムゾーンで表示し、画面にもタイムゾーン名を示す。初期検索は端末で7日前の0時以降。検索・条件クリア・最新取得は先頭ページに戻る。検索結果は自動更新せず、記録状態のみ30秒間隔で更新する。
- 1ページ50件で前/次ページへ移動する。読み込み中・空結果・DB失敗・記録無効を区別する。長い表は表内で横スクロールする。

### 閲覧API

全エンドポイントはGET、`{base_path}/site/api/audit` 以下。未認証401、一般利用者・開発ログイン403、不正パラメーター400、DB未設定・障害・検索混雑503。全応答にCache-Control: no-store。閲覧はDBへSELECTのみを実行する。

| パス | 検索条件 | itemsの内容 |
| --- | --- | --- |
| `/events` | from / until / actor / owner / type / outcome / ip / broadcastId / channelId | イベントの公開済み型、DB保存日時recordedAt |
| `/broadcasts` | from / until / owner / status / channelId | 作成時設定・所有者・時刻・状態等の配信集約 |
| `/broadcasts/{id}/inputs` | パスの配信履歴ID | その履歴のRTMP受信区間。該当行がなければ空配列 |

共通パラメーターはlimit（省略50、1〜100）とcursor。応答は `{ "items": [...], "nextCursor": "..." }`、次がなければnextCursor省略、空結果のitemsは `[]`。次ページの取得では同じ検索条件に返却されたcursorを付ける。cursorは取得対象・日時・IDを含む不透明な値で、別の一覧や別配信の受信区間へ流用すると400。

fromは以上、untilは未満。日時はタイムゾーン付きRFC3339を受け付けUTCへ変換する。両方ある場合はfrom < untilが必要。イベントはoccurred_at、配信はcreated_at、受信区間はstarted_atとそれぞれのIDで降順に並べる。同じ日時でもIDにより順序を固定する。取得中の新規記録や配信状態の変化を固定するスナップショット検索ではない。

文字列条件は完全一致。画面ではアカウント欄の数字のみの入力を `site:x:<数字>` に変換する。API自体はアカウント文字列をそのまま照合する。IPは正規化したリテラルのみで、開始・終了の両方と31日以内の期間が必要。未知のパラメーター・重複パラメーターは400。SQLの列名・テーブル名・並び順は指定できない。

通常起動では閲覧に別のDBプール（最大2接続）を使う。接続設定は記録用と共通の環境変数。サイト全体で同時に2検索まで受け付け、超過時は待たず503。要求ごとの期限は3秒。writerの接続は検索で使用しない。APIは許可した列とpayload型だけを返し、DBの未知JSONキーやSQLエラーの詳細を応答しない。

既存スキーマを使用するため、この閲覧機能の導入にmigrationは不要。アプリの更新・ビルド・再起動で管理パネルに「ログ」が追加される。
