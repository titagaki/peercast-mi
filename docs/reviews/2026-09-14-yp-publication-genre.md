# 0yp掲載とジャンル接頭辞の確認

2026-09-14。mi `ef2eea2e819262fd4f098b70dae9842d24295c4e`、0yp `c278834849977b4c725eaad85626b75fe5bf7b51`。調査開始時の両作業ツリーはclean。0yp参照元は `/home/megan/src/go/peercast-0yp`。対象はサイト入力からBCST生成・送信、0ypのBCST受理・掲載条件。外部YPの掲載規約は対象外。

## 確認できた差異・問題

- mi `ui/src/SiteApp.tsx` の配信ジャンルは初期値が空。`internal/site/broadcast.go` の `broadcast` はジャンルをそのまま `Manager.Broadcast` に渡す。`ChannelInfo.ToPCP` と `yp.Client.buildBcst` も接頭辞を補わない。
- 0yp `internal/pcp/server.go` の `processBcst` はHOSTのRecvフラグが真でCHAN情報があるときだけ `Store.AddHit` を呼ぶ。Recvが偽なら掲載ヒットを削除する。
- 0yp `internal/channel/channel.go` の `Store.AddHit` は非ゼロChannelID、非空名、小文字 `yp` で始まるGenreを要求する。既存ChannelIDのBroadcastID不一致も拒否する。`ゲーム`、空文字、`YPゲーム` は拒否、`ypゲーム` は接頭辞条件を満たす。接頭辞不一致時に拒否ログや通知は出さない。
- miのフォームが受理するジャンルと0ypの掲載条件が一致していない。過去の配信フォーム検証はこの条件を検証していなかった。

## BCST送信経路

`Client.Run` は配信等のチャンネルができるまで待つ。`run` はハンドシェイク後に初回BCSTを送る。`buildBcst` はROOT宛BCSTにCHAN（ジャンルを含む）とHOST（IsReceivingをRecvへ反映）を格納する。`sendAnnouncements` は配信チャンネルを対象に、1秒間隔の状態比較による変更通知、定期通知、Bump通知、停止通知を行う。サイト配信枠作成時もBumpする。RTMP接続の認証成功だけではメディア受信中とは判定しない。

## 検証

- mi既存テスト `TestBuildBcst`、`TestRun_HandshakeAndBcst`、`TestRun_BumpSendsBcst`、`TestCompatAnnouncementChangesAndRemoval`: 計4件成功（キャッシュ不使用）。BCST構築、模擬YPとのローカル送受信、変更・停止時のフラグを確認。
- 0yp既存 `TestAddHit*`: 計5件成功（キャッシュ不使用）。非ypジャンル・空名の拒否、BCID所有権など。初回はサンドボックスのGoキャッシュ書込制限で失敗し、権限付き再実行で成功。
- 新規の実機相互接続チェックは未実施。既存の模擬YPテスト成功を本番BCSTの到着確認とはしない。

## 未確認と修正候補

本番で入力された実際のGenreと受信中フラグ、本番BCSTの送信・到着、VPS上の0ypの版は未確認。貼られたログにはBCST受理の証拠がない。

当面は管理画面の対象チャンネルのジャンルを `ypゲーム` などへ編集し、映像受信中に掲載を確認する。恒久対応候補は、サイトの固定掲載先の要件として接頭辞を自動補完し、既存の `yp` 制御文字列は二重に付けないこと。管理APIや他YP全体への一律補完は別途影響を検討する。今回は調査依頼のため実装は変更していない。意図的に接頭辞補完を省略したという判断記録は確認できなかった。

## 続報: 未掲載ローカル枠の一覧混入

ユーザーの追加報告に対して `internal/site/directory.go` の `channelList` を確認。カタログにないローカルチャンネルを末尾へ追加していた。これを削除し、カタログに掲載されたIDだけを一覧へ返すよう変更した。サイト本人用配信APIと管理APIのローカル表示は維持。yayaue.meの本番設定からSP・p@YPのchannels_urlを除き、0ypだけを一覧取得元にした。ジャンルの自動補完はこの変更に含めない。

`TestDirectoryExcludesUnlistedLocalBroadcast` を追加。取得元未設定・空一覧・掲載・掲載終了で両一覧APIの件数を確認し、未掲載でも本人が配信枠を確認できることを検証。`go vet ./...` と `go test ./...` は成功。本番TOMLのパースと取得URLが0ypだけであることも成功。本番適用は未実施。

### 取得元の訂正

ユーザーへの再確認で、0yp限定は依頼の誤解と判明した。3YP取得を維持し、未掲載ローカル枠だけを除外する意図だった。yayaue.meでSP・p@YPのchannels_urlを復元し、TOMLパースと3件のURL・順序を検証済み。miの一覧実装はそのまま。

### 掲載用接頭辞の修正完了

サイト作成時のジャンルにypを自動補完した。pcgw `8cab31088104089f0649eef908594e570f26fbce`（作業ツリーclean）の `routes/broadcast.rb` のgenreも `models/yellow_page.rb` のadd_prefixを呼び、掲載先の接頭辞がなければ付ける。miでは固定掲載運用のypを補完する。pcgwの任意掲載先別prefix設定は導入しない。

`TestBroadcastAddsPublicationGenrePrefix` で空欄・通常ジャンル・前後空白・既存接頭辞・制御文字列を検証し、ChannelInfoからPCPへ渡るGenreも確認した。`go vet ./...`、`go test ./...` は成功。本番への適用と実際の掲載は未確認。既存枠はジャンル編集または停止後の作り直しが必要。
