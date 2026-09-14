# peca-live の視聴機能・認証調査と peercast-mi 拡張案

追記: 掲示板閲覧は [ページ分離・コメント対応](2026-09-14-viewing-pages-comments.md) で追加した。

追記: 外部 YP 一覧・`updateYPChannels`・カタログからの中継開始は [後続実装](2026-09-14-yp-directory.md) で追加した。以下の現状・未実装の記述は調査時点のもの。

調査日: 2026-09-14。静的コード調査。以下の設計は**提案・未採用・未実装**であり、現在の仕様や採用済み ADR ではない。

追記: 同日の追加依頼で初期実装を行った。この文書は調査時点の記録として残す。採用した範囲と判断は [ADR 0019](../decisions/0019-authenticated-site.md)、現在の動作は [site.md](../spec/site.md)、検証と未実装部分は [実装記録](2026-09-14-site-implementation.md) を参照。

## 対象と結論

ユーザーの希望は「peca-live を参考に視聴機能を充実させ、X ログインで認証済みの人が視聴・配信できること」。ここでの認証済みはログイン成功を仮定し、X の認証バッジ保有や運営による承認とは区別する。

同日のユーザー確認: PeerCast の公開・中継を制限する要求ではない。目的はサイトの帯域等を無条件で利用させないことであり、X ログインを求める対象は自サイト経由の視聴・配信利用。公開 PCP 中継と他ノード経由の視聴は維持する。この要件は確認済みで、具体的な認証実装・制限値は未決定。

| 対象 | HEAD | 調査開始時の作業ツリー |
|:--|:--|:--|
| `/home/megan/src/peca-live` | `e21a01640fcc691a70818255b7ba2ece37d999f6` | `Gemfile`、`Gemfile.lock` に変更あり。変更せず現状を参照 |
| `/home/megan/src/go/peercast-mi` | `b89ff9dadeaaa61abeb13579b6831b634470865e` | 先行の管理 UI 改善と関連文書が未コミット。Go 実装は HEAD、UI は作業ツリーを参照 |

peca-live の一覧・プレイヤー・掲示板閲覧は参考になる。ただし、Twitter ログインはお気に入り等の個人機能に使われ、視聴の必須条件ではない。アカウント別の配信権限管理も今回読んだ実装にはない。UI の移植だけでは希望する認証境界はできない。

参照範囲: peca-live の `app/javascript/packs/`、認証・チャンネル・お気に入りの controller/helper/model、`lib/json_rpc.rb`、`lib/bbs.rb`、ルーティング。mi の管理 UI、JSON-RPC 認証、HTTP/PCP 入口、RTMP 認証、キー保存。デプロイ先・外部サービス・全コードのセキュリティ監査は対象外。

## 機能比較

peca-live のパスは参照リポジトリ内。実装が存在することと現在稼働することは区別する。

| 機能 | peca-live で確認できた動作と根拠 | peercast-mi の現状 / 拡張案 |
|:--|:--|:--|
| 配信一覧 | `ChannelsController#index` → `JsonRpc#update_yp_channels` で `updateYPChannels` を呼ぶ。`channelsModule.updateChannels` がお気に入り優先・開始の新しい順に整列 | 管理 UI は自ノードのチャンネル一覧。YP 全体のカタログ取得 API はない。自ノードのみか外部 YP も含むか選ぶ必要あり |
| 一覧の更新 | `app.tsx` は実コードで 10 秒ごと、controller の取得キャッシュは 1 分。コメントの「1分」と区別 | 視聴用一覧を追加し、更新中も再生を継続させる |
| ブラウザー内再生 | `components/Video.tsx` が flv.js で HTTP FLV を再生。`types/Channel.ts#flvStreamUrl` でノードの `/stream/<id>.flv?tip=...` を直接指定 | HTTP 出力とオンデマンドリレーはある。管理 UI に内蔵プレイヤーはない。認証済みの同一オリジン配信口を追加する案 |
| 再生操作 | `Video.tsx` に再生、音量・ミュート、全画面、対応時の PiP、前後のチャンネル切替 | 視聴画面へ追加。端末の機能検出、再生拒否・配信終了・再試行の表示も必要 |
| 再接続 | `ChannelPlayer.tsx` は GET の bump API を呼びページ再読込 | mi の `bumpChannel` は共有リレーに影響する。一般視聴者の再試行はまず自分のプレイヤーのみ再接続し、ノード bump は管理者等に限定する案 |
| スマホ・HLS | `app.tsx` は iOS を HLS 分岐に送るが `Video.tsx` の HLS 処理は TODO / return。iOS や非 FLV は VLC 向け URL へ誘導 | HLS 変換・配信はない。ブラウザー内のスマホ視聴を必須にするなら、変換処理と認証付きマニフェスト・セグメント配信を追加し端末で検証する |
| 外部プレイヤー | `Channel#vlcStreamUrl` は FLV に RTMP、WMV に MMS URL を作る | mi の RTMP は push 受信であり、この視聴 URL をそのまま再利用できない。外部プレイヤー対応は認証方法も別途必要 |
| 掲示板 | `Comments.tsx` にスレッド選択・10 秒更新。`lib/bbs.rb` はしたらば系 / jpnkn 形式を解析し最新 30 コメントを返す。投稿 UI は確認できない | 閲覧 API と画面を追加。掲示板投稿は「同等機能」に含めず別要件。外部 URL 取得には接続先制限・タイムアウト・サイズ上限が必要 |
| お気に入り | `FavoritesController` と `User#favorite!` がユーザー別にチャンネル名を保存。匿名では空一覧、書込を行わない | ユーザー・お気に入り保存を追加。名称変更や同名衝突を考慮して識別子を設計する |
| 配信開始通知 | `ChannelsController#notification_broadcasting` にお気に入りユーザーの端末向け通知処理あり | 通知は後続候補。外部通知サービスとジョブが現在稼働するか未確認 |
| 非表示設定 | `Channels::PrivateController#show` は配信履歴と IP の一致で切替。`visible_channel?` で一覧から除外 | 映像のアクセス制御ではない。今回の「認証済み限定」と同一視しない |
| X ログイン | `SignInScreen.tsx` の Firebase Twitter provider → `app.tsx` が ID token を Rails へ POST → `AccountsController#create` / `SessionsHelper` が session を作る | X / Firebase のユーザー認証はない。一般利用者用の認証層が必要 |
| 認証した人の配信 | 今回の routes/controller/model に X ユーザーと配信キーの所有権を対応させる実装は見当たらない | `StreamKeyStore` にアカウント名→キーの保存と RTMP `OnPublish` の発行済みチェックはある。ログインユーザーへの紐付け・所有者限定操作を追加する |

## そのまま取り込まない点と理由

以下は今回提案する差異。peca-live 作者の意図は、コードコメントで確認できるもの以外は推定しない。

| 項目 | 確認できた現状 / 既存の理由 | 拡張時の案と理由 |
|:--|:--|:--|
| HTTP へのリダイレクト | `ApplicationController#to_valid_url` のコメントは PeerCast と HTTP 通信するためと説明 | サイト・メディアとも HTTPS。Cookie と配信データを平文 HTTP に降格させない |
| Firebase トークン検証 | `FirebaseHelper::Auth.verify_id_token` は署名検証を行うが、独自 `validate` の呼出しがコメントアウト。そこに audience / issuer / subject の確認がある。無効化理由は未確認 | この検証コードは移植しない。Firebase 採用時は公式 Admin SDK で対象プロジェクトの ID token を検証する |
| アカウント更新 | `AccountsController#create` は UID・表示名・画像すべてで `find_or_create_by!`。UID に一意制約あり | 安定した provider ID で検索し、表示名・画像は更新する。表示名変更で別アカウントを作らない |
| 再接続の権限 | peca-live の bump controller にログイン必須チェックはなく GET で操作。理由は未確認 | 一般視聴者に管理用 RPC をそのまま公開しない。共有状態を変える操作は権限・CSRF・頻度制限を設ける |
| シェル経由の外部通信 | `JsonRpc#command`、通知処理は文字列を組み立てシェルから curl を実行。理由は未確認 | HTTP クライアントで実装し、入力をシェルへ渡さない |
| 秘密情報 | peca-live の通知処理に資格情報らしき定数を確認。有効性は未確認で値は記載しない。mi の RTMP `OnPublish` はキーをログ・エラー文に含める | 定数は移植しない。有効なら所有者による失効・再発行が必要。mi は一般公開前にログのキーを除去する |
| 公開 HTTP 視聴 | mi の [ADR 0016](../decisions/0016-relay-request-source-policy.md) は既存チャンネルの公開視聴を維持し、新規リレー要求だけ送信元制限 | 自サイトの HTTP 視聴口を認証必須にする要件を確認済み。公開 PCP は維持。具体的な入口制御と従来の公開ノード利用との両立方法を ADR にする |

## 構成案

管理 UI と利用者向け画面は役割を分ける。以下は責務の分離であり、Go に同居させるか別サービスにするかは未決定。

```text
ブラウザー ─ HTTPS ─ 利用者向け Web / API
                     ├─ X ログイン / セッション / ユーザー権限
                     ├─ 一覧 / お気に入り / 自分の配信設定
                     ├─ 認証付きメディア出力 ─ peercast-mi HTTP 出力
                     └─ 許可した管理操作のみ ─ peercast-mi 管理 API
配信ソフト ─ 保護された ingest ─ ユーザー別キーの確認 ─ RTMP 受信
```

### ログインと配信の所有権

- X 直接連携なら OAuth 2.0 Authorization Code + PKCE を候補とし、サーバーでコードを交換して `/2/users/me` でユーザーを識別する。`state` の一回限り検証、PKCE S256、固定 callback、期限、失敗時の拒否を実装する。X の方式とユーザー取得は [OAuth 公式資料](https://docs.x.com/fundamentals/authentication/oauth-2-0/authorization-code)、[Authenticated User Quickstart](https://docs.x.com/x-api/users/lookup/quickstart/authenticated-lookup) を確認した。
- peca-live と同様に Firebase を使う選択肢もある。[Twitter 認証の公式手順](https://firebase.google.com/docs/auth/web/twitter-login) と [ID token 検証](https://firebase.google.com/docs/auth/admin/verify-id-tokens) に従う。Firebase の UID と X の ID は区別する。どちらの方式もアプリ登録・callback 設定等が必要。利用可能な契約・料金・実アカウントでのログインは未確認で、無料利用を前提にしない。
- X のユーザー名ではなく provider の安定 ID をローカルユーザーに結び付ける。メール・投稿権限等は要求せず、利用する本人情報 API に必要な scope を実装時に確定する。X の配信操作をするわけではないので、X の `broadcast.write` 等を peercast-mi 配信権限と混同しない。
- セッションはサーバー側で管理し、Cookie に Secure / HttpOnly / SameSite を設定。ログイン時に更新、ログアウト・停止時に失効。Cookie 認証の状態変更 API は CSRF を防ぐ。OAuth token や管理用 Basic パスワードを UI に配らない。
- 視聴者 / 配信者 / 管理者の権限を内部で分ける。ログイン成功者へ両権限を自動付与するか、承認を必要にするかは未決定。
- 本人専用 API でキー発行・再発行・配信設定・停止を提供する。クライアント指定のアカウント名を信じずセッションから所有者を確定し、他人のキー・チャンネルを操作できなくする。
- OBS 等はブラウザーの X セッションを送らないため、発行済みキーで認証する構成になる。キー保有者が配信可能であり、接続者本人が毎回 X ログインしている保証とは異なる。失効・期限・同時配信数を定義し、公開ネットワークでは RTMPS 終端等も検討する。現在の RTMP に TLS があるとは扱わない。
- 現キーは生値を保存・一覧取得する設計。安全な乱数生成、漏えい対策、保存方式、既存キー移行を決める。キー失効は現行では新規 Publish の拒否であり、アカウント停止時は既存配信接続の終了も必要。

### 視聴制限の境界（ユーザー確認済み）

| 選択肢 | 保証できること | 制約 |
|:--|:--|:--|
| 今回の要件: サイト利用はログイン必須、公開 PCP は維持 | このサイトの HTTP 視聴・配信操作をログインユーザーに制限 | 他ノード経由の視聴は意図どおり許可する。映像自体の会員限定は求めない |
| 対象外: 公開 PCP を制限して映像自体を会員限定にする | 今回は実装しない | 当初の調査で選択肢に挙げたが、ユーザーが不要と明示した |

サイトの帯域を守るため、ブラウザーの表示制御だけでなく、自サイトの `/stream/`、`/pls/`、導入する場合の HLS マニフェスト・全セグメントを保護し、バックエンド HTTP 直アクセスやキャッシュによる迂回を防ぐ。公開 PCP の `/channel/` 経由の中継は許可するもので、認証の迂回として扱わない。PCP と HTTP が同ポートのため、PCP を公開しながら HTTP だけ制限するにはプロトコルを理解する入口制御かバックエンド変更が必要で、ポート単位の firewall だけでは分けられない。

ログインだけでは帯域の総量を制御できない。サイト視聴のユーザー別同時接続数・全体上限・再接続頻度制限を追加候補とし、値は運用条件に合わせて決める。公開 PCP 中継も帯域を使うが、その通常の容量管理とサイト利用者の認証は分ける。

mi の管理 API は実 TCP 接続元が loopback なら Basic を省略する (`internal/jsonrpc/server.go#Handler`、`internal/servent/listener.go#handleAPIRequest`)。したがって localhost 宛ての無制限リバースプロキシは作らない。利用者向け API が認証・所有権・許可する操作を検査し、バックエンド管理 API を直接外部へ公開しない。

長時間の FLV 接続は入口の一回認証だけでは後の失効を反映しない。セッション失効時の切断、最大接続時間等を設計する。HLS はセグメント要求でも権限を確認する。新規リレーの `tip` を一般ユーザーからそのまま受け取るとゲートウェイ権限で任意接続できるため、承認したカタログ等から解決し、接続先・帯域・同時数を制限する。

## 進め方と検証条件

1. 公開 PCP 維持を前提に、ログインのみか承認制か、視聴対象が自ノードか外部 YP も含むかを確定する。採用する認証方式・責務分担を ADR にする。
2. ユーザー・セッション・所有権と認証付きメディア入口を作る。他人の配信停止、キー取得、直アクセスの迂回ができないことを先にテストする。
3. 自ノードの視聴一覧、プレイヤー、再試行、全画面、音量、本人の配信設定を追加する。未ログイン・期限切れ・配信終了・非対応形式を UI に明示する。
4. お気に入り、掲示板閲覧、必要なら外部 YP 一覧と HLS を追加する。通知や掲示板投稿は別途優先度を決める。

受入テストには OAuth callback の改ざん・再送・拒否、ログアウト後のサイト映像接続、他人の ID 指定、失効キーの新規/既存配信、未認証の直 `/stream/` 拒否、公開 `/channel/` 中継の維持、サイト視聴の接続上限、複数視聴者中の再試行、スマホ実機、実際のエンコーダーと相互接続を含める。

今回実施: ファイル・関数の静的確認、HEAD / 作業ツリー確認、関連 ADR 0009・0014・0016 と視聴プロトコル資料の照合、公式認証資料の確認、文書差分確認。Go / UI 実装は変更せず、テストは実行していない。peca-live の起動、外部 YP / 掲示板 / 通知 / X 認証の疎通、実動画の再生は未検証。参照リポジトリで LICENSE / COPYING 名のファイルは見つからず、コードを直接転載する場合は別途利用条件を確認する。

既存の [ADR 0010](../decisions/0010-bump-channel-named-params.md)・[0014](../decisions/0014-stream-on-demand-relay.md) は peca-live 呼出しへの部分的な対応記録であり、視聴サービス全体やユーザー認証の実装済みを意味しない。
