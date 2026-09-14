# peercast-mi

Go 製の PeerCast ノード。ブロードキャストノード (RTMP → PCP 配信) と、上流 PCP ノードから受信して中継するリレーノードに対応する。

Web UI (`ui/` は別管理) と firewalled ノード向けの push (GIV) 接続は対象外。

## 主な構成

- `internal/channel` — チャンネル、コンテンツバッファ、下流ノード情報、ストリームキー、リレーの起動と自動削除
- `internal/rtmp` — RTMP push の受信と FLV 変換
- `internal/relay` — 上流 PCP ノードへの接続・接続先選択・再接続
- `internal/servent` — ポート 7144 の受け付け (PCP リレー送信、HTTP 視聴、/pls/、JSON-RPC の転送)
- `internal/yp` — YP への COUT 接続とチャンネル情報の送信
- `internal/jsonrpc` — JSON-RPC API
- `internal/pcputil` — PCP Host アトム構築の共通処理
- `internal/config`, `internal/id`, `internal/version` — 設定、ID 生成、バージョン定数

詳細は以下を参照する (索引は `docs/README.md`)。

- `docs/spec/overview.md` — 全体構成、ライフサイクル、並行処理、定数
- `docs/spec/components.md` — 各コンポーネントの仕様
- `docs/spec/api/jsonrpc.md` — JSON-RPC API 仕様
- `docs/decisions/` — 設計判断の記録 (ADR)。PeerCastStation との挙動合わせは `peercaststation-compat.md`
- `docs/reference/protocol/` — PCP など外部プロトコルの参照資料
- `docs/tasks.md` — 未着手・進行中・保留の作業

## 実行

```sh
go run . [-config config.toml] [-yp <YP名>]
```

チャンネルの作成と設定は JSON-RPC API (`POST /api/1`) で行う。

既定のポート (config.toml で変更可):

- `1935` — RTMP push 受信
- `7144` — PCP、HTTP 視聴、JSON-RPC API

## 依存ライブラリ

- `github.com/titagaki/peercast-pcp` — PCP プロトコル層
- `github.com/yutopp/go-rtmp` — RTMP サーバー

`peercast-pcp` の API は推測せず、次のコマンドまたは依存元のコードで確認する。

```sh
go doc github.com/titagaki/peercast-pcp/pcp
```

## 作業ルール

### 互換性

- 互換性に関わる作業では、関連する ADR、`docs/decisions/peercaststation-compat.md`、`docs/reviews/` の比較調査と `docs/reference/protocol/` を確認する。過去の調査やコードコメントの「互換」という記述だけで一致と判断せず、必要な範囲で参照実装を確認する
- 比較調査には参照リポジトリのコミット、作業ツリーの変更有無、対象範囲、ファイル・関数の根拠を残す。「確認できた差異」「意図的な差異」「未確認」を区別し、意図や理由を推測で補わない
- 調査・レビューの依頼では、結果と修正候補を記録する。実装変更も依頼されている場合はその範囲で進める
- 互換性を意図的に崩す変更で、既存の依頼・合意・ADRに判断根拠がない場合は、影響と選択肢を示して実装前にユーザーに確認する。既に合意済みの範囲では再確認しない
- JSON-RPC API は PeerCastStation 互換より peercast-mi 独自の使いやすさを優先する (`docs/decisions/0009`)。例外的に互換性を持たせる場合は ADR に記録する

### ドキュメント

- 実装の動作を変更した場合は、対応する `docs/spec/` の記述を同じ変更に含める。仕様書には「現在どう動くか」だけを書く
- JSON-RPC のメソッド名、引数、戻り値、エラー形式を変更した場合は、実装・テスト・`docs/spec/api/jsonrpc.md` を一緒に更新する
- 公開 API、互換性、責務分担、永続化形式など、将来の実装を拘束する重要な判断は `docs/decisions/NNNN-slug.md` に記録し、`docs/decisions/README.md` の一覧に行を足す。書式は同 README を参照。小さな実装上の判断はコードコメントで足りる
- ADR には判断理由と却下した案を書き、仕様書には書かない
- 仕様書と実装が食い違っていたら、実装側に判断の記録があるかを確認したうえで仕様書を実装に合わせる (`docs/decisions/0013`)
- 比較・検証の結果は `docs/reviews/` に記録し、`docs/README.md` からリンクする。過去の記録と矛盾した場合は、過去の経緯を残しつつ再検証結果への参照を加える
- 未着手・保留の作業は `docs/tasks.md` に記録し、AGENTS.md・CLAUDE.md や仕様書に TODO を追加しない

### 検証

- 変更した Go ファイルは `gofmt -w` で整形する
- Go の実装・依存関係を変更した場合は、影響に応じたテストを追加・更新し、完了前に以下を実行する。文書だけの変更では差分・リンク・記述根拠を確認し、Go の検証は必要な場合に実行する

```sh
go vet ./...
go test ./...
```

- 調査で使う再現チェックは、既存テストの成功と区別して結果を記録する。単体テストだけで他実装との相互接続確認済みとはしない
- 実行した検証と結果、実行できなかった検証と理由を報告する
- テストでループバックの「閉じたポート」が必要なときは `127.0.0.1:1` のような固定アドレスを使わず、`net.Listen("tcp", "127.0.0.1:0")` で取得して閉じたポートを使う (環境によっては SYN が捨てられ、接続拒否ではなくタイムアウト待ちになる)
