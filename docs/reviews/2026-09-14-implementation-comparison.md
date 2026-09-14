# PeerCastStation・peercast-yt・peercast-mi 実装比較

調査日: 2026-09-14。結論として、3 実装は一致していない。peercast-mi には記録済みの意図的な差異がある一方、BCST の配送、接続枠の広告、送出開始・タイムアウトなどには、意図的と確認できない差異も残っている。

本書の比較表・再現チェックは修正前 HEAD の調査記録として保存している。調査後にユーザーから理由のない差異の修正を依頼され、同じ作業ツリーで修正した。現在の状態は [修正・検証結果](2026-09-14-comparison-corrections.md)、判断理由は [ADR 0018](../decisions/0018-comparison-corrections.md) を参照する。以下の「未決定」「残っている」は調査当時の状態である。

## 比較対象と方法

| 実装 | ローカルチェックアウト | HEAD |
|:--|:--|:--|
| peercast-mi (以下 mi) | `~/src/go/peercast-mi` | `1e62a304de2dca57a5ed53a52963ed83eee45fac` |
| PeerCastStation (以下 PCS) | `~/src/peercaststation` | `33e4849a974b2db7ba80a38f5627da8e7d910b7c` |
| peercast-yt (以下 yt) | `~/src/peercast-yt` | `b60f176317406e79a5468ba80da8be1d83bb6126` |

- 調査開始時、PCS・yt の作業ツリーは clean。mi は直前の依頼による `CLAUDE.md` / `AGENTS.md` の変更のみで、Go 実装の未コミット変更はなかった。
- mi の PCP 依存は `github.com/titagaki/peercast-pcp v0.3.1`。今回は主に各アプリケーションの呼び出し・状態遷移を比較し、依存ライブラリのアトムパーサ全体の監査はしていない。
- 対象は識別子、配信入力、PCP の接続・中継・HOST/BCST、バッファ・HTTP 視聴、YP 通知、JSON-RPC の主要操作。UI、全コーデック、全 API メソッド・エラー・設定項目の完全な対応表は対象に含めない。
- ローカルのソースを根拠にした静的比較と、mi の限定的な再現チェック。3 プロセスを接続した相互運用試験は未実施。「同じ」も表に挙げた条件に限り、完全互換を意味しない。
- 以下の外部ソースリンクは上記の `~/src` 配置を前提とする相対リンク。移動した場合は、表のコミットとファイル・関数名で追跡する。

## 以前の調査と理由の所在

既存のまとまった記録は [PCS 互換性ノート](../decisions/peercaststation-compat.md) と [ADR 一覧](../decisions/README.md)。特に [0001](../decisions/0001-channel-id-algorithm.md)、[0003](../decisions/0003-no-push-connection.md)、[0009](../decisions/0009-jsonrpc-api-design-policy.md)、[0014](../decisions/0014-stream-on-demand-relay.md)、[0015](../decisions/0015-stream-position-wrap.md)、[0016](../decisions/0016-relay-request-source-policy.md)、[0017](../decisions/0017-http-wait-for-info.md) に比較・判断理由がある。

ただし、既存ノートの「互換」「修正済み」は現在の 3 実装の一致を保証しない。今回、次の記述を再評価した。

| 過去の記述 | 現在のコードで確認したこと |
|:--|:--|
| Overflow の 5 秒判定を PCS に合わせた | 時間の基準が違う。mi は受信済みコンテンツの古さ、PCS は出力キューへの投入時刻差。D04 参照 |
| 劣勢リレーの退出を PCS に合わせた | mi の対象は firewalled のみ。PCS の「満杯かつ下流 0」や local 除外とは一致しない。D07 参照 |
| 下流報告値は切断まで残る「PCS も同様」(ADR 0005) | PCS の `Channel.HostsView` は 180 秒を超えたノードを集計から除く。mi の stats に同じ期限はない。D06 参照 |
| 満杯時は自ノード HOST → 代替 HOST → QUIT | 現在の mi は代替 HOST のみ。同ノートの別節には既に削除済みと記録されており、後段の説明が古い |
| `/stream/` は応答前に待たない | ADR 0017 で変更済み。現在は ChannelInfo.Type を最大 10 秒待ち、時間切れは 504 |
| PCS の YP `OnPCPRoot` に合わせて即時更新 | 今回の PCS の `PCPYellowPageClient.cs` には同名処理がなく、受信ディスパッチは BCST/QUIT、通常通知は 30 秒周期。過去に参照した版の挙動を現行版へ持ち込めない。D10 参照 |
| `x-peercast-pos=0` はいずれも「全部送る」 | mi の最古からの送出と PCS の最新の開始可能コンテンツ選択は違う。0 と未指定の扱いが近いことと、初期バックログの選択が同じことは別。D04 参照 |
| `broadcastChannel` は mi 独自 (ADR 0009) | 名前自体は PCS の `APIHost.BroadcastChannel` にも存在する。mi 独自なのは streamKey ベースの引数・開始モデル。I03 参照 |

## 記録済みの意図的な差異

「意図的」は ADR・スコープ・既存コメントに根拠がある範囲だけに使用する。理由の欄は既存記録の要約であり、本調査で新しく採用した判断ではない。

| ID / 項目 | mi | PCS | yt | 記録された意図・理由 |
|:--|:--|:--|:--|:--|
| I01 ChannelID | XOR 方式。name に NUL と StreamKey を連結 | SHA512(bcid) と名前・genre・source 等をシリアライズして MD5 | `GnuID::encode` による XOR。通常は name/genre、HTTP push では name/mount。ランダム化オプションあり | [0001](../decisions/0001-channel-id-algorithm.md): yt のアルゴリズムを採用しつつ、別アカウントの同名配信を区別。生成 ID の移行互換は約束していない。[S01](#s01-識別子) |
| I02 GIV / push | 非対応 | PCP push の接続処理を確認できず、mi と同様に非対応 | `acceptGIV` / `givProc` がある | [0003](../decisions/0003-no-push-connection.md): ポート開放済みノードとして動作する前提で、接続管理を簡単にする。[S02](#s02-入力と-giv) |
| I03 配信の開始方法と API | 発行済み StreamKey + RTMP push + `broadcastChannel([{streamKey,info,track}])` | `broadcastChannel` は sourceUri・コンテンツリーダー等を指定。複数のソース方式 | HTTP push、URL ソース等。API の構成も異なる | [0007](../decisions/0007-stream-key-store.md)、[0009](../decisions/0009-jsonrpc-api-design-policy.md) とスコープ: mi の主要クライアントの都合を優先。**同名メソッドでも代替 API ではない**。[S02](#s02-入力と-giv)、[S09](#s09-api) |
| I04 JSON-RPC params | 原則位置指定。`bumpChannel` だけ名前指定も受理 | 名前指定を含む RPC 呼び出し | メソッドごとの引数名を持つ RPC 呼び出し | [0009](../decisions/0009-jsonrpc-api-design-policy.md)、[0010](../decisions/0010-bump-channel-named-params.md): peca-live 用に引数形式のみ例外対応。ただし処理の意味は D08 のとおり違う。[S09](#s09-api) |
| I05 リレー開始の送信元制限 | 既定は private。登録済みの視聴はこの制限を受けない。`max_relay_channels` も用意 | 既定のリスナー設定ではグローバルからの視聴自体を制限 | リレー開始は private または有効な auth token。視聴フィルタは別 | [0016](../decisions/0016-relay-request-source-policy.md): mi の公開 HTTP 視聴を維持。tip の接続先 allowlist は設けず、要求元を制限する。[S08](#s08-http-視聴) |
| I06 内部のストリーム位置 | uint32 と `PosBefore` の符号付き差分 | 内部 long、PCP 送出時に下位 32 bit | unsigned なストリーム位置・パケットバッファ | [0015](../decisions/0015-stream-position-wrap.md): 受信側で 64 bit の上位を復元する仕組みを増やさず、一周を扱う。開始位置の細部まで同一ではない。[S05](#s05-コンテンツと送出) |
| I07 上流再選択 | PCS 由来の重み付きランダムスコア、3 分 ignore、即時次候補、tracker 失敗等で終了 | 同系統のスコア・ignore・停止条件 | `pickFromHitList`: local relay → global relay → local tracker → global tracker と待機条件 | [既存ノート](../decisions/peercaststation-compat.md): ノード切り替えは PCS に合わせる。ホスト枯渇で終了するため、yt と同じ探索手順にはならない。候補抽出自体の違いは D12。[S03](#s03-上流接続と再選択) |
| I08 ICY メタデータ挿入 | 非対応 | HTTP 出力に yt の ICY メタデータ挿入相当は確認できず | MP3 向け `icy-metaint` / `sendRawMetaChannel` | [tasks 完了欄](../tasks.md)、[components 4.9](../spec/components.md): FLV 配信の対象にはしない。[S08](#s08-http-視聴) |

API の独自設計を許す方針は、すべての現状の挙動について意図・理由が記録されていることを意味しない。例えば `bumpChannel` の「再接続しない」という判断は、引数互換の ADR だけからは導けない。

## 残っている差異・要検討事項

優先度は本調査での影響評価。「高」は通知やデータ送出・再接続に直接影響するもの、「中」は状態広告・探索品質・運用上の差異。修正の採否は未決定。表中の影響は、再現チェック欄に明記したもの以外はコード経路からの推論。

| ID / 優先度 | 項目 | mi | PCS | yt | 影響・意図の確認状況 |
|:--|:--|:--|:--|:--|:--|
| D01 / 高 | BCST の配送と宛先 | 下流からの BCST は他の PCP 出力へだけ配送。上流への配送経路がない。上流からは payload をローカル処理するだけ | `Channel.Broadcast` が group に従い source と sinks へ配送。宛先一致時も payload を処理 | `readBroadcastAtoms` が group に従い上流・COUT・CIN・RELAY に配送 | mi を中間に置くと下流 HOST が tracker に届かず、リレーツリー・全体集計・代替候補が欠ける。上流由来の一般 BCST も下流へ届かない。理由の記録なし。[S04](#s04-bcst-とノード表) |
| D02 / 高 | 満杯時の HOST 広告 | relay の `buildRelayBcstAtom` は RelayFull/DirectFull を渡さず、常に Relay ビット、global IP 取得後は Direct ビットも立つ。YP/PCP 出力もチャンネル単位の上限だけを参照 | `IsRelayable` / `IsPlayable` による実際の受付可否を反映 | `canAddRelay` / `directFull` 等を反映 | 枠・全体リレー数・送信帯域上限で拒否するノードを「空きあり」と案内しうる。共通ビルダー化しただけでは全呼び出し元が一致しない。理由の記録なし。[S06](#s06-host-広告と受付) |
| D03 / 中 | firewalled / Receiving の広告 | `BuildHostAtom` は Push ビットを立てない。`handleOleh` は rip のみ採用し port を状態に反映しない。Receiving は `HasData()` | listener の port status、source の受信率や状態を反映 | firewall 状態と `isPlaying` を反映。firewalled 時は global port を 0 にする | mi は受信済みデータが残っていれば受信停止後も Receiving。I02 の運用前提はあるが、疎通確認結果を広告へ反映しないこと自体の理由は記録なし。[S06](#s06-host-広告と受付)、[S07](#s07-yp) |
| D04 / 高 | 送出開始位置・Overflow・無通信 | 最古のバッファから開始し、最初の未送信パケットの**受信時刻**が 5 秒以上前なら QUIT+SKIP。データ追加なしも 5 秒で切断 | 初期バックログは `GetFirstContents` が最新の開始可能点を選択。Overflow は**出力メッセージ投入時刻差**が 5 秒を超えた場合。`SendRelayBody` は次の通知を待つ | `sendPCPChannel` はヘッダー後に rawData を送り、PCS/mi と同じ 5 秒の経過時間判定はない | 新規接続でも古いが有効なバックログを「遅い下流」と扱える。無通信と送信詰まりも混同。6 秒前のパケットで即 QUIT を再現。既存の互換説明では正当化できない。[S05](#s05-コンテンツと送出) |
| D05 / 高 | 上流ハンドシェイクの期限 | Dial は 10 秒だが、成立後の GET/helo/HTTP 応答/oleh に deadline がない。60 秒 ReadDeadline は body/hosts から | `PCPHandshakeTimeout=18000`。接続ストリームにも read/write timeout | `connectFetch` は tracker/YP 接続で read/write timeout を 30 秒に設定してから `handshakeFetch` へ進む | TCP 接続だけ成功して沈黙する上流で、手動停止等まで次候補へ進まない。mi の下流受付の 18 秒、YP 探索の 10 秒とは別の欠落。理由の記録なし。[S03](#s03-上流接続と再選択) |
| D06 / 中 | 代替 HOST と統計の期限 | 上流候補は 3 分で失効するが、下流側 knownHosts は最大 32 件で時刻なし。stats も時刻なし | `Nodes` / `SourceNodes` はともに `HostsView` で 180 秒を超える報告を除外 | `clearDeadHits` が dead/経過時間で削除。呼び出し元の期限は 180 秒 | 代替 HOST に退去済みノードが残り、通知をやめた子孫の数も残りうる。直下切断時の `RemoveNodeStats(peerID)` は子孫 SID をまとめて消さない。理由の記録なし。[S04](#s04-bcst-とノード表) |
| D07 / 中 | 劣勢ノードの退出条件 | firewalled を 1 つ退出。全体リレー数・帯域が満杯なら退出を試す前に拒否 | 非 local で firewalled、または RelayFull かつ下流 0 を退出。sink を外して再判定 | `isTerminationCandidate`: relay 不可かつ下流 0、または port 0 かつ GIV 非対応 | ポート開放済みでもリレーしないノードが枠を占有する条件で差が出る。mi の BAN 90 秒は存在するが、退出条件まで同じではない。差異の理由なし。[S06](#s06-host-広告と受付) |
| D08 / 高 | `bumpChannel` の意味 | channel の存在は検証するが、処理は全配信の YP 再通知のみ。YP 未設定なら成功して何もしない | 対象 channel の `Reconnect()` | 対象 channel の `bump=true`。`checkBump` で受信ループを終了して接続をやり直す | 接続先を変えたい視聴クライアントには互換でない。0010 は params の理由だけで、この動作差の理由はなし。[S09](#s09-api) |
| D09 / 中 | BroadcastID の永続化 | `main` で毎起動ランダム生成。StreamKey が同じでもプロセス再起動を跨ぐ ChannelID は維持しない | 設定から BroadcastID を復元し保存 | 設定の `broadcastID` を読み書き | 固定視聴 URL を期待する移行・再起動で差が出る。現状は仕様書に明記されているが、非永続化を選んだ理由は ADR に見つからない。I01 の入力差とは別問題。[S01](#s01-識別子) |
| D10 / 中 | YP 更新周期・停止通知 | 既定 120 秒、接続時の root.uint を反映。接続後は root.update のみ処理し uint 変更を無視。削除済み channel は次回一覧から省くだけ | 今回の版は 30 秒周期と変更通知。`ChannelRemoved` が playing=false の最終 HOST 更新をキューに入れる | `readRootAtoms` は受信ごとに uint を反映し update で tracker 更新要求 | 接続中の周期変更に追随しない。PCS と比べ配信停止の明示通知がなく、他チャンネルの告知を続ける場合の掲載削除が YP 側の失効に依存しうる。yt の停止時全経路との同等性は未確認。差異全体の理由なし。[S07](#s07-yp) |
| D11 / 中 | IPv6 PCP | HOST の IP を uint32 として生成・解析し、relay 要求は PCP 1 固定 | network type 別に PCP 1 / 100 を選択 | IPv6 の local/global HOST とアドレス書き込みに対応 | TCP リスナーが IPv6 を受けられることと、IPv6 PCP 互換は別。IPv6 の HOST 探索・広告を mi に期待できない。明示的な対象外判断は見つからず。[S10](#s10-ipv6) |
| D12 / 中 | 同一 NAT と接続候補の選択 | global **IP** 一致で localAddr を選び、firewalled 候補は除く。IP 0/loopback はアドレス候補から除く | global **endpoint (IP+port)** 一致を判定。global が null のとき local 扱い。`GetConnectableNodes` は ignore による除外 | `pickFromHitList` の段階選択と `ChanHitSearch` を使用 | スコアの係数が同じでも候補集合・接続アドレスは一致しない。mi コメントの「PCS と同じ NAT 判定」は正確でない。理由の記録なし。[S03](#s03-上流接続と再選択) |
| D13 / 高 | 音声のみの FLV の送出 | RTMP 音声を常に cont=0x04 とし、HTTP/PCP は初回 cont=0 を待って全 nonzero を捨てる | `FLVContentBuffer.OnContentChanged` は通常コンテンツを None で渡す。開始可能点がない場合の fallback もある | キーフレームを観測していない FLV には `m_streamHasKeyFrames` による cont 制約を掛けない | mi の AAC 音声のみではヘッダー後の本文が送られない経路になる。PCP 送出関数で audio 3 パケットがすべてスキップされることを再現。実エンコーダーでの E2E は未実施。対象外とする記録なし。[S05](#s05-コンテンツと送出) |
| D14 / 中 | リレーツリー API の情報量 | 自ノード・直下・上流 1 個を構築。直下の relays/directs=0、receiving=true 等は固定。上流 SID は空文字 | `GetChannelRelayTree` は保持した Host ツリーを API 化 | `HostGraph` からツリーを生成 | 接続一覧に近く、全段の正確なツリーとしては使えない。API 独自設計の方針はあるが、固定値・深さの判断理由は記録なし。D01/D06 とあわせて評価が必要。[S09](#s09-api) |

### D01: 配送経路の具体例

`配信元 A ← 中継 mi B ← 中継 C ← 視聴者` という構成で、C の `group=Trackers` の BCST HOST は B の `PCPOutputStream.forwardBcst` に到着する。B はローカルのノード表を更新するが、`Channel.Broadcast` は出力ストリームしか走査しないため A へ送らない。B 自身の上流 HOST は local 数のみを載せるので、C 配下の情報を代わりに集約して送る仕組みにもなっていない。

さらに mi の下流側 `forwardBcst` は自分宛の dest で payload 処理前に return し、別の宛先であっても HOST をローカル集計する。上流側 `handleBcst` は dest を見ずに CHAN/HOST を処理する。PCS は自分宛または dest なしの場合に payload を処理する。TTL だけの修正では解消しない差異である。

なお、配信名や track の更新は mi の `handleChan` → `SetInfo/SetTrack` → 出力通知で別途伝わる経路がある。「BCST を転送しない」から「すべてのメタデータ更新が止まる」とまではいえない。

### D02 / D06: 集計と広告を分けて考える

上流へ送る HOST の numl/numr が **local 数**なのは mi・PCS・yt で共通し、それ自体は不具合ではない。YP への tracker 広告では **total 数**を使う。問題は D01 により子孫の独立した HOST が上流へ届かない点と、D06 の失効規則の違いにある。

接続枠についても、`pcputil.BuildHostAtom` のフラグ生成コードが正しくても、relay 呼び出し元が満杯の値を渡さなければ常に空き扱いになる。全体帯域・全体接続数の制限は `Listener` の受付にはあり、YP と PCP 出力の広告ではその判定を共有していない。

### D04: 古いバックログと送信遅延は別

mi の `Timestamp` は `ContentBuffer.Write` で付く受信時刻。視聴者が今接続して送信キューが空でも、6 秒前のバッファを渡すと即 Overflow になる。PCS のキュー時刻は `ChannelMessage.Create*` で設定されるため、新しく投入したバックログが古いというだけでは同じ判定にならない。

また mi のバッファは bitrate と「1 packet 約 15 KB」から**パケット数**を計算する方式で、8 秒という設定が実際の保存秒数を保証するわけではない。PCS はコンテンツ timestamp の時間幅 (既定 3 秒) で管理する。初期送出位置、キーフレームの選択、バッファ保持方式、Overflow をまとめて判断する必要がある。

### D08: 引数の互換と操作の互換

`{"method":"bumpChannel","params":{"channelId":"…"},"id":1}` は mi でも受理される。しかし対象がリレーチャンネルでも `RelayClient` を再接続する呼び出しはなく、`YPClient.Bump` の先は配信チャンネルだけを再告知する。このため API 成功は、対象リレーの再接続を意味しない。

## 一致または部分的な整合を確認した点

| 項目 | 今回確認した範囲 | 留意点 |
|:--|:--|:--|
| PCP over HTTP | `/channel/<id>` の HTTP 応答に続いて helo/oleh。mi の relay は追加の `pcp\n` magic を送らない。yt の経路とも整合 | IPv6、期限、入力検証まで同一ではない |
| 満杯時の案内 | mi は 503 → helo/oleh → 最大 8 代替 HOST → QUIT+UNAVAILABLE。PCS の代替候補を返す方式に整合 | 退出条件・候補の鮮度は D06/D07。競合により handshake 後の最終受付で拒否される経路もある |
| 大きな PCP data | mi と PCS は 15 KB 分割、後続の Fragment を元のフラグに OR | yt は送信時に cont を真偽として出すため、PCS 拡張フラグの詳細が保存されるとは限らない |
| 位置の一周 | mi は uint32 の一周を差分比較し、PCS の下位 32 bit 送出と整合 | 再開時の初期バックログ選択が同じという意味ではない |
| 上流再選択の終了条件 | mi と PCS は Unavailable で次候補、非 tracker の失敗で次候補、tracker の失敗等で停止する | mi の handshake 無期限待ちが入ると、この判断へ到達しない |
| HTTP の info 待ち | mi と PCS は content type を最大 10 秒待って応答し、時間切れは 504 | yt は `getChannel` の playing 待ちで、準備完了条件は同じではない。mi の `/pls/` まで同じ待ちとはいえない |
| HTTP のヘッダー変更 | mi と PCS とも新ヘッダーを書き直して送出を継続する実装がある | ソース切断で channel 自体が消える場合とは区別する |
| リレー数・視聴者数 | relay HOST は local 数、YP tracker HOST は total 数という使い分けが mi・PCS・yt にある | D01/D06 による収集範囲・期限の差が残る |

## 再現チェックと検証

調査用の一時 Go テストを mi のパッケージ内に置き、以下の**現状の挙動**を確認した。これらは修正後に期待する挙動を検証する回帰テストではない。一時ファイルは検証後に削除し、製品実装・既存テストを変更していない。

| チェック | 入力 / 条件 | 観測した結果 |
|:--|:--|:--|
| `TestAuditOldBacklogQuit` | 新規 PCP 出力に、受信時刻が 6 秒前の cont=0 のパケットを渡す。net.Pipe の相手は即読取 | 最初に QUIT+SKIP、関数は overflow error。ネットワークの送信遅延なしでも D04 を確認 |
| `TestAuditAudioGate` | 初期キーフレーム待ちの PCP 出力へ、現在時刻の cont=4 を 3 個渡す | 送出位置は進むが待機状態は継続、本文バイトは 0。D13 の送出側を確認 |
| `TestAuditRelayHostAdvertisesSlots` | local relay/direct を各 1 個登録し、上限 1 に対する IsRelayFull/IsDirectFull が true の channel で relay HOST を構築 | Relay/Direct ビットが両方立つ。relay ビルダーに上限が渡らない D02 を確認 |

実行コマンド: `go test ./internal/servent ./internal/relay -run TestAudit -v`。3 チェックとも観測結果を確認できた。

一時テスト削除後の `go vet ./...` / `go test ./...` はともに成功した (既存テストはキャッシュ利用)。ビルドキャッシュ・ループバック通信のためサンドボックス外で実行した。文書の相対リンク先と `git diff --check` も確認した。既存テストが成功しても、今回の差異が解消されたことは意味しない。

## 根拠となるソース

### S01 識別子

- mi: [main.go](../../main.go) の SessionID/BroadcastID 生成、[id.go](../../internal/id/id.go) `ChannelID`、[manager.go](../../internal/channel/manager.go) `channelIDForBroadcast`。
- PCS: [BroadcastChannel.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/BroadcastChannel.cs) `CreateChannelID`、[AppBase.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.App/AppBase.cs) BroadcastID の設定読み書き。
- yt: [gnuid.cpp](../../../../peercast-yt/core/common/gnuid.cpp) `GnuID::encode`、[servhs.cpp](../../../../peercast-yt/core/common/servhs.cpp) `setBroadcastIdChannelId` / `createChannelInfo`、[servmgr.cpp](../../../../peercast-yt/core/common/servmgr.cpp) broadcastID の読み書き。

### S02 入力と GIV

- mi: [rtmp/server.go](../../internal/rtmp/server.go) `OnPublish` / `OnAudio` / `OnVideo` / `OnClose`、[listener.go](../../internal/servent/listener.go) プロトコル振り分け。
- PCS: [RTMPSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.FLV/RTMP/RTMPSourceStream.cs)、[HTTPPushSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.HTTP/HTTPPushSourceStream.cs)、[HTTPSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.HTTP/HTTPSourceStream.cs)。GIV は PCP ソース・出力と `*.cs` の GIV / PCP_PUSH 検索も確認。
- yt: [channel.cpp](../../../../peercast-yt/core/common/channel.cpp) `startURL` / `startHTTPPush` / `createSource`、[servent.cpp](../../../../peercast-yt/core/common/servent.cpp) `acceptGIV` / `givProc`。

### S03 上流接続と再選択

- mi: [relay/client.go](../../internal/relay/client.go) `Run` / `connectTo` / `handshake` / `processBody`、[select.go](../../internal/relay/select.go)、[sourcenode.go](../../internal/relay/sourcenode.go)、[findtracker.go](../../internal/relay/findtracker.go)。
- PCS: [PCPSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPSourceStream.cs) `PCPHandshakeTimeout` / `CreateHostUri` / `IsSiteLocal` / `GetConnectableNodes` / `SelectSourceHost` / `OnConnectionStopped`。
- yt: [channel.cpp](../../../../peercast-yt/core/common/channel.cpp) `connectFetch` / `handshakeFetch` / `PeercastSource::pickFromHitList` / `checkBump`。

### S04 BCST とノード表

- mi: [relay/client.go](../../internal/relay/client.go) `handleBcst`、[servent/pcp.go](../../internal/servent/pcp.go) `forwardBcst` / `extractNodeStats` / `runStreaming`、[channel.go](../../internal/channel/channel.go) `Broadcast` / `TotalListeners` / `TotalRelays`、[nodes.go](../../internal/channel/nodes.go)。
- PCS: [PCPSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPSourceStream.cs) `OnPCPBcst`、[PCPOutputStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPOutputStream.cs) `OnPCPBcst` / `OnPCPHost`、[Channel.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/Channel.cs) `Broadcast` / `HostsView` / `TotalDirects` / `TotalRelays`。
- yt: [pcp.cpp](../../../../peercast-yt/core/common/pcp.cpp) `readBroadcastAtoms`、[chanhit.cpp](../../../../peercast-yt/core/common/chanhit.cpp) `clearDeadHits`、[chanmgr.cpp](../../../../peercast-yt/core/common/chanmgr.cpp) `clearDeadHits`。

### S05 コンテンツと送出

- mi: [content.go](../../internal/channel/content.go) `Write` / `Since` / `SetHeader` / `ContentBufferSizeForBitrate`、[servent/pcp.go](../../internal/servent/pcp.go) `startCursor` / `streamLoop` / `sendDataPackets`、[http.go](../../internal/servent/http.go) `run`、[rtmp/server.go](../../internal/rtmp/server.go) `OnAudio`。
- PCS: [PCPOutputStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPOutputStream.cs) `ChannelMessage.Create*` / `Enqueue` / `SendRelayBody` / `CreateContentBodyPacket`、[Content.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/Content.cs) `PacketTimeLimit` / `GetFirstContent`、[Channel.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/Channel.cs) `AddContentSink`、[FLVContentBuffer.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.FLV/FLVContentBuffer.cs) `OnContentChanged`。
- yt: [servent.cpp](../../../../peercast-yt/core/common/servent.cpp) `handshakeStream` / `sendPCPChannel`、[flv.cpp](../../../../peercast-yt/core/common/flv.cpp) `FLVTagBuffer::sendImmediately`、[pcp.cpp](../../../../peercast-yt/core/common/pcp.cpp) continuation の読み取り。

### S06 HOST 広告と受付

- mi: [host.go](../../internal/pcputil/host.go) `BuildHostAtom`、[relay/client.go](../../internal/relay/client.go) `buildRelayBcstAtom`、[servent/pcp.go](../../internal/servent/pcp.go) `buildHostAtom`、[listener.go](../../internal/servent/listener.go) `canAdmitRelay` / `tryAdmit`、[channel.go](../../internal/channel/channel.go) `MakeRelayable` / `HasData`。
- PCS: [PCPSourceStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPSourceStream.cs) `CreatePCPHOST`、[Channel.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/Channel.cs) `MakeRelayable` / `IsRelayable`、[PCPOutputStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPOutputStream.cs) `SelectSourceHosts` / `SendHost`。
- yt: [cstream.cpp](../../../../peercast-yt/core/common/cstream.cpp) local HOST 更新、[chanhit.cpp](../../../../peercast-yt/core/common/chanhit.cpp) `initLocal` / `flags1` / `writeAtoms`、[channel.cpp](../../../../peercast-yt/core/common/channel.cpp) `canAddRelay`、[servent.cpp](../../../../peercast-yt/core/common/servent.cpp) `isTerminationCandidate`。

### S07 YP

- mi: [yp/client.go](../../internal/yp/client.go) `run` / `handleOleh` / `sendAllBcst` / `buildBcst`、[handler_channel.go](../../internal/jsonrpc/handler_channel.go) `stopChannel`。
- PCS: [PCPYellowPageClient.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPYellowPageClient.cs) `ProcessAtom` / `OnPCPOleh` / `UpdateTimeSpan` / `ChannelRemoved` / `PostHostInfo`。
- yt: [pcp.cpp](../../../../peercast-yt/core/common/pcp.cpp) `readRootAtoms`、[channel.cpp](../../../../peercast-yt/core/common/channel.cpp) `broadcastTrackerUpdate` / `totalListeners` / `totalRelays`。

### S08 HTTP 視聴

- mi: [listener.go](../../internal/servent/listener.go) `lookupChannel` / `parseViewerRequest`、[http.go](../../internal/servent/http.go) `run`。
- PCS: [HTTPOutputStream.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.HTTP/HTTPOutputStream.cs) `GetChannelAsync` / `StreamHandler`、[AppBase.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.App/AppBase.cs) 初期 listener 設定、[AuthMiddleware.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.Core/Http/AuthMiddleware.cs)。
- yt: [servhs.cpp](../../../../peercast-yt/core/common/servhs.cpp) `/pls/` / `/stream/`、[servmgr.cpp](../../../../peercast-yt/core/common/servmgr.cpp) `getChannel`、[servent.cpp](../../../../peercast-yt/core/common/servent.cpp) `sendRawMetaChannel` / `waitForChannelHeader`。

### S09 API

- mi: [server.go](../../internal/jsonrpc/server.go) `dispatch`、[handler_channel.go](../../internal/jsonrpc/handler_channel.go) `broadcastChannel` / `bumpChannelWithParams` / `bumpChannel`、[handler_relay.go](../../internal/jsonrpc/handler_relay.go) `getChannelRelayTree`。
- PCS: [APIHost.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.UI.HTTP/APIHost.cs) `BroadcastChannel` / `BumpChannel` / `GetChannelRelayTree`、[JSONRPC.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.UI.HTTP/JSONRPC.cs)。
- yt: [jrpc.cpp](../../../../peercast-yt/core/common/jrpc.cpp) `bumpChannel` / `getChannelRelayTree`、[jrpc.h](../../../../peercast-yt/core/common/jrpc.h) メソッド・引数名対応表。

### S10 IPv6

- mi: [sourcenode.go](../../internal/relay/sourcenode.go) IP の `GetInt`、[host.go](../../internal/pcputil/host.go) IP の `NewIntAtom`、[relay/client.go](../../internal/relay/client.go) `x-peercast-pcp: 1`。
- PCS: [PCPVersion.cs](../../../../peercaststation/PeerCastStation/PeerCastStation.PCP/PCPVersion.cs) `GetPCPVersionForNetworkType`。
- yt: [chanhit.cpp](../../../../peercast-yt/core/common/chanhit.cpp) `initLocal` の IPv6 分岐と `writeAtoms` の `writeAddress`。

## 未確認範囲と次の検証

相互接続での影響の大きさは、PCS/yt を配信元・中継・下流として入れ替えた試験が必要。特に 3 段以上の BCST、送信枠満杯時の接続先選択、長いバックログからの参加、音声のみの配信、沈黙する上流、YP 接続を保ったままの配信停止を分けて確認する。

RTMP の各コーデック・再接続競合、未知/不正アトム、全 HTTP ヘッダー・プレイリスト形式、全 JSON-RPC メソッドの値・エラー形式、IPv6 相互接続、yt の停止通知の全経路は未確認。未着手の扱いは [tasks.md](../tasks.md) で管理する。
