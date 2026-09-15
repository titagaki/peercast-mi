# 認証付き視聴・配信サイト

`site.enabled = true` で Go プロセス内の `internal/site` を起動する。無効が既定。管理コンソールとは別の HTTP 入口を使い、利用者向けの一覧は `/`、視聴は `/channels/<32桁ID>`、配信は `/broadcast`、管理 UI は `/admin`。管理 JSON-RPC は指定されたX管理者だけがサイト経由で利用できる。

## 管理パネル

`site.admin_x_ids = ["123456789"]` のように管理者のX数値IDを明示する。省略・空配列ではサイト管理権限を誰にも与えない。数字のみの1〜32文字を受け付け、無効な値は起動時に拒否する。表示名・ユーザー名による判定はしない。開発ログインには管理権限を与えない。変更はGo再起動で反映される。

`/admin` のUIはXログイン状態と `/site/api/me` の `admin` を確認し、未ログインならログインリンク、一般利用者なら権限不足を表示する。管理者には既存のノード管理画面を表示する。認証後の復帰先に `/admin` と `/admin/` も受理する。

管理操作は同一オリジンの `POST /admin/api/1` へJSON-RPCを送る。全メソッドで有効なセッション、許可リストのX ID、設定と一致するOrigin、セッションの `X-CSRF-Token` が必要。未ログイン・期限切れは401、一般利用者・CSRF不一致は403。Content-Typeはapplication/json（それ以外415）、クエリは不可（400）、本文上限は1MiB。認証情報を転送せず、固定のloopbackノードAPI `/api/1` へ中継する。接続失敗は502。利用者指定の転送先は受け付けない。

この入口は管理JSON-RPCの全メソッドを管理者に許可する。Xログアウト・セッション期限切れで利用不可になる。ノードの直接 `/api/1` のBasic認証・loopback規則は変更しない。サイト `/api/1` は引き続き404。

本番ビルドの管理UIはサイトの公開接頭辞を含む `/admin/api/1` を既定の接続先とする。Vite開発時の既定は従来の `http://127.0.0.1:7144/api/1`。`VITE_PEERCAST_ENDPOINT` を明示すれば単独の管理UIとしてその接続先を使う。サイト入口を開発時に使う場合は `/mi/admin/api/1` などの相対パスを指定し、そのパスもGoサイトへ転送する。

## サブパスへの配置

`site.base_path` は既定の空文字でルート配置、`"/mi"` なら `/mi/` 配置になる。以下のページ・API・認証パスは base_path を省略した表記で、設定時はすべてその接頭辞を付ける。`site.origin` は `https://yayaue.me` のようなオリジンのままとする。

base_path は `/` から始まり、各要素が英数字・`_`・`-` のみのパス。末尾 `/`、空要素、ドット要素、クエリ・エスケープ表現を受け付けない。`/mi` 自体への GET は `/mi/` へ 308 を返す。設定した接頭辞の外ではサイトのルートを提供しない。

UI は `ui/` で `PEERCAST_SITE_BASE_PATH=/mi npm run build` として同じパスでビルドする。開発サーバーにも同じ環境変数を指定する。リンク、アセット、API、映像 URL にこのパスを使う。X の callback 登録は `https://yayaue.me/mi/auth/x/callback`。ログイン後の `next` は接頭辞を含むサイト内ページのみ受理し、その他は `/mi/` に戻す。セッション・OAuth cookie の Path は `/mi/`（ルート配置時は `/`）。削除時も同じ Path を使う。Origin・CSRF 検証は引き続きオリジン単位。

リバースプロキシは `/mi` と `/mi/*` をサイト専用待受へ転送し、接頭辞を削らない。管理JSON-RPCの直接入口は転送せず、管理者用 `/admin/api/1` はサイト側の認証を経由する。Dockerfile は UI をビルドして同梱し、build arg `PEERCAST_SITE_BASE_PATH`（既定空文字）でパスを指定する。実行ユーザーは UID/GID 10001。`/config` はこのユーザーが書き込めるようにマウントし、設定と同じディレクトリの `broadcast_id`・`stream_keys.json`・`site-data/broadcast-history/` を永続化する。

## 有効化

以下は通常の X 認証モードの手順。X 登録前のローカル確認は次の「開発用ログイン」を使う。

### 開発用ログイン

既存 `[site]` に `dev_login = true` を設定し、`listen = "127.0.0.1:8080"`、`origin = "http://localhost:5173"` とする。Go をビルド・再起動して `ui/` で `npm run dev` を実行し、`http://localhost:5173/` の「開発用ユーザーでログイン」を押す。X の Client ID / Secret、callback 登録、`.env` の読込は不要。

- `site.dev_login` は既定 false。明示的な true の場合だけ X 資格情報を不要にする。
- origin は HTTP の `localhost` / loopback IP のみ。listen は loopback IP リテラルのみ。全インターフェース・LAN 待受・公開 / HTTPS origin は起動時に拒否する。
- 全サイト要求で実 TCP 接続元の loopback と Host が設定 origin の host:port に一致することを確認する。`X-Forwarded-For` で loopback と偽装できない。
- `POST /site/api/dev-login` は設定と一致する Origin が必須。成功応答は `{ok:true}` とセッション Cookie。GET は不可。X の `/auth/x/start`・callback は登録しない。
- `GET /site/api/me` の `devLogin` が true になり、未ログイン画面に開発ログインボタン、ログイン前後に開発モードの注意を表示する。通常モードでは devLogin は false、開発 endpoint は 404。
- 開発ユーザーは `{id:"dev-local",name:"ローカル開発ユーザー"}`。配信キーの所有者名は `site:dev:local`。全ブラウザーで同じ開発ユーザーを使う。実際のキー保存・配信・公開 PCP 中継に反映される。
- 通常の Cookie セッション・期限・CSRF・視聴枠・本人の配信操作・HTTP 視聴入口の認証は維持する。ログアウト後はログインし直す必要がある。

**開発モードの Go サイトや Vite をトンネル / 外部プロキシで公開しない。** 公開時は `dev_login = false` に戻して再起動し、X の資格情報と HTTPS origin / callback を設定する。Go の再起動で開発セッションは失効するが、保存された開発キーは残る。理由は [ADR 0020](../decisions/0020-local-development-login.md)。

### 通常モードの設定

既存 `config.toml` に次のテーブルを追加する。URL とポートは運用環境に合わせる。秘密情報は TOML / `VITE_*` には入れない。

```toml
[site]
enabled = true
listen = "127.0.0.1:8080"
origin = "https://live.example.com"
ui_dir = "ui/dist"
rtmp_url = "rtmps://live.example.com/live"
max_viewers = 20
max_viewers_per_user = 2
```

| 設定 | 動作 |
|:--|:--|
| `listen` | サイト HTTP の bind 先。省略時 `127.0.0.1:8080` |
| `origin` | ブラウザーから見える公開オリジン。必須。パスなしの HTTPS。開発時だけ loopback の HTTP も可 |
| `admin_x_ids` | サイト管理者のX数値IDの配列。既定空配列 |
| `base_path` | 公開パスの接頭辞。既定空文字、例 `/mi`。UI のビルド設定と一致させる |
| `ui_dir` | Vite build 出力。省略時 `ui/dist`。相対パスはプロセスの作業ディレクトリ基準 |
| `broadcast_history_dir` | 本人の配信設定履歴の保存先。省略時は設定ファイルと同じディレクトリの `site-data/broadcast-history`。明示した相対パスは作業ディレクトリ基準。単一プロセスで使用し、永続領域として保全する |
| `rtmp_url` | 利用者へ表示する RTMP(S) アプリケーション URL。必須。Go の待受先を変更する設定ではない |
| `max_viewers` | サイト全体の同時メディア接続数。省略 / 0 は 20。負数は起動エラー |
| `max_viewers_per_user` | X ID 単位の同時メディア接続数。省略 / 0 は 2。負数は起動エラー |
| `max_relay_channels` | サイトで新規中継を開始できるノード内中継数の上限。省略 / 0 は 8。負数は起動エラー。既存チャンネルの視聴・公開 PCP の通常処理はこの上限では拒否しない |

プロセス環境変数 `PEERCAST_X_CLIENT_ID` と `PEERCAST_X_CLIENT_SECRET` を設定する。サービスマネージャーの保護された環境ファイル等で渡し、リポジトリに保存しない。有効モードで未設定なら起動エラーとなる。X のアプリは OAuth 2.0 の confidential Web App とし、callback を **`https://live.example.com/auth/x/callback`** に完全一致で登録する。

フロントエンドをビルドし、環境変数を設定したプロセスで起動する。

```sh
cd ui
npm ci
npm run build
cd ..
go build -o peercast-mi .
./peercast-mi
```

公開 HTTPS サーバーから **サイトの 8080 番**へ同一オリジンでリバースプロキシする。映像ストリームを全量バッファーしないようプロキシのストリーミング設定とタイムアウトを調整する。node の 7144 番（設定変更時はその番号）を汎用 HTTP バックエンドとして公開しない。特に `/api/1` を localhost 宛てに転送すると管理 API の loopback 認証例外を公開してしまう。

`rtmps://` を表示する場合は実際に RTMPS 終端を用意し、Go の `rtmp_port` へ転送する。Go 本体は RTMP 平文受信。`rtmp://` での直接公開も可能で、発行済みストリームキーを Publish 時に検証する。RTMP 接続自体に TLS 暗号化はない。公開 PCP ポートはそのまま利用できる。

ブラウザーで `https://live.example.com/` を開く。開発時も `npm run dev` だけでは認証 API は起動しない。ビルド済み UI を Go サイトで配信する方法なら同一オリジンになる。開発用 HTTP origin は X 側の callback 登録可否も確認する。

### Vite 開発サーバー経由

`ui/` で `npm run dev` を実行すると、`http://localhost:5173/` の画面から `/site/` と `/auth/` を Go サイトへ転送する。転送先の既定は `http://127.0.0.1:8080`。変更は `ui/.env.local` の `PEERCAST_SITE_TARGET` で行い、Vite を再起動する。管理 API `/api/1` は転送しない。

この構成では Go の `site.origin = "http://localhost:5173"`、X の callback は `http://localhost:5173/auth/x/callback` とする。Go の `site.listen` は `127.0.0.1:8080` のままでよい。Origin・Host・Cookie・CSRF ヘッダーを保持し、認証検査を省略しない。X の実資格情報は引き続き必要で、Vite は認証サーバーを代替しない。設定例は [UI 開発手順](../../ui/README.md#localhost5173-での開発設定)。

開発ポートは既定 5173、使用中なら自動で別ポートへ移動せず失敗する。サイト API が未起動ならプロキシは 503 と接続先の確認案内を返す。HTML 等の不正な API 応答は画面で設定エラーとして表示し、接続確認が失敗している間はログインリンクを表示しない。

## 認証と入口

- X OAuth 2.0 Authorization Code + PKCE S256、scope `tweet.read users.read`。メール・投稿・フォロー・refresh token の権限は要求しない。本人情報 API の結果 ID を使い、X の認証バッジは判定しない。
- OAuth フローはランダムな cookie と state / verifier の組合せで検証し、一回消費・5 分期限。未完了フローは最大 1000。
- Web セッションはランダム cookie とサーバー側レコード。HttpOnly / SameSite=Lax、HTTPS origin では Secure。固定 12 時間、最大 10000 セッション。再起動で失効。
- OAuth access token は本人確認にだけ使用し、保存・ブラウザー返却はしない。表示名はログイン時に取得し、所有者は X ID で識別する。
- 書込要求はセッションに加え、設定 origin と一致する `Origin` および `X-CSRF-Token` が必要。不正 Origin の要求には 403。CORS は提供しない。
- 全レスポンスに `Cache-Control: no-store`、`Referrer-Policy: no-referrer` 等を付ける。
- サイト有効時はノードの `/stream/`、`/pls/` に内部 Bearer token が必要で、欠如 / 不一致は 403。loopback も例外にしない。トークンは起動ごとに生成され、サイトのプロキシ以外には渡さない。
- `/channel/` の PCP 中継と PCP ping はこの認証対象外。PCP を経由した他ノードでの視聴は意図どおり可能。

## 利用者 API

JSON の成功応答、失敗時は HTTP ステータスとテキストメッセージ。JSON-RPC とは別 API。未認証は `/me` 以外 401。管理用パラメータ・キー・アカウント名を一般利用者から受け付けない。

| Method / Path | 入力 | 成功応答 |
|:--|:--|:--|
| `GET /auth/x/start` | 任意の `next`（サイト内復帰先） | X 認可画面へ 302 |
| `GET /auth/x/callback` | X の code / state | セッション作成、検証済みの元ページへ 303（既定 `/`）。拒否・期限切れ・state 不一致 400、外部 API 失敗 502 |
| `GET /site/api/me` | なし | 未認証 `{user:null}`、認証済み `{user:{id,name},csrf,admin,devLogin}` |
| `POST /site/api/logout` | CSRF | `{ok:true}`。このセッションとその視聴接続を終了 |
| `GET /site/api/channels` | なし | `ChannelView[]`。YP のHTTP番組一覧に掲載されたチャンネルのみ |
| `GET /site/api/directory` | なし | `{channels:ChannelView[],sources:YPStatus[]}`。画面が利用する一覧・取得状態 |
| `GET /site/api/channels/{id}/comments` | 任意の `thread`（数値 ID） | `{supported,threadId,threadTitle,commentCount,threads,comments}`。認証済み掲示板閲覧 |
| `GET /site/api/broadcast` | なし | `{streamKey,rtmpUrl,channel:ChannelView|null,history:BroadcastHistoryEntry[]}`。本人のキー・履歴のみ。履歴読込失敗500 |
| `GET /site/api/broadcast/board` | `url` 1件（最大2048 bytes） | 認証必須。`{supported,boardTitle,boardUrl,thread,threadUrl,latestThread,latestThreadUrl,threadError?}`。不正クエリ400、取得失敗502 |
| `POST /site/api/key` | CSRF | `{streamKey}`。生成 / 再発行。本人の配信枠がある間は 409 |
| `POST /site/api/broadcast` | CSRF、JSON `{name,genre,description,comment?,contactUrl?,bitrate?}` | `ChannelView`。1 ユーザー 1 枠。未発行キー / 既存枠は 409。履歴読込・保存失敗500（作成枠は取り消す） |
| `DELETE /site/api/broadcast` | CSRF | `{ok:true}`。本人の枠だけ停止。枠なしでも成功 |
| `GET /site/stream/{id}` | 32 hex のチャンネル ID | 認証付き HTTP ストリーム。任意クエリは 400、一覧外 / 接続不可 404、視聴枠 / サイト中継数の上限 429、中継生成失敗 503 |

`ChannelView` は `{id,name,genre,description,comment?,uptime?,contactUrl,contentType,bitrate?,receiving,listeners,yellowPage?,playable?}`。ID は小文字 hex。配信キー・ソース URL・接続先 IP を含めない。自ノードに存在するだけのリレー・配信枠は一覧対象外。`yellowPage` は取得元の名前、`playable:false` はサイトが接続できる tracker がないことを表す（再生形式の対応判定とは別）。視聴者数の `-1` は非表示 / 不明。任意 `tip` 指定や汎用リレー生成 API はない。

### YP の番組一覧

各 `[[yp]]` に `channels_url` を追加する。既存の `addr` は PCP 掲載接続用であり、一覧 URL は独立して指定する。設定変更は Go 再起動で反映される。

```toml
[[yp]]
name = "0yp"
addr = "pcp://yayaue.me/"
channels_url = "https://yayaue.me/yp/index.txt"

[[yp]]
name = "SP"
addr = "pcp://bayonet.ddo.jp:7146/"
channels_url = "http://bayonet.ddo.jp/sp/index.txt"
```

`channels_url` は管理者が管理する HTTP(S) の index.txt URL（userinfo / fragment 不可）。未設定の YP には HTTP 要求を送らない。`-yp` の選択にかかわらず設定された全一覧を取得する。ブラウザーの認証情報やプロキシ環境変数を取得通信に渡さない。管理 JSON-RPC `updateYPChannels` と共通の 60 秒キャッシュを使用する。取得全体は 5 秒以内、同時 HTTP 取得は最大 4。詳細な解析・サイズ上限は [JSON-RPC 仕様](api/jsonrpc.md#updateypchannels) を参照。

サイト一覧は YP 設定順・掲載順。同じ ID は最初の YP を採用し、ゼロ ID の告知行は除外する。自ノードに同じ ID がある場合は受信状態を反映し、取得済みのローカル情報を優先する。リレー接続直後で CHAN_INFO 未取得の場合は YP の名前・形式を保持する。自ノードだけにあるチャンネルは追加しない。取得先未設定・取得結果が空の場合もローカル一覧へフォールバックしない。本人の配信ページと管理パネルでは引き続きローカルの配信枠を確認できる。

`YPStatus` は `{name,configured,error?,stale,updatedAt?}`。`updatedAt` は最終成功の RFC3339 時刻。初回失敗はその YP の番組なし、再取得失敗は最終成功から 5 分未満の一覧だけを保持して `stale:true` を返し、以後は除外する。他の YP と自ノードは表示を継続する。YP 別の取得失敗・URL 未設定・古い一覧の診断情報は閲覧画面には表示しない（API の `sources` には保持する）。「一覧を更新」でも 60 秒以内はキャッシュを利用する。

トップ一覧の表示では中継を開始しない。チャンネル名から個別視聴ページを開くと自動的に `/site/stream/{id}` を要求する。そのとき、既存チャンネルがあればそれを使用する。なければ取得済みカタログに ID があることを確認し、記載された public IP リテラルの tracker へ `Manager.StartRelay` で接続する。カタログが 5 分以上古い場合は使用しない。hostname、loopback、private、link-local、非 unicast、IPv4 の 0/8・240/4・共有アドレス 100.64/10・ベンチマーク用 198.18/15、無効ポートは不可。空 tracker の push / GIV 接続は行わない。利用者指定の tracker や YP 探索へのフォールバックは提供しない。

中継開始前に認証・サイト視聴枠・中継数を検査し、既存のノード共通中継上限も適用する。既定のサイト中継上限は 8。これは番組一覧数の上限ではない。同じ ID の中継を視聴者ごとに増やさない。視聴を閉じても共有中継は即停止せず、既存の `channel_cleanup_minutes`（既定 20 分、0 は自動削除なし）で未使用中継を回収する。公開 PCP 中継の認証・接続先規則は変更しない。

配信設定は最大 8192 bytes の JSON。未定義フィールド・複数 JSON は拒否。名前は前後空白除去後に必須、最大 256 bytes、ジャンルは前後空白除去後、小文字 `yp` で始まらなければ `yp` を先頭に補い、補完後256 bytesまで（空欄は `yp`）。既存の `yp` 接頭辞・制御文字列は維持する。説明・コメント・URL は各 2048 bytes。`comment` と `contactUrl` は省略時空文字。URL は前後空白除去後に空またはユーザー情報を含まない HTTP(S) の絶対URL。`bitrate` は kbps の整数で、省略・0 は自動、指定時は1〜2147483647。FLV チャンネルとして登録し、RTMP メタデータで実際のメディア情報を更新する。

## 配信キーとライフサイクル

`stream_keys.json` のアカウント名 `site:x:<X ID>` に 暗号学的乱数で一様に選んだ小文字英字2文字＋数字4桁のキー（例: `ab1234`、数字の先頭0も保持）を対応付ける。新しいブラウザーセッションでも同じ X ID なら既存キーを利用する。ノード全体の発行キー数が 10000 以上ならサイトからの新規発行は 503（既存ユーザーの再発行は可）。

既存キーは自動変更せず、次回の発行・再発行から新形式を使う。再発行時は旧キーと使用中のキーを避け、衝突時は最大128回試す。生成・発行保存に失敗した場合は 500 を返し、以前のキーを維持する。保存と変更は直列化し、他アカウントと同一キーの共有は拒否する。従来同様、保存は生値・0600、管理 API からは管理者が一覧取得できる。

小文字英字2文字＋数字4桁の RTMP Publish は、成功・失敗を含め接続元IPごとに60秒で10回、ノード全体で120回まで。接続し直してもカウンターを共有し、超過時は Publish を拒否する。既存の送信中メディアと他形式のキーはこの制限の対象外。

配信枠作成後に OBS 等で配信を開始する。配信枠作成・停止時に YP があれば再通知する。Web ログアウトでは OBS 配信・キーは失効しない。サイトの停止操作はチャンネルを停止し、エンコーダーの TCP 接続自体は強制切断しない。キー再発行後、古いキーでは新規 Publish できず、新しいキーの配信枠にも対応しない。

## 視聴画面

### 掲示板コメント

チャンネルの連絡先 URL から、したらば（`jbbs.shitaraba.net`、旧 `jbbs.livedoor.jp` は同ホストへ正規化）と JPNKN（`bbs.jpnkn.com`）を判定する。したらばの板 URL / `bbs/read.cgi`、JPNKN の板 URL / `test/read.cgi` に対応する。それ以外の連絡先は `supported:false` と空配列を返し、元の連絡先リンクを表示する。チャンネルは自ノードまたは直近の YP カタログから検索する。

板の `subject.txt` とスレッドの rawmode / dat を HTTPS で取得する。UTF-8 と、したらば EUC-JP / JPNKN Shift_JIS の日本語を扱う。スレッド指定の連絡先なら初回からコメントを表示し、板 URL ならスレッド選択を表示する。最新 30 コメントを新しい順に並べ、スレッド・コメントを 10 秒ごとに更新する。投稿機能・掲示板画像・任意 HTML の埋め込みはない。HTML の br を改行に変換し、タグを除去してエンティティをデコードしたテキストを React がエスケープ表示する。

`threads` は `{id,title,comments}[]`（最大 200 スレッド、重複 ID 除外）、`comments` は `{no,name,date,body}[]`。`commentCount` は最新レス番号。未選択なら `threadId` は空文字。利用者指定の `thread` は板の subject に存在するもの、または配信者の連絡先にある元のスレッドのみ受理する。外部 URL・別の板・任意のクエリは指定できない。不正な入力 400、存在しないチャンネル / スレッド 404、取得・解析失敗 502、チャンネル情報の確認失敗 503。

取得はブラウザーの Cookie / Authorization を転送せず、任意の転送プロキシも利用しない。許可ホストのみを名前解決し、private 等の IP を拒否して検証した IP へ接続する。リダイレクトは追従しない。HTTP 応答は最大 2 MiB、行は 256 KiB 未満。ネットワーク取得は並行 4 件、各 5 秒、API 全体 11 秒。URL ごとに成功・失敗を 10 秒キャッシュし、同時要求を集約する。キャッシュは最大 64 件。既存の取得に対する呼出元キャンセルは共有取得を中断しない。

### ページとプレイヤー

表示用ジャンルでは、先頭の `yp` / `sp` / `tp` / `pp` に続く英数字ネームスペース＋`:`、`?`、`@` 列を YP4G 制御部分として除去する。制御記号がなくても接頭辞直後が末尾または英数字以外なら接頭辞を除去する。`sports` など英数字が直結し制御記号のない曖昧な文字列、未知の接頭辞、途中の制御記号は保持する。除去後に空になったジャンルの区切り ` - ` は表示しない。一覧・詳細・検索に共通で適用し、API・カタログ・PCP の元データや YP の人数・帯域制御は変更しない。

チャンネル一覧のカード全体は個別視聴ページへの通常のリンク。名前・アイコン・説明・余白のどこからでも移動でき、Tab / Enter、新しいタブで開く操作にも対応する。カード内にリンクを入れ子にしない。

視聴サイトは OS のダーク設定によらず白背景・濃いグレーの文字を使う。管理パネルの配色は変更しない。一覧と詳細は YP アイコン、チャンネル名、`ジャンル - 説明 配信者コメント`、既知の視聴人数・配信経過時間を表示する。詳細のチャンネル情報は動画の下に置く。ジャンル中の独立した `game`、説明の `<Open>` / `<Free>` / `<2M Over>` / `<Over>` は表示時だけ省略する。文字列は HTML として描画しない。検索対象は名前・表示説明（ジャンルとコメントを含む）。アイコンは同梱画像を使い、SP / TP / 0yp 以外には共通画像を表示する。0yp は同梱の favicon を使い、丸く切り取らず正方形の枠内に画像全体を表示する。個人アバター・外部画像 URL の取得は行わない。

ChannelView の任意フィールド `comment` は配信者コメント（掲示板コメントとは別）、`uptime` は秒単位の配信経過時間。不明な uptime は省略する。YP 由来は YP の値を使い、ローカル中継を開始しても中継経過時間に置き換えない。自ノードの配信は自ノードでの開始からの経過時間を使う。視聴人数が負なら表示しない。

ログイン後のナビゲーションは右上のメニューボタンに集約する。利用者名、チャンネル一覧、配信する、ログアウトを表示し、Escape・外側のクリック・フォーカスが外れたときに閉じる。フッターには小さな `/admin` の管理パネルリンクを置く（管理 API の認証条件は変更しない）。スマホではヘッダーと余白を縮め、視聴プレイヤーと掲示板を縦に並べる。メニューボタンとリンク、管理パネルリンクは高さ 44px 以上の操作領域を持つ。

ライブ動画には「ライブ配信」を表示する。Chromium / WebKit の標準動画コントロールではシークバー・経過時間・総時間を CSS で非表示にする。他の標準操作は維持する。これは表示の調整であり、バッファ内シーク自体を禁止するものではない。Firefox など、この非標準 CSS に対応しないブラウザーや OS 独自の全画面プレイヤーの時間軸表示はブラウザーに従う。

一覧は 10 秒ごと、セッションは 30 秒ごとに更新する。`/` で名前・ジャンルを検索し、チャンネル名リンクで `/channels/<id>` へ移動する。直接 URL を開いた場合も同じページを表示する。未ログインならログイン画面を表示し、認証後に元のサイト内ページへ戻る。`next` は `/`・`/broadcast`・有効 ID の `/channels/<id>`・`/admin`・`/admin/` を受理し、それ以外は `/` とする。

個別ページのマウントで FLV を mpegts.js の MediaSource に接続して音声付き自動再生を試みる。ブラウザーから `NotAllowedError` を受けたらミュートで再試行し、その状態を表示する。再試行も拒否された場合はプレイヤー内の再生操作を案内する。video の標準 controls に再生・一時停止・音量・ミュート・対応ブラウザーの全画面 / PiP を任せる。独自の「視聴する」「再生を開始」「全画面」「ミニプレイヤー」「視聴を閉じる」ボタンはない。メディアエラーは状態を表示し、ページ再読み込みを案内する。

`/broadcast` へは「配信する」リンクで移動する。本人の配信キー発行・配信枠作成はこのページで行う。配信名・ジャンル・説明・コメント・URLを入力できる。ビットレート入力欄はなく、作成時は `bitrate: 0` を送信する。RTMPメタデータの映像レート（maxBitrate優先、なければvideodatarate）とaudiodatarateから更新する。メタデータにレートがなければ0のまま。コメントとURLはチャンネル情報に反映され、一覧・視聴ページにも返す。フッターはアプリ名のみ。YP の取得設定に関する状態表示は閲覧画面に出さない。

Go のサイトサーバーは `/`・`/channels/<id>`・`/broadcast`・`/admin` にビルド済み UI を返し、従来の `/watch` は `/` へリダイレクトする。Vite 開発時も同じ UI パスを使える。管理画面の「配信を開始」とキー発行フォームはない。既存キーの確認・失効、チャンネルの停止・編集などは残す。管理 JSON-RPC のメソッドや認証方式は変更しない。`/admin` は UI の入口であって管理権限を与える仕組みではなく、許可リストに登録したX管理者だけがサイトの管理API入口を使える。直接の `/api/1` はサイトからプロキシしない。管理者専用の `/admin/api/1` を使う。

メディアの形式 / ブラウザーが非対応なら案内する。HLS 変換は行わない。HTTP 接続は pause 中もデータを受信する場合があるため、帯域利用を止めるときは一覧へ戻るかページを閉じる。ページ遷移・ログアウト時はプレイヤーを破棄する。

サイト全体と X ID ごとの視聴接続数をロック下で判定し、上限は 429 と `Retry-After: 5`。サイト経由でもノード側の `max_listeners` / `max_upstream_kbps` が適用される。別ログインセッションを作ってもユーザー枠は増えない。ログアウト・セッション期限切れはそのセッションのプロキシ接続をキャンセルする。

メディアの各書込には 15 秒の期限を設け、受信しないブラウザーが接続枠を保持し続けないようにする。切断までにこの書込待ちが残る場合がある。

クライアントの Cookie / Authorization / 転送ヘッダーをノードにそのまま渡さず、パスを組み直し内部 token だけを付ける。公開 PCP の接続数はサイトの接続枠に含めず、ノード側の制限で管理する。

実装の理由は [ADR 0019](../decisions/0019-authenticated-site.md)、検証範囲は [実装記録](../reviews/2026-09-14-site-implementation.md) を参照。

### 配信状態と停止

配信ページは本人の状態を5秒ごとに取得し、配信枠があれば「OBS接続待ち」または「配信中」、ジャンル・説明・コメント・コンタクトURL・ビットレートを表示する。視聴ページへのリンクを提供する。`ChannelView.bitrate` はkbps（0は不明・自動）。自ノードの情報から設定する。

停止はチャンネル名を示して確認し、失敗時は配信状態を維持してエラーを表示する。成功時は停止したチャンネル名を表示し、そのページ内の入力値に直前のチャンネル情報を戻す。同じ内容で再作成できるが、ページを閉じた後の履歴保存は行わない。停止後もキーは有効で、OBSの送信停止はOBS側で行う。

### 配信フォームの履歴と掲示板情報

`BroadcastHistoryEntry` は `{name,genre,description,comment,contactUrl,createdAt}`。作成成功時の入力5項目（name / genre / contactUrl は前後空白除去、genre は公開用 yp 補完前）と UTC の日時を新しい順で最大10件保存する。ビットレート・キーは含めない。アカウントはセッションで確定し、他人の指定は受け付けない。X アカウント単位で別端末・再ログイン・キー再発行・再起動後も利用できる。

フォーム初期値は直近履歴、履歴なしは空欄。「以前の設定を読み込む」で名前・日時・詳細（空ならコメント）を見て選択し、5項目を読み込む。未送信の編集は永続化しない。定期取得では編集中の値を上書きしない。配信停止後は直近履歴を再表示する。導入前の枠で履歴がなければ停止時のチャンネル情報を引き継ぐ。

保存は `broadcast_history_dir` 内のアカウント識別子 SHA-256 をファイル名とする JSON（`{version:1,entries:[...]}`）。新規ディレクトリ0700、ファイル0600。一時ファイルを同期して rename で置換する。読込時は512 KiBまで・バージョンと件数を確認し、不正ファイルは上書きしない。書込失敗時は作成枠を停止して500を返す。同じディレクトリを複数プロセスから使用しない。

コンタクトURLは入力停止400ms後に掲示板情報を取得する。対応先はしたらば（旧 livedoor ホストは正規化）と jpnkn。許可ホスト・既知パスだけを解析し、既存掲示板 reader の公開IP制限・リダイレクト拒否・応答サイズ制限・10秒キャッシュを適用する。未対応URLは `supported:false`。掲示板設定と subject.txt から掲示板名・現在のスレ名・レス数を表示し、一覧にない指定スレは本文取得で補完を試みる。本文取得不可は `threadError` に返し、新スレ候補は利用可能。

`thread` / `latestThread` は `{id,title,comments}` または null。`BBS_THREAD_STOP`（未設定・不正値は1000）よりレス数が少ないスレのうち数値ID最大を最新とする。「新スレに移動」は再取得した最新候補の正規スレッドURLで入力欄を差し替える。板トップからも使える。候補なしはボタン無効、取得失敗時はURLを維持する。スレッド作成や配信中の情報変更は行わない。URL変更後に古い応答で入力を上書きしない。
