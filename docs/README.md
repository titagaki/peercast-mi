# docs/

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

## decisions/

番号付きの ADR (`NNNN-slug.md`) と、PeerCastStation との挙動合わせをまとめた [peercaststation-compat.md](decisions/peercaststation-compat.md)。一覧と書き方は [decisions/README.md](decisions/README.md)。

## reference/

- [protocol/PCP_SPEC.md](reference/protocol/PCP_SPEC.md) — PCP プロトコル仕様 (アトム形式・パケット種別)
- [protocol/broadcasting.md](reference/protocol/broadcasting.md) — PeerCast の配信 (ソース接続) プロトコル
- [protocol/viewing.md](reference/protocol/viewing.md) — PeerCast の視聴・リレー接続プロトコル
- [protocol/yp_channel_registration.md](reference/protocol/yp_channel_registration.md) — YP へのチャンネル掲載プロトコル

peercast-pcp ライブラリの API は `go doc github.com/titagaki/peercast-pcp/pcp` で確認する。

## reviews/

- [2026-09-14-implementation-comparison.md](reviews/2026-09-14-implementation-comparison.md) — PeerCastStation・peercast-yt・peercast-mi の実装比較。意図的な差異と理由、残る差異、過去の互換性ノートの再評価、再現チェック
- [2026-09-14-comparison-corrections.md](reviews/2026-09-14-comparison-corrections.md) — 比較後の修正状況、維持する差異の理由、回帰テスト、運用上の注意
