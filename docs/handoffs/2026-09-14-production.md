# 本番導入セッション終了時の引き継ぎ — 2026-09-14

前の [同日引き継ぎ](2026-09-14.md) は本番導入前の履歴。本メモを優先する。ユーザーは最後に「配信できました」と確認し、セッションを区切るよう依頼した。新しい実装に着手せず次の依頼を待つ。

## 保存状態

終了処理開始時、miとyayaue.meの作業ツリーはclean。mi実装・設定例は984a542まで、インフラは089b47fまでpush済み。今回追加する終了記録のコミットはgit logで確認する。秘密の.env・inventory・キーは記録しない。

## 現在の運用と合意

- 本番はhttps://yayaue.me/mi/。Xログイン、一覧視聴、管理パネルのチャンネル・キー・ノード情報の読み込みをユーザー確認済み。
- RTMPは公開1935、ストリームキー認証。RTMPS・SSHトンネルはユーザー指定で使わない。PCPは7154、0ypは7144。
- 配信掲載先は0yp固定。サイト一覧の取得先は0yp・SP・p@YPの3件。0yp限定は途中の誤解で、復元済み。YP未掲載のローカル枠をサイト一覧へ追加しない。本人用配信ページと管理パネルでは確認可能。
- サイト管理者はX ID 121039382。Basicは直接ノードAPI用として存続、Xサイト管理とは別。
- 配信キーは小文字英字2文字＋数字4桁。既存キーは再発行まで維持。短いキーのPublish制限は1分でIPごと10回・ノード全体120回。
- 配信入力は名前・ジャンル・説明・コメント・URL。ビットレート入力は削除、作成時0、RTMPメタデータで反映。ジャンルにypがなければ自動補完。
- 同一Dockerネットワークの0ypがOLEHで内部IPを返す問題を確認。public_ipv4を追加し本番では153.127.50.98。ユーザーは直書きを了承。HOST公開側IPを上書きし、OLEHのポート判定は維持する。
- ローカルconfig.tomlはRTMP1945/PCP7154、dev_login=true。public_ipv4とadmin_x_idsの本番値はコメント例のみ。サンプルBasic資格情報はコメント化。非loopbackから直接管理したい場合は明示設定が必要。

## 配置・更新

アプリのVPSソースは/opt/peercast-mi。VPSでsudo git -C /opt/peercast-mi pull --ff-only（ユーザーのupdate-peercast-mi.shでも可）。

本番設定は手元 ~/src/yayaue.me/docker/peercast-mi/config.toml → Ansible → /opt/yayaue.me/data/peercast-mi/config.toml。mi直下のconfig.tomlは本番には使わない。配信キー・broadcast_idはdata/peercast-miで保持。インフラのVPS git pullは不要。

手元WSLで実行:

```sh
cd ~/src/yayaue.me
export ANSIBLE_COLLECTIONS_PATH="$PWD/ansible/collections"
ansible/.venv/bin/ansible-playbook -i ansible/inventory.yml ansible/deploy.yml --ask-become-pass
```

checkモードはファイル差分のみでビルド・起動なし。ユーザーがVPS/WSLコマンドを実行し結果を貼る運用。指示では実行場所を明記する。

## 確認済みと未確認

- 公開IP修正前の本番ログでRTMP接続・キー認証・stream started成功。公開0yp APIでいまいch掲載と内部tracker=172.20.0.4を直接確認した。ビットレート0は未受信の証明ではない。
- 外部から153.127.50.98:7154のTCP接続成功。
- 修正案内後、終了時にユーザーが配信成功を確認。最終VPSコミット・最終Ansible出力は提示されていないので断定しない。
- 修正後tracker値の独立再取得、他のPeerCastノードからの再生は未確認。ユーザーの「配信できた」と区別する。
- 管理接続一覧のsource行はsendRate/recvRateとも0固定。実測入力速度の表示は未実装。Receiving状態とは別。残課題をtasks.mdへ保存した。

## 検証・参照

各Go変更後にgo vet ./... / go test ./...成功。UI変更時lint/buildと配信ワークフローdesktop/mobile/dark成功。既存の500kB超チャンク警告は残る。ローカル/本番TOMLパース・差分検査済み。終了記録は文書のみでアプリ再テストは不要。

- ADR0024〜0028: X管理認証、短いキー、YP掲載分一覧、yp接頭辞、公開IPv4。
- reviews/2026-09-14-yp-publication-genre.md: BCST・掲載・内部IPの調査。
- reviews/2026-09-14-config-inventory.md: 全設定棚卸し。
- reviews/2026-09-14-broadcast-fields.md: pcgw参照、入力・停止・ビットレート。
- インフラdocs/production.md: 本番観測とユーザー報告。
