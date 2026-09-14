# 3 実装比較に基づく修正・検証結果

2026-09-14。[修正前比較](2026-09-14-implementation-comparison.md) の後、ユーザーから「理由のなさそうな差異は修正する」と依頼されて実装した。参照コミットと比較対象範囲は修正前比較に記載したまま。mi は同じ HEAD 上の未コミット変更、参照リポジトリは変更していない。

I01–I08 の意図的な設計は維持する。以下の「修正」は対象経路のコード修正を意味し、実機相互接続確認済み・完全互換を意味しない。今回新たに判断した理由と却下案は [ADR 0018](../decisions/0018-comparison-corrections.md)、現在の動作は [仕様](../spec/components.md) と [API](../spec/api/jsonrpc.md) に記録した。

## 差異への対応

| ID | 対応 | 変更内容 / 残す理由 | 主なコード |
|:--|:--|:--|:--|
| D01 | 修正 | group に従う上下流転送、dest のローカル配送、TTL/hops、上流 writer 直列化 | `pcputil.RouteBcst`, `Channel.Broadcast`, relay writer |
| D02 | 修正 | 全体リレー数・送信帯域・チャンネル別上限を HOST と API に反映 | `Channel.SlotStatus`, `Manager.configureChannel` |
| D03 | 修正 | family 別 IP/port 状態、未知/閉鎖時 PUSH + global port 0。Receiving は直近 30 秒の data、切断で即 false | `pcputil.NetworkState`, `Channel.IsReceiving` |
| D04 | 一部修正・一部維持 | 初期バックログの年齢による即 Overflow と 5 秒の無通信切断を除去。最古からの開始・パケット数バッファ・32 bit 位置は ADR 0002/0015 の既存契約を維持 | `servent.streamLoop`, `sendDataPackets` |
| D05 | 修正 | TCP 接続後の handshake 全体に 18 秒期限、bump でも中断可能 | `relay.Client.handshake`, `Reconnect` |
| D06 | 修正 | HOST と統計の 180 秒失効、直下切断時に経由した子孫も除去 | `channel/nodes.go`, `observed_hosts.go` |
| D07 | 修正 | 非 local で firewalled、または relay full かつ下流 0 を退出。全体上限でも対象チャンネルで退出を試す。90 秒 BAN は維持 | `Channel.makeRelayable`, `Listener.canAdmitRelay` |
| D08 | 修正 | リレーは対象上流の再接続要求。外部 RTMP 配信はエンコーダーを切断せず、従来の YP 再通知を維持 | `Manager.Reconnect`, `relay.Client.Reconnect`, `bumpChannel` |
| D09 | 修正 | BroadcastID を `broadcast_id` に保存。競合作成でも同じ値、不正値は上書きせず起動エラー | `id.LoadOrCreateBroadcastID`, `main.go` |
| D10 | 修正 | YP 既定 30 秒、接続後 root.uint 反映、変更検出 1 秒、削除時 RECV=false の最終通知 | `yp/client.go`, `announcements.go` |
| D11 | 対応追加 | 逆順 16 bytes IP、version 100、IPv6 endpoint、oleh.rip。実機相互接続は未確認 | `pcputil/network.go`, HOST/relay/YP/ping/config |
| D12 | 理由を確認して維持 | 同じ NAT 内では公開ポートが異なるため、外部 IPv4 の一致で LAN 接続を選ぶ。PCS の IP+port 一致にはしない。GIV・unspecified/loopback 候補の除外も維持。「PCS と同じ」という旧コメントは訂正 | `relay/select.go`、今回の判断は ADR 0018 |
| D13 | 修正 | FLV media bit を実入力に合わせ、音声のみなら完全な AudioFrame から HTTP/PCP 送出を開始 | `rtmp.rebuildHeader`, `Channel.CanStartContent` |
| D14 | 修正 | 新鮮な HOST の人数・状態・親 endpoint を使った子孫ツリー。未知を true で補わず、循環は有限に処理 | `jsonrpc.getChannelRelayTree` |

追加修正: 最小 PCP ping が 64 bytes 未満でも識別できるようにした。YP 直結 magic は protocol version を含む INT にした。上流 HOST は人数だけでなく Receiving・空き枠・疎通状態の変化も 5 秒周期で検出する。

## 修正後の検証

| 回帰テスト | 確認内容 |
|:--|:--|
| `TestCompatBcstRouting`, `TestCompatBcstDirections` | 宛先・TTL/hops・group・上下流配送・元アトム非破壊 |
| `TestCompatGlobalSlots` | 別チャンネルのリレーが全体枠を埋めたときの判定、呼び出し側上限 |
| `TestCompatIPWireAndReachability` | IPv4/IPv6 IP アトム往復、IPv6 バイト順、port/PUSH と疎通状態 |
| `TestCompatReceivingAndAudioBoundaries` | stale buffer と Receiving の分離、切断、音声/映像/Fragment 境界 |
| `TestCompatOldBacklogAndAudio`, `TestCompatActualQueueOverflow` | 6 秒前の初期バックログ・音声が送出され、実際のキュー遅延では QUIT |
| `TestCompatIdleDoesNotDisconnect` | 5.2 秒の data 停止後も同じ PCP 接続で送出再開 |
| `TestCompatHandshakeTimeoutAndBump` | 沈黙する loopback TCP 上流への期限と、接続試行だけの中断 |
| `TestCompatHostExpiryAndSubtreeRemoval` | 180 秒超の HOST/統計失効、直下切断で子孫削除 |
| `TestCompatEvictionLocalAndUnproductive` | local 保護、未報告の開放ノード保護、満杯かつ下流 0 の退出・即時枠解放・BAN |
| `TestPersistentBroadcastID` | 再読み込み、8 並行作成、0600、不正値を上書きしない |
| `TestCompatAnnouncementChangesAndRemoval` | YP 初期通知、変更なしの抑制、メタデータ変更、削除の RECV=false |
| `TestCompatIPv6SourceNode` | IPv6 global/local 候補の抽出 |
| `TestCompatMinimalPingThroughListener` | IPv4 loopback で Listener 経由の最小 PCP ping |
| `TestCompatBumpRelay` | RPC が対象リレーの再接続を要求 |
| `TestCompatRebuildHeaderMediaFlags` | AVC/AAC の有無に応じた FLV media bit |
| `TestCompatRelayTreeDescendantsAndCycles` | 子孫・HOST 値・循環の有限処理 |

`go vet ./...`、`go test ./... -timeout 60s`、`go test ./... -race -count=1 -timeout 90s` は成功。キャッシュなしの競合検出でも全パッケージ成功。ビルドキャッシュと loopback 通信のためサンドボックス外で実行した。修正前比較の一時再現テストとは別に、上記は恒久的な回帰テストとして追加した。

実機の PCS/yt/mi 3 段接続、IPv6 多段・dual stack、全コーデック、実エンコーダーとプレイヤーによる再生確認は未実施。BCST のテストは部品の配送規則を検証するもので、3 プロセス相互接続の代わりではない。未確認項目は [tasks.md](../tasks.md) で管理する。

## 運用上の注意

`broadcast_id` 導入時は以前のプロセス内ランダム ID を復元できないため、一度 ChannelID が変わる。その後は同ファイルと `stream_keys.json` を保管すれば同じ入力で同じ ChannelID を維持できる。初回作成には config ディレクトリの書き込み権限が必要。別実装の ChannelID への移行互換は保証しない。
