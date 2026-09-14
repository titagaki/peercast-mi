# X 認証付きサイトの初期実装

追記: 同日の後続依頼で [YP 一覧・カタログ経由の中継開始](2026-09-14-yp-directory.md) を追加した。以下は初期実装時点の記録。

日付: 2026-09-14。設計は [ADR 0019](../decisions/0019-authenticated-site.md)、有効化手順は [サイト仕様](../spec/site.md)。既存 UI 改善を含む未コミット作業ツリーに追加し、既存設定ファイル・秘密情報・稼働プロセスは変更していない。

## 実装範囲

- オプトインの利用者向けサイトと `/watch`。X OAuth 2.0 / PKCE、セッション、CSRF、ログアウト。
- 自ノードの配信一覧・検索、FLV 再生、ネイティブ音量 / ミュート、全画面、PiP、プレイヤーのみの再接続。
- X ID に紐付く本人専用のキー生成・再発行、配信枠作成・停止。管理 API をサイトに転送しない。
- サイト有効時の HTTP 視聴入口保護、全体 / X ID ごとの同時視聴数制限、失効時のプロキシ接続キャンセル。停止した読み手への書込は 15 秒の期限付き。
- 公開 PCP 中継・ping は認証対象外。サイト無効なら HTTP 視聴も従来どおり。
- RTMP ハンドラーのキー入りログと未発行キーのエラー文を修正。キー保存の並行書込を直列化し、発行保存失敗時のロールバックと他アカウントの重複キー拒否を追加。

## 検証

| 検証 | 結果 / 対象 |
|:--|:--|
| `go vet ./...` | 成功 |
| `go test ./...` | 成功。最初の実行は追加テストの PCP 未登録応答を 503 と誤記して失敗。実装の 404 を確認し期待値を訂正後に成功 |
| `go test -race ./internal/site ./internal/channel ./internal/servent` | 成功 |
| サイト Go テスト | 未認証・CSRF / Origin 不正・管理 RPC 非公開・所有権・キー再発行・保存失敗・OAuth PKCE / callback 再送 / 期限 / 拒否 / 外部 API 障害・同一 ID の別セッションを含む視聴枠・ログアウトによる接続終了 |
| servent 入口テスト | `/stream/` / `/pls/` の token 欠如拒否、正しい内部 token は通常のチャンネル解決へ進む、PCP は token なしで通常処理へ進む |
| `npm run lint` / `npm run build` | 成功 |
| Playwright 通常テスト | 既存管理 UI 30 ケース + サイト操作 6 ケースが成功（desktop / mobile 幅 / dark）。API は模擬応答 |
| 生成 FLV のデコード | FFmpeg でテストパターン映像 + 音声を H.264/AAC FLV として生成し、ブラウザーで映像幅 160 のデコード結果を検査。明示的な再生開始前はメディア要求 0 件。desktop / mobile 幅 / dark の 3 ケース成功。模擬 HTTP メディア応答を使用 |

最終実行では `PEERCAST_TEST_FLV` を指定して全 39 ケースが成功。desktop / mobile 幅の再生スクリーンショットを目視確認し、狭い画面の横はみ出しもテストで検査した。

制限環境内では一部 Go テストの socket 作成が拒否されたため、許可を得て実行。ブラウザー実行は前回の `/tmp/peercast-ui-browser.96pdOY` の共有ライブラリ・フォントを使用。Prettier の取得は DNS 制限で失敗したため、許可を得て再実行した。依存 audit は以前からの 9 件を継続して検出し、今回まとめて自動更新していない。

最終 `go vet` もビルドキャッシュの書込権限で一度失敗し、許可を得た再実行は成功した。

任意の生成映像テストの再現方法（FFmpeg が必要、秘密情報や実配信を使用しない）:

```sh
ffmpeg -f lavfi -i testsrc=size=160x90:rate=10 -f lavfi -i sine=frequency=440 -t 2 -c:v libx264 -pix_fmt yuv420p -preset ultrafast -c:a aac -f flv /tmp/peercast-site-test.flv
cd ui
PEERCAST_TEST_FLV=/tmp/peercast-site-test.flv npm test -- --grep 'generated FLV'
```

この環境変数を指定しない通常の `npm test` では生成映像テスト 3 ケースを skip する。

## 未検証・後続作業

X アプリの実資格情報は未設定のため、実アカウントでの認証往復・契約上の利用可否・課金は未検証。公開 HTTPS / RTMPS 終端の設定、実際の OBS → Go → サイトの長時間再生、PCP 他実装との実機中継、Safari / iPhone 実機は未検証。生成 FLV のデコードや Go 単体テストをこれらの完了とは扱わない。

外部 YP 一覧・掲示板内蔵閲覧・お気に入り同期・通知・HLS は未実装。現段階を peca-live の全機能同等とは呼ばない。アカウント停止・承認制・利用者別レート制限 / 帯域割当・監査運用も後続候補。具体的な未着手作業は [tasks.md](../tasks.md) に記録する。
