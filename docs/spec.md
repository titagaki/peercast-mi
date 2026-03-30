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
    │ stream key で認証
    ▼
┌──────────────────────────────────────────────────────────┐
│ internal/rtmp — RTMPServer                               │
│  OnPublish でストリームキーを検証                         │
│  FLV タグを解析し Channel へ渡す                         │
└──────────────────┬───────────────────────────────────────┘
                   │ Channel.Write / SetHeader / SetInfo
                   ▼
┌──────────────────────────────────────────────────────────┐
│ internal/channel — Manager + Channel                     │
│  ┌─────────────────────────────────────────────────┐    │
│  │  Manager                                        │    │
│  │  streamKeys  map[string]struct{}  (発行済みキー) │    │
│  │  byID        map[GnuID]*Channel  (放送中チャンネル)│  │
│  │  byStreamKey map[string]*Channel               │    │
│  └──────────────────┬──────────────────────────────┘    │
│                     │ 0..N                               │
│                     ▼                                    │
│  ┌─────────────────────┐  ┌─────────────────────────┐   │
│  │  ContentBuffer      │  │  ChannelInfo / TrackInfo │   │
│  │  (head + data 64件) │  │  (メタデータ)             │   │
│  └─────────────────────┘  └─────────────────────────┘   │
└──┬────────────────────────────────────────────────────┬──┘
   │ fan-out (チャンネルID でルーティング)               │ 定期 bcst (全チャンネル)
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
       ├─ pcp\n             → handlePing() (YP ファイアウォール疎通確認)
       └─ POST /api/1       → JSON-RPC API (internal/jsonrpc)
```

### パッケージ構成

| パッケージ | 役割 |
|:---|:---|
| `internal/rtmp` | RTMPServer |
| `internal/channel` | Manager、Channel、ContentBuffer、ChannelInfo/TrackInfo |
| `internal/servent` | Listener、PCPOutputStream、HTTPOutputStream |
| `internal/yp` | YPClient |
| `internal/jsonrpc` | JSON-RPC API サーバー |
| `internal/id` | SessionID / BroadcastID / ChannelID 生成 |
| `internal/version` | バージョン定数 |
| `internal/config` | 設定ファイル (TOML) 読み込み |

---

## 3. 識別子

### 3.1 セッション ID (SessionID)

ノードの一意識別子。起動ごとにランダム生成する 16 バイトの GnuID。
PCP ハンドシェイクの `helo.sid` および `bcst.from`、`oleh.sid` として使用する。

### 3.2 ブロードキャスト ID (BroadcastID)

配信セッションの識別子。起動ごとにランダム生成する 16 バイトの GnuID。
`helo.bcid`、`bcst > chan.bcid` として使用する。

### 3.3 ストリームキー (StreamKey)

RTMP エンコーダーの認証に使用するトークン。`sk_` プレフィックスに 16 バイトのランダム hex を続けた形式 (`sk_a1b2c3...`)。

`issueStreamKey` で発行し、プロセスが終了するまで有効。チャンネルのライフサイクルに依存しない。

### 3.4 チャンネル ID (ChannelID)

チャンネルを識別する 16 バイトの GnuID。次の入力から決定論的に生成する:

```
input     = BroadcastID, Name + "\x00" + StreamKey, Genre, Bitrate
ChannelID = peercast-yt 互換 XOR アルゴリズム
```

同じパラメータで `broadcastChannel` を再呼び出しすると同じ ChannelID が返る。

> アルゴリズム詳細は [§10.1](#101-channelid-生成アルゴリズム-実装済み) を参照。

---

## 4. ライフサイクル

### 4.1 プロセス起動フロー

```
1. 設定ファイル (config.toml) を読み込む
2. SessionID / BroadcastID (ノードレベル) を生成
3. ChannelManager を生成
4. Listener を起動 (ポート 7144 待ち受け)
5. YPClient を起動 (COUT 接続・bcst ループ開始) ← YP が設定されている場合のみ
6. JSON-RPC API ハンドラーを Listener に登録
7. RTMP サーバーを起動 (ポート 1935 待ち受け)
```

### 4.2 チャンネル開始フロー (API 経由)

```
1. クライアントが issueStreamKey を呼ぶ → streamKey 発行
2. エンコーダーが rtmp://host:1935/live/<streamKey> に RTMP push
   ↳ RTMPServer.OnPublish でストリームキーを検証（未発行なら拒否）
3. クライアントが broadcastChannel を呼ぶ
   ↳ sourceUri からストリームキーを抽出
   ↳ Channel を生成、Manager に登録、channelId を返す
4. RTMP データが Channel.ContentBuffer に流れ始める
5. YPClient が次の bcst サイクルで新チャンネルを YP に通知
```

> エンコーダー接続 (手順 2) と broadcastChannel 呼び出し (手順 3) は順不同。
> broadcastChannel より先に RTMP が来た場合、データはチャンネルが作成されるまで静かにドロップされる。

### 4.3 チャンネル停止フロー

```
1. クライアントが stopChannel(channelId) を呼ぶ
2. Channel.CloseAll() → 全 PCPOutputStream に quit 送信、全接続を閉じる
3. Manager から Channel を削除
4. streamKey は残る（再度 broadcastChannel を呼べば同じ channelId で再開可能）
```

### 4.4 プロセス終了フロー

```
1. SIGINT / SIGTERM シグナル受信
2. RTMPServer.Close() → RTMP 受信停止
3. Listener.Close() → 新規接続受け付け停止
4. Manager.StopAll() → 全チャンネルの CloseAll()
5. YPClient.Stop() (defer) → quit(QUIT+SHUTDOWN) 送信 → YP 接続を閉じる
```

### 4.5 RTMP 再接続時

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

## 10. 設計決定メモ

peercast-yt (`_ref/peercast-yt/core/common/`) のコードリーディングに基づく設計決定の記録。

---

### 10.1 ChannelID 生成アルゴリズム (実装済み)

peercast-yt (`gnuid.cpp:24`, `servhs.cpp:2440`) と互換の XOR ベースアルゴリズムを採用。
`internal/id/id.go` の `ChannelID(broadcastID, name, genre, bitrate)` として実装。

- 入力: `BroadcastID`, `Name`, `Genre`, `Bitrate(uint8)`
- SHA512+MD5 方式は非互換のため不採用
- peercast-yt の `randomizeBroadcastingChannelID` フラグは peercast-mm では実装しない

---

### 10.2 `x-peercast-pos` による途中参加 (実装済み)

`PCPOutputStream.handshake()` で `x-peercast-pos` ヘッダーを読み取り、`streamLoop()` の初期位置として使用。
- `reqPos` がバッファ範囲内: その位置から開始
- `reqPos` が古すぎる: `OldestPos()` から開始
- `reqPos == 0`: ヘッダー位置から開始

---

### 10.3 push 接続 (firewalled ノード向け)

peercast-yt の GIV プロトコルは「firewalled リレーノードへのアウトバウンド接続」であり、**対象外**。
peercast-mm はブロードキャストノード専用で、下流ノードは常に peercast-mm 側に接続してくる。

---

### 10.4 `helo.ping` によるファイアウォール疎通確認 (実装済み)

- `yp/client.go` の `buildHelo()` に `ping = listenPort` を含める
- `servent/listener.go` の `handlePing()` で `pcp\n` + helo を受け取り、oleh + quit を返す
- ファイアウォール状態管理 (FW_UNKNOWN/FW_OFF/FW_ON) および `flg1` フラグ更新は将来課題

---

### 10.5 リスナー数・リレー数の正確なカウント (実装済み)

`Channel` が `numListeners` / `numRelays` カウンタを保持し、`AddOutput` / `RemoveOutput` 時に更新。
`buildBcst()` では `ch.NumListeners()` / `ch.NumRelays()` の実際の値を使用。
