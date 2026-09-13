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

- PeerCastStation 互換の処理を変更する前に、`docs/decisions/peercaststation-compat.md` と関連する `docs/reference/protocol/` を確認する
- 互換性を意図的に崩す変更は、実装前にユーザーに確認する
- JSON-RPC API は PeerCastStation 互換より peercast-mi 独自の使いやすさを優先する (`docs/decisions/0009`)。例外的に互換性を持たせる場合は ADR に記録する

### ドキュメント

- 実装の動作を変更した場合は、対応する `docs/spec/` の記述を同じ変更に含める。仕様書には「現在どう動くか」だけを書く
- JSON-RPC のメソッド名、引数、戻り値、エラー形式を変更した場合は、実装・テスト・`docs/spec/api/jsonrpc.md` を一緒に更新する
- 公開 API、互換性、責務分担、永続化形式など、将来の実装を拘束する重要な判断は `docs/decisions/NNNN-slug.md` に記録し、`docs/decisions/README.md` の一覧に行を足す。書式は同 README を参照。小さな実装上の判断はコードコメントで足りる
- ADR には判断理由と却下した案を書き、仕様書には書かない
- 仕様書と実装が食い違っていたら、実装側に判断の記録があるかを確認したうえで仕様書を実装に合わせる (`docs/decisions/0013`)
- 未着手・保留の作業は `docs/tasks.md` に記録し、CLAUDE.md や仕様書に TODO を追加しない

### 検証

- 変更した Go ファイルは `gofmt -w` で整形する
- 完了前に以下を実行し、結果を報告する

```sh
go vet ./...
go test ./...
```

- テストでループバックの「閉じたポート」が必要なときは `127.0.0.1:1` のような固定アドレスを使わず、`net.Listen("tcp", "127.0.0.1:0")` で取得して閉じたポートを使う (環境によっては SYN が捨てられ、接続拒否ではなくタイムアウト待ちになる)
