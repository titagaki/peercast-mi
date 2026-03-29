# peercast-mm 実装仕様

RTMP で受け取ったストリームを PCP ネットワークに配信する、Go 製 PeerCast ブロードキャストノードの実装仕様。

コンポーネント別の詳細仕様は [spec-components.md](spec-components.md) を参照。

---

## 1. スコープ

### 対象

- RTMP サーバー (エンコーダーからの push 受信)
- PCP ブロードキャストノード (non-root、IsBroadcasting = true)
  - YP への COUT 接続・チャンネル登録
  - 下流 PeerCast ノードへの PCP リレー送信
  - 視聴クライアントへの HTTP 直接送信

### 対象外

- リレーノード (上流から受け取って中継する機能)
- Web UI
- push 接続 (firewalled ノード向け)

---

## 2. システム構成

```
エンコーダー
    │ RTMP push (ポート 1935)
    ▼
┌──────────────────────────────────────────────────────────┐
│ internal/rtmp — RTMPServer                               │
│  FLV タグを解析し Channel へ渡す                         │
└──────────────────┬───────────────────────────────────────┘
                   │ Channel.Write / SetHeader / SetInfo
                   ▼
┌──────────────────────────────────────────────────────────┐
│ internal/channel — Channel                               │
│  ┌─────────────────────┐  ┌─────────────────────────┐   │
│  │  ContentBuffer      │  │  ChannelInfo / TrackInfo │   │
│  │  (head + data 64件) │  │  (メタデータ)             │   │
│  └─────────────────────┘  └─────────────────────────┘   │
└──┬────────────────────────────────────────────────────┬──┘
   │ fan-out                                            │ 定期 bcst
   ▼                                                    ▼
┌────────────────────┐                     ┌──────────────────────┐
│ internal/servent   │                     │ internal/yp          │
│  Listener          │                     │  YPClient            │
│  ポート 7144       │                     │  (COUT 接続)          │
│  プロトコル識別    │                     │  YP に bcst 送信      │
└──────┬─────────────┘                     └──────────────────────┘
       │
       ├─ GET /channel/<id> → PCPOutputStream  × N  (下流リレーノード)
       ├─ GET /stream/<id>  → HTTPOutputStream × N  (視聴プレイヤー)
       └─ pcp\n             → (将来: PING 応答等)
```

### パッケージ構成

| パッケージ | 役割 |
|:---|:---|
| `internal/rtmp` | RTMPServer |
| `internal/channel` | Channel、ContentBuffer、ChannelInfo/TrackInfo |
| `internal/servent` | Listener、PCPOutputStream、HTTPOutputStream |
| `internal/yp` | YPClient |
| `internal/id` | SessionID / BroadcastID / ChannelID 生成 |
| `internal/version` | バージョン定数 |
| `internal/config` | CLI フラグ・設定 |

---

## 3. 識別子

### 3.1 セッション ID (SessionID)

ノードの一意識別子。起動ごとにランダム生成する 16 バイトの GnuID。
PCP ハンドシェイクの `helo.sid` および `bcst.from`、`oleh.sid` として使用する。

### 3.2 ブロードキャスト ID (BroadcastID)

配信セッションの識別子。起動ごとにランダム生成する 16 バイトの GnuID。
`helo.bcid`、`bcst > chan.bcid` として使用する。

### 3.3 チャンネル ID (ChannelID)

チャンネルを識別する 16 バイトの GnuID。次の入力から決定論的に生成する:

```
input     = BroadcastID || NetworkType || ChannelName || Genre || SourceURL
ChannelID = MD5(SHA512(input))[:16]
```

NetworkType は固定値 `"ipv4"` とする。

> **未解決**: 既存実装 (peercast-yt 等) との互換性は未確認。→ [§10](#10-未解決事項)

---

## 4. ライフサイクル

### 4.1 配信開始フロー

```
1. エンコーダーが RTMP 接続を確立
2. onMetaData から ChannelInfo を構築 → Channel.SetInfo()
3. Channel を生成 (ChannelID 生成、ContentBuffer 初期化)
4. YPClient を起動 (COUT 接続・bcst ループ開始)
5. Listener を起動 (ポート 7144 待ち受け)
6. RTMP データを ContentBuffer に流し始める
```

### 4.2 配信終了フロー

```
1. RTMP 接続が切れる (または手動停止)
2. YPClient: quit 送信 → 接続を閉じる
3. 既存の PCPOutputStream: quit 送信 → 接続を閉じる
4. 既存の HTTPOutputStream: 接続を閉じる
5. Listener: 停止
6. Channel を破棄
```

### 4.3 RTMP 再接続時

エンコーダーが再接続した場合:

1. `Channel.SetHeader()` を再呼び出し (新しいシーケンスヘッダー)
2. 既存の PCPOutputStream に `chan > pkt(type=head)` を再送
3. ストリームバイト位置は継続 (リセットしない)

---

## 5. 並行処理設計

### Goroutine 一覧

| goroutine | 役割 |
|:---|:---|
| `RTMPServer` accept | TCP accept ループ |
| `RTMPServer` per-conn | 接続ごと: RTMP 受信・デコード |
| `YPClient.Run` | COUT 接続・bcst 送信ループ |
| `Listener` accept | TCP accept ループ |
| `PCPOutputStream.run` | 接続ごと: PCP 送信ループ |
| `HTTPOutputStream.run` | 接続ごと: HTTP 送信ループ |

### データ共有

```
RTMPServer ──(SetHeader/Write)──→ ContentBuffer ←──(Since/Header)── 出力 goroutine 群
```

- `ContentBuffer` の読み書きは `sync.RWMutex` で保護する
- 出力 goroutine は自前の `pos uint32` を持ち、`Buffer.Since(pos)` でポーリングする

### 通知チャネル

ヘッダー・メタデータ更新は各出力 goroutine の専用チャネルで通知する。

```go
type PCPOutputStream struct {
    headerCh chan struct{} // SetHeader 時に通知
    infoCh   chan struct{} // SetInfo 時に通知
    trackCh  chan struct{} // SetTrack 時に通知
    closeCh  chan struct{} // 終了通知
}
```

各チャネルはバッファサイズ 1 でノンブロッキング送信 (`select { case ch <- struct{}{}: default: }`)。
これにより通知の取りこぼし防止と goroutine ブロック回避を両立する。

---

## 6. エラー処理

| 状況 | 対応 |
|:---|:---|
| RTMP 接続切断 | ストリーム停止。YP へ quit 送信。出力接続を全て閉じる |
| YP 接続切断 | 指数バックオフで再接続。出力接続は維持 |
| 出力ストリーム切断 | 該当接続のみ終了。他は継続 |
| 出力キュー詰まり (5 秒) | 該当接続を強制切断 |
| helo バリデーション失敗 | quit atom を送信して接続を閉じる |

---

## 7. 定数

| 定数 | 値 | 説明 |
|:---|:---|:---|
| `defaultPCPPort` | 7144 | PCP リスニングポート |
| `defaultRTMPPort` | 1935 | RTMP リスニングポート |
| `PCPVersion` | 1218 | PCP プロトコルバージョン |
| `PCPVersionVP` | 27 | VP 拡張バージョン |
| `ContentBufferSize` | 64 | コンテンツバッファのパケット数 |
| `bcstTTL` | 7 | bcst TTL |
| `retryInitial` | 5s | YP 再接続初期待機時間 |
| `retryMax` | 120s | YP 再接続最大待機時間 |
| `defaultInterval` | 120s | YP bcst 送信間隔 (root.uint で上書きされる) |
| `outputQueueTimeout` | 5s | PCP 出力キュー詰まり検出時間 |
| `directWriteTimeout` | 60s | HTTP 直接送信タイムアウト |
| `httpPollInterval` | 200ms | HTTP 出力ポーリング間隔 |
| `pollInterval` | 50ms | PCP 出力ポーリング間隔 |

---

## 8. peercast-pcp ライブラリ対応表

`github.com/titagaki/peercast-pcp/pcp` が提供する API とこの実装での使用箇所。

| API | 使用箇所 |
|:---|:---|
| `pcp.Dial` | `YPClient.run` — YP への COUT 接続 |
| `pcp.ReadAtom` / `Atom.Write` | 全 PCP 通信 |
| `pcp.NewParentAtom` / `pcp.New*Atom` | 全アトム構築 |
| `pcp.GnuID` | SessionID / BroadcastID / ChannelID |
| `pcp.PCPHostFlags1*` | `flg1` ビット定数 |
| `pcp.PCPBcstGroup*` | `grp` ビット定数 |
| `pcp.PCPError*` | quit アトムのエラーコード |

---

## 9. 依存ライブラリ

| ライブラリ | 用途 |
|:---|:---|
| `github.com/titagaki/peercast-pcp` | PCP プロトコル層 |
| `github.com/yutopp/go-rtmp` | RTMP サーバー |
| `github.com/yutopp/go-amf0` | AMF0 デコード (onMetaData) |

---

## 10. 未解決事項

- [ ] ChannelID 生成アルゴリズムの既存実装 (peercast-yt 等) との互換性確認
- [ ] `x-peercast-pos` による途中参加時のストリーム位置決定ロジック
- [ ] push 接続 (firewalled ノード向け) の対応範囲
- [ ] `helo.ping` によるファイアウォール疎通確認の実装
- [ ] リスナー数・リレー数の正確なカウント (`numl` / `numr`)
