# peercast-mi 実装仕様

Go 製 PeerCast ノードの実装仕様。ブロードキャストノード（RTMP → PCP 配信）とリレーノード（上流 PCP ノードから受け取って中継）の両方に対応する。

コンポーネント別の詳細仕様は [components.md](components.md) を参照。JSON-RPC API は [api/jsonrpc.md](api/jsonrpc.md) を参照。

---

## 1. スコープ

### 対象

- RTMP サーバー (エンコーダーからの push 受信)
- PCP ブロードキャストノード (non-root、`IsBroadcasting = true`)
  - YP への COUT 接続・チャンネル登録
  - 下流 PeerCast ノードへの PCP リレー送信
  - 視聴クライアントへの HTTP 直接送信
- PCP リレーノード (上流 PeerCast ノードからストリームを受け取って中継)
  - 上流ノードへの PCP 接続・ストリーム受信
  - 受信したストリームを下流ノード・視聴クライアントに配信

### 対象外

- Web UI
- push 接続 (firewalled ノード向け)

---

## 2. システム構成

```
エンコーダー                       上流 PeerCast ノード
    │ RTMP push (ポート 1935)          │ PCP (ポート 7144)
    │ stream key で認証                │ GET /channel/<id>
    ▼                                  ▼
┌──────────────┐          ┌───────────────────────┐
│ internal/rtmp│          │ internal/relay         │
│ RTMPServer   │          │ RelayClient            │
│ FLV タグ変換 │          │ 上流接続・ストリーム受信│
└──────┬───────┘          └───────────┬───────────┘
       │ Write/SetHeader/SetInfo       │ Write/SetHeader/SetInfo
       ▼                               ▼
┌──────────────────────────────────────────────────────────┐
│ internal/channel — Manager + Channel                     │
│  ┌────────────────────────────────────────────────────┐  │
│  │  Manager                                           │  │
│  │  keys          *StreamKeyStore     (ストリームキー) │  │
│  │  byID          map[GnuID]*Channel  (全アクティブch) │  │
│  │  byStreamKey   map[string]*Channel (broadcast のみ) │  │
│  │  streamKeyByID map[GnuID]string    (逆引き)       │  │
│  │  relays        map[GnuID]RelayHandle (relay のみ)  │  │
│  └──────────────────┬─────────────────────────────────┘  │
│                     │ 0..N                                │
│                     ▼                                     │
│  ┌─────────────────────┐  ┌─────────────────────────┐    │
│  │  ContentBuffer      │  │  ChannelInfo / TrackInfo │    │
│  │  (head + data ring) │  │  (メタデータ)             │    │
│  └─────────────────────┘  └─────────────────────────┘    │
└──┬────────────────────────────────────────────────────┬───┘
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
       ├─ GET /pls/<id>     → M3U プレイリスト (未登録ならオンデマンドリレー開始。?tip= がなければ YP に tracker を問い合わせる)
       ├─ pcp\n             → handlePing() (YP ファイアウォール疎通確認)
       └─ POST /api/1       → JSON-RPC API (internal/jsonrpc)
```

### パッケージ構成

| パッケージ | 役割 |
|:---|:---|
| `internal/rtmp` | RTMPServer |
| `internal/relay` | RelayClient (上流 PCP 接続・ストリーム受信) |
| `internal/channel` | Manager、StreamKeyStore、Channel、ContentBuffer、nodeTable、Cleaner、ChannelInfo/TrackInfo |
| `internal/servent` | Listener、PCPOutputStream、HTTPOutputStream |
| `internal/yp` | YPClient |
| `internal/jsonrpc` | JSON-RPC API サーバー (ChannelManager インターフェース経由で Manager に依存) |
| `internal/pcputil` | PCP Host アトム構築ユーティリティ (BuildHostAtom) |
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

RTMP エンコーダーの認証に使用するトークン。値は `issueStreamKey` の呼び出し側 (pcgw-0yp 等) が生成して渡す任意の文字列で、peercast-mi 側で形式は検証しない (慣例として `sk_` + ランダム hex)。

`issueStreamKey` で発行すると `stream_keys.json` (config.toml と同じディレクトリ) に永続化され、`revokeStreamKey` で失効させるまでプロセス再起動をまたいで有効。チャンネルのライフサイクルに依存しない。

### 3.4 チャンネル ID (ChannelID)

チャンネルを識別する 16 バイトの GnuID。次の入力から決定論的に生成する:

```
input     = BroadcastID, Name + "\x00" + StreamKey, Genre, Bitrate
ChannelID = peercast-yt 互換 XOR アルゴリズム
```

同じパラメータで `broadcastChannel` を再呼び出しすると同じ ChannelID が返る。

> アルゴリズム選定の経緯は [decisions/0001-channel-id-algorithm.md](../decisions/0001-channel-id-algorithm.md) を参照。

---

## 4. ライフサイクル

### 4.1 プロセス起動フロー

```
1. 設定ファイル (config.toml) を読み込む
2. シグナルハンドラ (SIGINT/SIGTERM) を設定
3. SessionID / BroadcastID (ノードレベル) を生成
4. ChannelManager を生成し、stream_keys.json からストリームキーを復元
   ↳ RelayClient のコンストラクタを Manager.NewRelay に注入する (channel → relay の依存を避けるため)
5. Listener を起動 (ポート 7144 待ち受け)
6. YPClient を起動 (COUT 接続・bcst ループ開始) ← YP が設定されている場合のみ
   ↳ oleh で得た globalIP を Listener と Manager (→ 全 RelayClient) に伝播する (OnGlobalIP)
7. Listener にオンデマンドリレー (`/pls/<id>[?tip=host:port]`) のフック (→ tip がなければ relay.FindTracker で全 `[[yp]]` に問い合わせ → Manager.StartRelay) を登録
8. JSON-RPC API ハンドラーを Listener に登録
9. Cleaner を起動 (channel_cleanup_minutes > 0 の場合)
10. RTMP サーバーを起動 (ポート 1935 待ち受け)
11. シャットダウンシグナルを待機
```

### 4.2 ブロードキャストチャンネル開始フロー (API 経由)

```
1. クライアントが issueStreamKey を呼ぶ → streamKey 発行
2. エンコーダーが rtmp://host:1935/live/<streamKey> に RTMP push
   ↳ RTMPServer.OnPublish でストリームキーを検証（未発行なら拒否）
3. クライアントが broadcastChannel を呼ぶ
   ↳ streamKey パラメータからストリームキーを取得
   ↳ Channel (IsBroadcasting=true) を生成、Manager に登録、channelId を返す
4. RTMP データが Channel.ContentBuffer に流れ始める
5. YPClient が次の bcst サイクルで新チャンネルを YP に通知
```

> エンコーダー接続 (手順 2) と broadcastChannel 呼び出し (手順 3) は順不同。
> broadcastChannel より先に RTMP が来た場合、データはチャンネルが作成されるまで静かにドロップされる。

### 4.3 リレーチャンネル開始フロー (オンデマンド)

```
1. プレイヤーが /pls/<channelId>[?tip=<host:port>] にアクセス
   (/stream/ や /channel/ への直接アクセスでは自動リレーは開始せず 404 を返す)
2. Listener がチャンネル未登録を検出し、OnDemandRelay フック → Manager.StartRelay() を呼ぶ
   ↳ tip がなければ先に relay.FindTracker() で config の全 [[yp]] に順に問い合わせ、tracker の host:port を得る
     (-yp で選んだ YP に限らない)。どの YP も知らなければ 404
3. Manager が Channel (IsBroadcasting=false) を生成して登録し、NewRelay で RelayClient を生成
   ↳ 既知の globalIP を渡し、停止時に自分を Manager から削除するフック (SetOnStopped) を登録
4. RelayClient をゴルーチンとして Run() で起動
5. RelayClient が tip (tracker) に TCP 接続
6. HTTP GET /channel/<channelId> (x-peercast-pcp, x-peercast-pos) + helo を送信
   ※ pcp\n magic は HTTP アップグレード接続では送らない
7. 上流から HTTP 200 + oleh + 初期 chan アトム (info/track/header) を受信
   (HTTP 503 の場合は HOST アトムで代替候補だけ受け取り、別ノードに接続し直す)
8. Channel に info/track/header をセット
9. 上流からの pkt アトムを Channel.ContentBuffer に継続的に書き込む
10. YPClient は globalIP 未取得なら一度 YP に接続して oleh から取得し、切断する
    (リレーチャンネルは YP に bcst しない)
```

> 再接続は PeerCastStation と同じ方式 (バックオフなし): 上流から受け取った HOST アトムを
> スコアリングして次の候補に即時接続し、失敗したホストは 3 分間無視する。
> tracker 自身の失敗・OffAir、または候補が尽きた時点で Run() は終了し、
> Channel の全出力を閉じて Manager から削除する (SetOnStopped → Manager.Stop)。
> ホスト切り替えの間は Channel と下流接続を維持する。

### 4.4 チャンネル停止フロー

```
1. クライアントが stopChannel(channelId) を呼ぶ
2. リレーチャンネルの場合: RelayClient.Stop() → 上流接続を閉じ、再接続ループを終了
3. Channel.CloseAll() → 全 PCPOutputStream に quit 送信、全接続を閉じる
4. Manager から Channel を削除
5. ブロードキャストチャンネルの場合: streamKey は残る（再度 broadcastChannel で再開可能）
```

### 4.5 プロセス終了フロー

```
1. SIGINT / SIGTERM シグナル受信
2. RTMPServer.Close() → RTMP 受信停止
3. Listener.Close() → 新規接続受け付け停止
4. Manager.StopAll() → 全 RelayClient.Stop() + 全チャンネルの CloseAll()
5. Cleaner.Stop() (defer)
6. YPClient.Stop() (defer) → quit(QUIT+SHUTDOWN) 送信 → YP 接続を閉じる
```

### 4.6 RTMP 再接続時

エンコーダーが再接続した場合:

1. 切断時に `RTMPServer.OnClose` → `Manager.Stop(channelID)` でチャンネルは一度削除される
   (ストリームキーは残るので `broadcastChannel` を再呼び出しすれば同じ channelId で再開できる)
2. 新しい RTMP 接続の `handler` は `streamPos = 0` から数え直す
3. 新しいシーケンスヘッダーで `Channel.SetHeader()` が呼ばれ、リングバッファはリセットされる
4. 既存の PCPOutputStream には `chan > pkt(type=head)` が再送され、HTTPOutputStream には新ヘッダーが書き込まれる

---

## 5. 並行処理設計

### Goroutine 一覧

| goroutine | 役割 |
|:---|:---|
| `RTMPServer` accept | TCP accept ループ |
| `RTMPServer` per-conn | 接続ごと: RTMP 受信・デコード |
| `relay.Client.Run` | リレーチャンネルごと: 上流接続・ストリーム受信ループ (再接続含む) |
| `relay.Client.bcstHostLoop` | 上流接続ごと: BCST HOST を定期送信 (120 秒 / 接続数変化時) |
| `YPClient.Run` | COUT 接続・bcst 送信ループ |
| `YPClient.run` reader | YP からの root(update)/quit を読む |
| `Cleaner.Run` | 5 秒ごとにアイドルなリレーチャンネルを削除 |
| `Listener` accept | TCP accept ループ |
| `PCPOutputStream.runStreaming` | 接続ごと: PCP 送信ループ |
| `PCPOutputStream.readLoop` | 接続ごと: 下流からの bcst/quit 読み取り |
| `HTTPOutputStream.run` | 接続ごと: HTTP 送信ループ |
| `HTTPOutputStream` read monitor | 接続ごと: プレイヤー切断検知 |

### データ共有

```
RTMPServer ──(SetHeader/Write)──→ ContentBuffer ←──(Since/Header)── 出力 goroutine 群
```

- `ContentBuffer` の読み書きは `sync.RWMutex` で保護する
- 出力 goroutine はポーリングせず、`Channel.Signal()` が返すチャネル (Write ごとに close される) で待機する
  - PCPOutputStream は自前の `pos uint32` を持ち `Channel.Since(pos)` で未送信分を取る
  - HTTPOutputStream は最後に送った `Content` を持ち `Channel.PacketsAfter(sent)` で (Timestamp, Pos) 順に取る
  （`ContentBuffer` は `Channel` の private フィールドで、委譲メソッド経由でアクセスする）

### 通知チャネル

ヘッダー・メタデータ更新は各出力 goroutine の専用チャネルで通知する。

```go
// servent/output_base.go — PCPOutputStream / HTTPOutputStream に埋め込まれる
type outputBase struct {
    headerCh chan struct{} // SetHeader 時に通知
    infoCh   chan struct{} // SetInfo 時に通知
    trackCh  chan struct{} // SetTrack 時に通知
    closeCh  chan struct{} // 終了通知 (Close で close される)
}
```

各チャネルはバッファサイズ 1 でノンブロッキング送信 (`select { case ch <- struct{}{}: default: }`)。
これにより通知の取りこぼし防止と goroutine ブロック回避を両立する。

---

## 6. エラー処理

| 状況 | 対応 |
|:---|:---|
| RTMP 接続切断 | `Manager.Stop(channelID)` でチャンネルを削除し、出力接続を全て閉じる。YP には次の bcst から載らなくなる (quit は送らない) |
| 上流 PCP 接続切断 | 候補ホストに即時再接続 (バックオフなし)。Channel・下流接続は維持。候補が尽きるか tracker が落ちたら停止・削除 |
| YP 接続切断 | 指数バックオフ (5〜120 秒) で再接続。出力接続は維持 |
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
| `DefaultContentBufferSize` | 64 | コンテンツバッファの最小パケット数 |
| `DefaultContentBufferSeconds` | 8.0 | バッファが保持するデフォルト秒数 |
| `bcstTTL` | 11 | bcst TTL (YP / リレー / 下流通知で共通) |
| `retryInitial` | 5s | YP 再接続初期待機時間 |
| `retryMax` | 120s | YP 再接続最大待機時間 |
| `defaultInterval` | 120s | YP bcst 送信間隔 (root.uint で上書きされる) |
| `outputQueueTimeout` | 5s | PCP 出力キュー詰まり / Overflow 検出時間 |
| `pcpWriteTimeout` | 10s | PCP アトム 1 回の書き込みタイムアウト |
| `pcpHandshakeTimeout` | 18s | PCP ハンドシェイク全体のタイムアウト |
| `maxContentBodyLen` | 15KB | これを超える data パケットは分割送信 |
| `directWriteTimeout` | 60s | HTTP 直接送信タイムアウト |
| `dialTimeout` (relay) | 10s | 上流への TCP 接続タイムアウト |
| `readTimeout` (relay) | 60s | 上流からのアトム読み取りタイムアウト |
| `bcstInterval` (relay) | 120s | 上流への BCST HOST 送信間隔 |
| `nodeExpiry` / `ignoredDuration` | 3m | 上流候補の有効期限 / 失敗ホストの無視期間 |
| `cleanerInterval` | 5s | Cleaner の巡回間隔 |
| `maxKnownHosts` | 32 | bcst 経由で蓄積する代替ホストの上限 |
| `relayBanDuration` | 90s | MakeRelayable で退出させた下流ノードの IP をリレー受付で拒否する期間 |

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

> **注意:** `relay.Client` は `pcp.Dial` を使わず `net.Dialer.DialContext` で TCP 接続したあと
> 手動で HTTP GET + helo を送信する (pcp\n magic は送らない)。HTTP アップグレード型の
> `/channel/` 接続では相手側も magic を期待しないため (peercast-yt: "don't need PCP_CONNECT here")。

---

## 9. 依存ライブラリ

| ライブラリ | 用途 |
|:---|:---|
| `github.com/titagaki/peercast-pcp` | PCP プロトコル層 |
| `github.com/yutopp/go-rtmp` | RTMP サーバー |
| `github.com/yutopp/go-amf0` | AMF0 デコード (onMetaData) |


---

## 10. 設計判断の記録

設計上の判断とその根拠は [docs/decisions/](../decisions/README.md) に記録する。
この仕様書は「現在どう動くか」だけを書き、「なぜそうしたか」は decisions 側を参照すること。

