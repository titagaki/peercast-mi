# 認証付き視聴・配信サイト

`site.enabled = true` で Go プロセス内の `internal/site` を起動する。無効が既定。管理コンソールとは別の HTTP 入口を使い、利用者向け画面は `/watch`。管理 JSON-RPC は公開しない。

## 有効化

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
| `ui_dir` | Vite build 出力。省略時 `ui/dist`。相対パスはプロセスの作業ディレクトリ基準 |
| `rtmp_url` | 利用者へ表示する RTMP(S) アプリケーション URL。必須。Go の待受先を変更する設定ではない |
| `max_viewers` | サイト全体の同時メディア接続数。省略 / 0 は 20。負数は起動エラー |
| `max_viewers_per_user` | X ID 単位の同時メディア接続数。省略 / 0 は 2。負数は起動エラー |

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

`rtmps://` を表示する場合は実際に RTMPS 終端を用意し、Go の `rtmp_port` へ転送する。Go 本体は RTMP 平文受信。公開ネットワークへ配信キーを平文で送らない構成にする。公開 PCP ポートはそのまま利用できる。

ブラウザーで `https://live.example.com/watch` を開く。開発時も `npm run dev` だけでは認証 API は起動しない。ビルド済み UI を Go サイトで配信する方法なら同一オリジンになる。開発用 HTTP origin は X 側の callback 登録可否も確認する。

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
| `GET /auth/x/start` | なし | X 認可画面へ 302 |
| `GET /auth/x/callback` | X の code / state | セッション作成、`/watch` へ 303。拒否・期限切れ・state 不一致 400、外部 API 失敗 502 |
| `GET /site/api/me` | なし | 未認証 `{user:null}`、認証済み `{user:{id,name},csrf}` |
| `POST /site/api/logout` | CSRF | `{ok:true}`。このセッションとその視聴接続を終了 |
| `GET /site/api/channels` | なし | `ChannelView[]`。自ノードに登録済みのチャンネルのみ |
| `GET /site/api/broadcast` | なし | `{streamKey,rtmpUrl,channel:ChannelView|null}`。本人のキーのみ |
| `POST /site/api/key` | CSRF | `{streamKey}`。生成 / 再発行。本人の配信枠がある間は 409 |
| `POST /site/api/broadcast` | CSRF、JSON `{name,genre,description}` | `ChannelView`。1 ユーザー 1 枠。未発行キー / 既存枠は 409 |
| `DELETE /site/api/broadcast` | CSRF | `{ok:true}`。本人の枠だけ停止。枠なしでも成功 |
| `GET /site/stream/{id}` | 32 hex のチャンネル ID | 認証付き HTTP ストリーム。任意クエリは 400、未登録 404、接続上限 429 |

`ChannelView` は `{id,name,genre,description,contactUrl,contentType,receiving,listeners}`。配信キー・ソース URL・接続先 IP を含めない。リレー・配信待機中のチャンネルも一覧対象。チャンネルの任意 `tip` 指定やリレー生成 API はない。

配信設定は最大 8192 bytes の JSON。未定義フィールド・複数 JSON は拒否。名前は前後空白除去後に必須、最大 256 bytes、ジャンル 256 bytes、説明 2048 bytes。FLV チャンネルとして登録し、RTMP メタデータで実際のメディア情報を更新する。

## 配信キーとライフサイクル

`stream_keys.json` のアカウント名 `site:x:<X ID>` に 32 bytes の乱数を base64url（padding なし）にしたキーを対応付ける。新しいブラウザーセッションでも同じ X ID なら既存キーを利用する。ノード全体の発行キー数が 10000 以上ならサイトからの新規発行は 503（既存ユーザーの再発行は可）。

発行保存に失敗した場合は 500 を返し、以前のキーを維持する。保存と変更は直列化し、他アカウントと同一キーの共有は拒否する。従来同様、保存は生値・0600、管理 API からは管理者が一覧取得できる。

配信枠作成後に OBS 等で配信を開始する。配信枠作成・停止時に YP があれば再通知する。Web ログアウトでは OBS 配信・キーは失効しない。サイトの停止操作はチャンネルを停止し、エンコーダーの TCP 接続自体は強制切断しない。キー再発行後、古いキーでは新規 Publish できず、新しいキーの配信枠にも対応しない。

## 視聴画面

一覧は 10 秒ごと、セッションは 30 秒ごとに更新する。名前・ジャンルを検索し、チャンネルを選ぶとプレイヤーを表示する。再生開始ボタンを押すまでは映像を要求しない。FLV は mpegts.js で MediaSource に変換して再生する。ネイティブの音量・ミュート・再生操作、全画面、対応時の PiP、映像のみの再接続、視聴を閉じる操作がある。再接続で共有ノードの bump は呼ばない。

メディアの形式 / ブラウザーが非対応なら案内する。HLS 変換は行わない。HTTP 接続は pause 中もデータを受信する場合があるため、帯域利用を止めるときは「視聴を閉じる」か画面を切り替える。

サイト全体と X ID ごとの視聴接続数をロック下で判定し、上限は 429 と `Retry-After: 5`。サイト経由でもノード側の `max_listeners` / `max_upstream_kbps` が適用される。別ログインセッションを作ってもユーザー枠は増えない。ログアウト・セッション期限切れはそのセッションのプロキシ接続をキャンセルする。

メディアの各書込には 15 秒の期限を設け、受信しないブラウザーが接続枠を保持し続けないようにする。切断までにこの書込待ちが残る場合がある。

クライアントの Cookie / Authorization / 転送ヘッダーをノードにそのまま渡さず、パスを組み直し内部 token だけを付ける。公開 PCP の接続数はサイトの接続枠に含めず、ノード側の制限で管理する。

実装の理由は [ADR 0019](../decisions/0019-authenticated-site.md)、検証範囲は [実装記録](../reviews/2026-09-14-site-implementation.md) を参照。
