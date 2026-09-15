# pcgw を参考にしたチャンネル作成・履歴・新スレ移動

2026-09-15。依頼範囲は5項目の配信フォーム、前回の初期値、履歴選択、コンタクトURLの掲示板情報と新スレ移動。

## 参照範囲

ローカル pcgw `8cab31088104089f0649eef908594e570f26fbce`。作業ツリー clean。GitHub 最新との差分は未確認。

| 根拠 | 確認した動作 |
| --- | --- |
| `routes/broadcast.rb` の `GET /create` | 本人の直近設定を初期値にし、最近10件を選択肢にする。指定テンプレートも本人のものに限定 |
| `routes/broadcast.rb` の `POST /broadcast` | 作成成功時に ChannelInfo を保存 |
| `views/form.slim`, `views/select_template_dropdown.slim` | チャンネル名・ジャンル・詳細・コメント・コンタクトURL、履歴の名前・日時・概要 |
| `routes/bbs.rb` の `/bbs/info`, `/bbs/latest-thread` | 板名・スレ名・レス数を取得。BBS_THREAD_STOP（既定1000）未満のスレからID最大を選択 |
| `public/bbs_checker.js` | 新スレ移動は確認ダイアログ後にコンタクトURL入力を変更 |
| `lib/bbs_reader.rb` の board settings URL | したらば setting.cgi と互換掲示板 SETTING.TXT |

## 実装と差異

- `BroadcastForm` は指定5項目を縦に配置。本人の最新履歴を初期値とし、名前・日時・概要から以前の設定を選択。定期取得で編集中の値を上書きしない。
- サーバーの `broadcastInfo` / `broadcast` と `broadcast_history.go` で本人の直近10件を保存・返却。ジャンルは yp 補完前の入力を保存。
- 保存先は MySQL / ActiveRecord ではなくユーザー別 JSON。[ADR 0029](../decisions/0029-broadcast-history.md) に選択理由を記録。
- `broadcastBoard` は既存の許可ホスト・パス・公開IP制限付き reader を利用。対象はしたらば / jpnkn。フォームの任意URLを直接 fetch しない。
- 新スレは未満員のスレの数値ID最大。subject.txt の先頭200件に限定しない。既存スレが一覧から消えている場合は本文で補完を試み、取得不可でも新スレ候補は返す。
- 意図的な差異: フォームだけの可逆な変更のため、新スレ移動の確認ダイアログは設けない。新しいスレを作成したり、配信中のチャンネル情報を書き換えたりはしない。
- 意図的な差異: 初回履歴なしでは空欄。pcgw のユーザー名の初期入力や、ソース・ストリームタイプ・掲載YP・配信サーバー等の追加項目は今回の依頼対象外。
- 未確認: 公開 pcgw との接続比較、各掲示板の現在の実スレッドでの比較。本テストは HTTP fixture による再現であり外部との相互接続確認ではない。

## 検証

- Go: 履歴の再読込・所有者分離・キー再発行・10件上限・ファイル権限・保存失敗・破損時保護。掲示板の最新未満員選択・満員スレ除外・一覧外スレ・許可URL・認証・取得失敗。
- ブラウザー: 前回値・5項目の履歴切替・定期更新時の編集保持・新スレのURL差替え・読み込み失敗・URL変更後の遅延応答を確認。

実行結果:

- `go vet ./...` / `go test ./...` 成功。
- `npm run lint` / `npm run build` 成功。ビルドは既存のチャンクサイズ警告あり。
- Playwright 全69件成功（desktop / mobile / dark、生成FLV再生を含む）。新フォームのPC・モバイル画像も確認。
- `npm run test:proxy` 成功。`git diff --check` と追加ドキュメントの相対リンク確認も成功。
- 初回の Go キャッシュ・Vite 起動・Prettier 取得は実行環境の制限により失敗し、承認済みの制限外実行で成功。
- 公開掲示板の実データでの新スレ移動とDocker再作成は未実施。Dockerの既定履歴パスは起動コードで設定ファイルのディレクトリ配下に配置し、既存 `/config` 永続マウントを利用することをコードで確認。
