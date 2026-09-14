# 設計判断の記録 (Decision Records)

「なぜそうしたか」を残す場所。仕様書 ([../spec/](../spec/)) は「現在どう動くか」だけを書き、判断の背景・却下した案・トレードオフはここに書く。

## 書き方

- ファイル名: `NNNN-短い-スラッグ.md` (連番 4 桁)。PeerCastStation との細かな挙動合わせは [peercaststation-compat.md](peercaststation-compat.md) に追記する
- 見出し構成: **状態** / **日付** / **背景** / **決定** / **結果・影響** / **参照**
- 状態は `採用` `却下` `置換 (→ NNNN)` `遡及記録` のいずれか。`遡及記録` は後から書き起こしたもので、日付は決定当時ではなく記録日
- 決定を覆すときは古い記録を書き換えず、新しい番号で記録して古い方の状態を `置換` にする

## 一覧

| # | タイトル | 状態 |
|:--|:--|:--|
| [0001](0001-channel-id-algorithm.md) | ChannelID は peercast-yt 互換の XOR アルゴリズムで生成する | 遡及記録 |
| [0002](0002-x-peercast-pos-resume.md) | `x-peercast-pos` による途中参加の位置決定 | 遡及記録 |
| [0003](0003-no-push-connection.md) | push (GIV) 接続は対象外とする | 遡及記録 |
| [0004](0004-helo-ping-firewall-check.md) | `helo.ping` によるファイアウォール疎通確認 | 遡及記録 |
| [0005](0005-listener-relay-counting.md) | 視聴者数・リレー数は実接続数と下流報告値の合算 | 遡及記録 |
| [0006](0006-interface-segregation.md) | パッケージ間依存はインターフェースで切る | 遡及記録 |
| [0007](0007-stream-key-store.md) | ストリームキー管理を Manager から StreamKeyStore に分離 | 遡及記録 |
| [0008](0008-host-atom-builder.md) | Host アトム構築を pcputil.BuildHostAtom に集約 | 遡及記録 |
| [0009](0009-jsonrpc-api-design-policy.md) | JSON-RPC API は互換性より peercast-mi 独自の使いやすさを優先 | 採用 |
| [0010](0010-bump-channel-named-params.md) | `bumpChannel` だけ名前指定 params も受け付ける | 採用 |
| [0011](0011-cors-policy.md) | JSON-RPC の CORS はループバック + 許可リストのみ | 採用 |
| [0012](0012-relay-lifecycle-in-manager.md) | オンデマンドリレーの組み立ては Manager.StartRelay に置く | 採用 |
| [0013](0013-docs-follow-code.md) | 仕様書と実装が食い違ったら仕様書を実装に合わせる | 採用 |
| [0014](0014-stream-on-demand-relay.md) | `/stream/` も `/pls/` と同じ規則でオンデマンドリレーを開始する | 採用 |
| [0015](0015-stream-position-wrap.md) | ストリーム位置は 32 bit で一周する 1 つの空間として扱う | 採用 |
| [0016](0016-relay-request-source-policy.md) | オンデマンドリレーの開始要求は送信元で制限し、接続先では制限しない | 採用 |
| [0017](0017-http-wait-for-info.md) | HTTP 視聴は ChannelInfo を待ってから 200 を返す | 採用 |
| [0018](0018-comparison-corrections.md) | 比較で判明した通知・状態・再接続の差異を修正する | 採用 |
| [0019](0019-authenticated-site.md) | 公開 PCP を維持し、サイト利用だけ X 認証で制御する | 採用 |
| [0020](0020-local-development-login.md) | loopback 限定の開発用ログインを設ける | 採用 |
| [0021](0021-yp-channel-directory.md) | YP 番組カタログを管理 API と視聴サイトで共有する | 採用 |
| [0022](0022-viewing-pages-and-comments.md) | ページ分離・自動再生・掲示板コメント閲覧 | 採用 |
| [0023](0023-site-base-path.md) | サイトの公開パスを明示設定する | 採用 |
| [0024](0024-site-admin-x-allowlist.md) | 指定Xユーザーだけにサイト管理APIを提供する | 採用 |
| [0025](0025-mobile-stream-keys.md) | サイト発行キーを小文字英字2文字＋数字4桁にする | 採用 |
| [peercaststation-compat](peercaststation-compat.md) | PeerCastStation との挙動合わせ (複数項目) | 採用 |
