# YP 番組一覧・認証付きオンデマンド視聴

追記: 再生ボタンと YP 診断表示は [後続のページ分離・掲示板対応](2026-09-14-viewing-pages-comments.md) で変更した。以下はカタログ追加時の記録。

追記: ジャンルの元データ保持は維持したまま、サイト表示層には [YP4G 制御部分の除去](2026-09-14-site-genre-controls.md) を追加した。

調査・実装日: 2026-09-14。`updateYPChannels` 相当の実装依頼と追加の YP 資料指定に対応。

## 参照範囲

| リポジトリ | HEAD | 作業ツリー |
|:--|:--|:--|
| peercast-mi | `0c9f1ed` | 先行の開発プロキシ、開発ログイン、config、文書の未コミット変更あり。保持して追加実装 |
| peca-live | `e21a01640fcc691a70818255b7ba2ece37d999f6` | Gemfile / Gemfile.lock に変更あり。参照のみ |
| PeerCastStation | `33e4849a974b2db7ba80a38f5627da8e7d910b7c` | clean |
| peca-docs | `820ca526127643e987c4d91d1a8d2ec0551ce63d` | clean |
| peercast-0yp | `80325f2b3f8534412d351f5547d6cc23132a8771` | clean |

ユーザー指定の [peca-docs/docs/yp](https://github.com/titagaki/peca-docs/tree/main/docs/yp) と [peercast-0yp/docs/yp](https://github.com/titagaki/peercast-0yp/tree/main/docs/yp) は Web ツールで取得できなかったため、ローカル checkout の同ディレクトリの `player.md` / `genre.md` を全文確認した。GitHub main の最新との差分は未確認。

- peca-live: `lib/json_rpc.rb#update_yp_channels`、`app/controllers/api/v1/channels_controller.rb#get_channels/fetch_channels/yp_channel?`、`app/javascript/packs/types/Channel.ts`。RPC 呼び出し・60 秒キャッシュ・ゼロ ID 除外・tracker を用いた視聴を確認。
- PCS: `PeerCastStation.PCP/PCPYellowPageClient.cs#GetChannelsAsync/ParseUptime/ParseStr`、`PeerCastStation.UI/YPChannelList.cs#UpdateAsync`、`PeerCastStation.UI.HTTP/APIHost.cs#YPChannelsToArray/UpdateYPChannels`。
- YP 資料: UTF-8・19 フィールド・`<>` 区切り、H:MM、非表示人数 -1、URL 導出、ジャンル制御の責務を確認。`NameURLEncoded`、`click`、`DirectFlag` は今回の PCS 型 RPC / FLV 再生では使用しない。統計・チャット UI の追加は対象外。
- 0yp 実装: `internal/httpd/server.go` の `/yp/index.txt` 登録、`indextxt.go#writeIndexLine/genreDisplay/writeStatusLine/writeInfoLine`。告知行の区切り位置には資料の通常行と一致しない箇所があるが、ゼロ ID でサイト視聴対象から除外する。参照実装は変更しない。

## 実装と意図的な差異

| 項目 | 参照実装 | mi / 理由 |
|:--|:--|:--|
| 取得元 | PCS の ChannelsUri | `[[yp]].channels_url` を明示。PCP 登録先から一覧パスは導出できない |
| RPC | `updateYPChannels` / PCS 型の配列 | 名前・使用フィールドを採用。API 全体互換ではない（ADR 0009 の例外） |
| キャッシュ | PCS 18 秒、peca-live の Rails 60 秒 | 共通 60 秒。要求集中を集約し YP の負荷を抑える |
| 取得失敗 | PCS は失敗 YP の空一覧と通知 | 最終成功から 5 分未満だけ保持し、サイトに失敗・古い一覧を明示 |
| 一覧 | peca-live は YP 一覧、ゼロ ID 除外 | サイトは YP + 自ノードを統合。RPC は告知行・重複 ID を維持 |
| 接続先 | peca-live のブラウザーが tracker を tip に指定 | サイトは ID のみ受理。カタログから public IP tracker を選ぶ。任意接続を防止 |
| 人数・ジャンル | YP が制御記号を処理し -1 等を出力 | -1 と掲載済みジャンルを保持。人数の推定復元・ジャンル制御の再適用はしない |
| 取得上限 | PCS 全体 5 秒 | 5 秒 + HTTP 並列 4 + 4 MiB/YP + 行 64 KiB 未満 + 10,000 有効行/YP |
| 中継開始 | peca-live は stream URL 要求 | 明示的な再生時のみ。認証・視聴枠・サイト中継上限（既定 8）・Manager 上限を適用 |
| PCP | 公開中継 | 変更なし。サイトからの新規接続制御と公開 PCP の選択規則は分離 |

決定理由・却下案は [ADR 0021](../decisions/0021-yp-channel-directory.md)、現仕様は [site.md](../spec/site.md) と [JSON-RPC](../spec/api/jsonrpc.md)。

## 検証

- 単体: 19 フィールド、UTF-8、HTML エンティティ、H:MM、非表示人数、告知、不正応答・サイズ、キャッシュ集約・失敗・期限・復旧・部分障害・呼出元キャンセル。
- RPC: ローカル HTTP の index.txt を取得し、返却の型・値・設定なし `[]` を検証。
- サイト: ローカル HTTP → 実 catalog → 認証済み一覧 → Manager.StartRelay（上流は fake）→ 内部プロキシ（transport は mock）。未認証で YP 取得なし、一覧で中継なし、ゼロ ID 除外、内部 IP と任意 tip 拒否、視聴枠・中継枠、重複中継防止、接続待ちメタデータ維持を確認。
- `go vet ./...`、`go test ./...`、`go test -race ./internal/catalog ./internal/site ./internal/jsonrpc ./internal/channel ./internal/servent`: 成功。loopback listen は sandbox 内では拒否されたため、許可を得て再実行。
- UI: `npm run lint`、`npm run build`、Playwright 51 件成功（desktop / mobile / dark）。YP 取得失敗・未設定・接続不可表示・一覧更新後の選択維持と、明示的な再生前の映像要求なしを確認。生成 H.264/AAC FLV を mock HTTP から実ブラウザーでデコードするテストを含む。
- 実 YP の読み取り: `https://yayaue.me/yp/index.txt` は HTTP 200、告知行のみ。`http://bayonet.ddo.jp/sp/index.txt` は HTTP 200、FLV 番組行あり。この 2 URL を既存 config に追加した。
- 0yp のルート `/index.txt` は 404。p@YP の候補ホストは名前解決できず、一覧 URL を設定していない。ユーザーの PCP 登録先は変更していない。
- 任意実行の `TestLiveDirectory`（`PEERCAST_TEST_YP_URL` を指定）でも本実装の取得・解析を確認。0yp は 2 行 / 告知以外の public tracker 0 件、SP は 24 行 / 同 19 件。これは取得時点の結果で、常時の番組数ではない。通常のテスト実行では外部通信せず、このテストは skip する。

外部 YP 取得と PCP 映像中継の成功は別。実 YP の番組を PCP 受信してブラウザーで再生する一連の実機確認、X 本番ログイン、スマホ実機、push/GIV は未検証。ユーザーの稼働中ノード・Vite は再起動していない。
