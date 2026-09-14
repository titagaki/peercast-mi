# docs/

次セッションの再開時は [2026-09-14 引き継ぎ](handoffs/2026-09-14.md) を参照。未コミットの変更範囲、開発起動方法、検証結果をまとめている。

| ディレクトリ / ファイル | 内容 | 書くとき |
|:--|:--|:--|
| [spec/](spec/) | **仕様**: 現在の実装がどう動くか | 動作を変えるコミットで同時に更新する |
| [decisions/](decisions/README.md) | **設計判断の記録**: なぜそうしたか、却下した案、トレードオフ | 設計上の判断をしたとき。仕様書には書かない |
| [reference/](reference/) | **参照資料**: PeerCast / PCP プロトコルなど外部仕様のまとめ | peercast-mi の動作ではなく、前提となる外部知識を書く |
| [reviews/](reviews/) | **調査結果**: 実装比較、検証結果、未確認事項 | 比較元の版とコード根拠を残す。設計判断の採否は ADR に分ける |
| [tasks.md](tasks.md) | **タスク**: 未着手・進行中・保留の作業 | 作業を思いついたら書き、終わったら完了に移す |

## spec/

- [overview.md](spec/overview.md) — スコープ、システム構成、識別子、ライフサイクル、並行処理、定数
- [components.md](spec/components.md) — コンポーネント別の詳細 (RTMPServer / Channel / RelayClient / YPClient / Listener / 出力ストリーム / JSON-RPC)
- [api/jsonrpc.md](spec/api/jsonrpc.md) — JSON-RPC API のメソッド・パラメータ・返却値
- [ui.md](spec/ui.md) — 管理 UI の画面、通信・更新操作、アクセシビリティ
- [site.md](spec/site.md) — X 認証付き視聴・配信サイトの設定、API、帯域・所有権制御

## decisions/

番号付きの ADR (`NNNN-slug.md`) と、PeerCastStation との挙動合わせをまとめた [peercaststation-compat.md](decisions/peercaststation-compat.md)。一覧と書き方は [decisions/README.md](decisions/README.md)。

## reference/

- [protocol/PCP_SPEC.md](reference/protocol/PCP_SPEC.md) — PCP プロトコル仕様 (アトム形式・パケット種別)
- [protocol/broadcasting.md](reference/protocol/broadcasting.md) — PeerCast の配信 (ソース接続) プロトコル
- [protocol/viewing.md](reference/protocol/viewing.md) — PeerCast の視聴・リレー接続プロトコル
- [protocol/yp_channel_registration.md](reference/protocol/yp_channel_registration.md) — YP へのチャンネル掲載プロトコル

peercast-pcp ライブラリの API は `go doc github.com/titagaki/peercast-pcp/pcp` で確認する。

## reviews/

- [2026-09-14-broadcast-fields.md](reviews/2026-09-14-broadcast-fields.md) — 配信フォームのコメント・URL・ビットレート復元

- [2026-09-14-site-admin-connection.md](reviews/2026-09-14-site-admin-connection.md) — 本番管理パネルのAPI接続不具合と修正候補

- [2026-09-14-site-base-path.md](reviews/2026-09-14-site-base-path.md) — `/mi/` 配置、OAuth・UI・Dockerとインフラ設定のローカル検証

- [2026-09-14-site-genre-controls.md](reviews/2026-09-14-site-genre-controls.md) — YP4G ジャンル資料とサイト表示の制御部分除去

- [2026-09-14-site-white-channel-info.md](reviews/2026-09-14-site-white-channel-info.md) — 白基調、ぺからいぶを参考にした文字情報・YP アイコンと検証

- [2026-09-14-site-mobile-navigation.md](reviews/2026-09-14-site-mobile-navigation.md) — スマホ向けメニュー、管理リンク、ライブの時間軸表示と検証

- [2026-09-14-viewing-pages-comments.md](reviews/2026-09-14-viewing-pages-comments.md) — 一覧・個別視聴・管理パスの分離、自動再生、掲示板コメントと検証

- [2026-09-14-yp-directory.md](reviews/2026-09-14-yp-directory.md) — YP 一覧取得・updateYPChannels・サイトからの中継開始、参照資料と検証

- [2026-09-14-development-login.md](reviews/2026-09-14-development-login.md) — X 登録前のローカル開発ログイン、制限と検証
- [2026-09-14-site-dev-proxy.md](reviews/2026-09-14-site-dev-proxy.md) — localhost:5173/watch の API 転送・エラー案内の修正と検証
- [2026-09-14-site-implementation.md](reviews/2026-09-14-site-implementation.md) — 認証付きサイトの実装範囲、検証と導入時の確認事項
- [2026-09-14-peca-live-viewing-auth-design.md](reviews/2026-09-14-peca-live-viewing-auth-design.md) — peca-live の視聴機能・認証調査、X ログインと視聴・配信制限の拡張案（未採用）
- [2026-09-14-ui-improvements.md](reviews/2026-09-14-ui-improvements.md) — 管理 UI の操作不具合・表示改善、ブラウザー回帰テストと未確認範囲

- [2026-09-14-implementation-comparison.md](reviews/2026-09-14-implementation-comparison.md) — PeerCastStation・peercast-yt・peercast-mi の実装比較。意図的な差異と理由、残る差異、過去の互換性ノートの再評価、再現チェック
- [2026-09-14-comparison-corrections.md](reviews/2026-09-14-comparison-corrections.md) — 比較後の修正状況、維持する差異の理由、回帰テスト、運用上の注意
