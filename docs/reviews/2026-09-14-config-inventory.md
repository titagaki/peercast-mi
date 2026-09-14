# config.toml 項目棚卸し

2026-09-14。対象はmi `7071635` とローカルconfig.toml。開始時の作業ツリーはclean。設定の根拠は `internal/config/config.go`、適用は `main.go`、サイト既定値は `internal/site/server.go:New`、認証は `internal/jsonrpc/server.go:checkBasicAuth` と `internal/site/admin.go`。本番設定は別リポジトリの `yayaue.me/docker/peercast-mi/config.toml`。本番実機の設定監査ではない。

## 結果

全TOML項目に読み取り・使用箇所がある。Basic認証は旧機能の消し忘れではなく、直接ノード管理API用として現役。サンプル資格情報が有効なままだったためローカル設定ではコメント化した。これにより非loopbackから直接APIを利用していた場合は明示設定が必要になる。localhostの開発UIとサイトのX管理経路には不要。

未記載だったpublic_ipv4・relay_request_from・max_relay_channels・allowed_origins・サイトの任意設定をコメント付きで追加。稼働中のプロセスには再起動まで反映されない。本番設定は今回変更していない。

## トップレベル

| 項目 | 既定・役割 | 棚卸し結果 |
|---|---|---|
| rtmp_port | 1935、RTMP入力 | ローカル1945を維持 |
| peercast_port | 7144、PCP/HTTP/直接JSON-RPC | ローカル7154を維持 |
| public_ipv4 | 未設定ならOLEH観測値 | コメント例追加。自分の公開IPv4に置換して利用 |
| log_level | info | ローカルdebugを維持 |
| max_relays | 0無制限、各チャンネルの直下リレー数 | 有効 |
| max_relays_total | 0無制限、全チャンネルのリレー接続合計 | 有効 |
| max_listeners | 0無制限、各チャンネルのHTTP視聴数 | サイト全体上限とは別 |
| max_upstream_kbps | 0無制限、ノード上り帯域上限 | 有効 |
| content_buffer_seconds | 0/省略で8秒 | 既存コメント例維持 |
| channel_cleanup_minutes | 20、未使用中継の回収、0無効 | 有効 |
| relay_request_from | private、直接の未登録中継要求元 | コメント例追加 |
| max_relay_channels | 0無制限、ノード中継数上限 | コメント例追加 |
| admin_user / admin_pass | 空なら非loopbackの直接管理拒否 | サンプル値をコメント化 |
| allowed_origins | 空、直接管理APIの追加CORS許可元 | コメント例追加。認証の代替ではない |

## [[yp]]

| 項目 | 役割 |
|---|---|
| name | 起動時の-yp選択名、カタログ取得元名 |
| addr | PCP接続先。-yp省略時の掲載先は先頭。Tracker探索にも使用 |
| channels_url | HTTP番組一覧。-yp選択とは独立して全指定元を取得 |

0yp・SP・p@YPの3件を維持。一覧取得先と配信掲載先は同じ意味ではない。

## [site]

| 項目 | 既定・役割 |
|---|---|
| enabled | false。ローカル設定はtrue |
| dev_login | false。ローカルはtrue、HTTP loopback専用 |
| listen | 127.0.0.1:8080、サイトHTTPの待受 |
| origin | サイト有効時に必須。ローカルはhttp://localhost:5173 |
| base_path | 空。公開パス接頭辞。UIビルドと一致が必要 |
| ui_dir | ui/dist、配布UIの配置先 |
| rtmp_url | サイト有効時に必須、配信アプリへ案内するURL。待受を変更しない |
| admin_x_ids | 空で全員拒否。サイト管理者のX数値ID。開発ログインには管理権限なし |
| max_viewers | 省略/0で20。サイト全体の視聴接続数 |
| max_viewers_per_user | 省略/0で2。ユーザーごとの視聴接続数 |
| max_relay_channels | 省略/0で8。サイトから新規中継する際の上限 |

同名のトップレベルmax_relay_channelsは0無制限、site側は0で8となるため注意。サイト側だけ緩めてもノード全体の上限は残る。

## 認証入口

- ノードの `/api/1`: loopbackは許可、その他はadmin_user/admin_passのBasic認証。ブラウザーのOriginは別途CORSで確認。
- サイトの `<base_path>/admin/api/1`: Xセッション、admin_x_ids、Origin、CSRFで認証。ノードへの内部接続はloopback。Basic資格情報を使用しない。
- 開発サイトログイン: 一般利用者用。管理API権限なし。ローカル管理UIはノードAPIへ直接接続する。
- X資格情報: PEERCAST_X_CLIENT_ID / PEERCAST_X_CLIENT_SECRET環境変数。TOML項目ではない。

## 追加所見

ポートや数値上限などの起動時検証は項目ごとに範囲が異なる。TOMLの未知キーも現在Loadで拒否しない。今回の棚卸しでは実装を変更せず、設定例と用途の整理に限定した。

## 検証

Python tomllibでローカルTOMLをパースし、公開IPとBasic資格情報が無効（未設定）であること、既存ポート・3YP・開発サイト設定が維持されることを確認。全構造体のTOMLタグが設定例にあることを照合。git diff --check成功。Go実装は変更していないためGo全テストは再実行していない。本番適用・稼働プロセスの再起動は実施していない。
