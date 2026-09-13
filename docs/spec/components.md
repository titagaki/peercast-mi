# peercast-mi コンポーネント仕様

[overview.md](overview.md) のシステム概要・ライフサイクル・並行処理設計を前提とする。

PeerCastStation との差異を埋めるために入れた動作の根拠 (rationale) については [decisions/peercaststation-compat.md](../decisions/peercaststation-compat.md) を参照。

---

## 4.1 RTMPServer

### 使用ライブラリ

`github.com/yutopp/go-rtmp` を使用する。`gortmp.NewServer` でサーバーを立て、`gortmp.Handler` インターフェースを実装して FLV タグを受け取る。

### 責務

- ポート 1935 で TCP 待ち受け
- RTMP ハンドシェイク処理 (go-rtmp に委譲)
- `OnPublish` でストリームキーを検証し、未発行キーの接続を拒否
- FLV タグ (Video / Audio / ScriptData) のストリームへの変換
- チャンネルメタデータ (`onMetaData`) からの `ChannelInfo` 更新

### ストリームキー認証

`OnPublish(cmd *message.NetStreamPublish)` コールバックで認証を行う。

```
cmd.PublishingName = "sk_a1b2c3..."  ← OBS の「ストリームキー」欄
```

- `Manager.IsIssuedKey(key)` が `false` → エラーを返して接続拒否
- `true` → 接続を受け付け、`handler.streamKey` に記録

`broadcastChannel` が呼ばれるまでの間、データは `Manager.GetByStreamKey()` が `nil` を返すため静かにドロップされる。

### FLV タグ形式

RTMP メッセージを FLV タグ形式に変換して扱う。

| フィールド | サイズ | 内容 |
|:---|:---|:---|
| TagType | 1 byte | 8=Audio / 9=Video / 18=ScriptData |
| DataSize | 3 bytes | ボディのバイト数 (big-endian) |
| Timestamp | 3 bytes | ミリ秒タイムスタンプ (big-endian 下位 24 bit) |
| TimestampExt | 1 byte | タイムスタンプ上位 8 bit |
| StreamID | 3 bytes | 常に `0x000000` |
| Data | N bytes | RTMP メッセージボディ |
| BackPointer | 4 bytes | タグ全体サイズ `11 + N` (big-endian) |

### シーケンスヘッダーの検出

RTMP メッセージボディの先頭バイトで判定する。

| タグタイプ | 条件 | 意味 |
|:---|:---|:---|
| Video (9) | `body[0] == 0x17 && body[1] == 0x00` | AVC sequence header (AVCDecoderConfigurationRecord) |
| Audio (8) | `body[0] == 0xAF && body[1] == 0x00` | AAC sequence header (AudioSpecificConfig) |
| ScriptData (18) | AMF0 文字列 = `"onMetaData"` | メタデータ |

- `0x17`: FrameType=1 (keyframe) + CodecID=7 (AVC/H.264)
- `0xAF`: SoundFormat=10 (AAC) + SoundRate=3 + SoundSize=1 + SoundType=1

### head パケットの組み立て

AVC または AAC シーケンスヘッダーが揃った時点で組み立て、`Channel.SetHeader()` を呼ぶ。

```
[FLV ファイルヘッダー 13 bytes]
  "FLV" + version(0x01) + flags(0x05: hasVideo|hasAudio) + dataOffset(9) + PreviousTagSize0(0)

[onMetaData タグ]            ← 存在する場合のみ。タイムスタンプは 0 に書き換え
[AVC sequence header タグ]   ← 存在する場合のみ。タイムスタンプは 0 に書き換え
[AAC sequence header タグ]   ← 存在する場合のみ。タイムスタンプは 0 に書き換え
```

### ストリーム位置

ヘッダーもデータも 1 つのバイト位置空間に置く (FLV ファイルと同じ並び。PeerCastStation / peercast-yt と同じ)。

- ヘッダーは `SetHeader(head, streamPos)` で現在位置に置き、`streamPos += len(head)` する。データはその末尾から続くので `Channel.ContentPosition()` と一致する
- 同じ RTMP セッション内で組み立て結果が前回と同一なら `SetHeader` を呼ばない (エンコーダーがシーケンスヘッダーを再送しても、バッファを消して視聴者をキーフレーム待ちにしない)
- RTMP セッションの最初のヘッダーは `streamPos = Channel.ContentPosition()` から始める。エンコーダーが再接続しても位置は 0 に巻き戻らず、同じヘッダーでも新しい位置で適用されて前セッションのデータが消える

各タグには 4 バイトの BackPointer (タグサイズ) を後置する。

シーケンスヘッダーが再送されたとき (エンコーダー再接続等) は head パケットを更新し、接続中の全出力ストリームに `NotifyHeader()` で通知する。

### RTMP → Channel マッピング

| RTMP メッセージ | 処理 |
|:---|:---|
| ScriptData `onMetaData` | AMF0 をデコードして `Bitrate`、`Type/MIMEType/Ext` を更新。head パケットに含める |
| Video `body[0]==0x17 && body[1]==0x00` | AVC sequence header として保存。head を再組み立て |
| Audio `body[0]==0xAF && body[1]==0x00` | AAC sequence header として保存。head を再組み立て |
| Video keyframe `body[0]==0x17` (上記以外) | `Channel.Write(data, pos, 0x00)` |
| Video inter frame `body[0]==0x27` | `Channel.Write(data, pos, 0x02)` |
| Audio (sequence header 以外) | `Channel.Write(data, pos, 0x04)` |

ContFlags は PeerCastStation 互換のビットフラグ:
- `0x00`: キーフレーム (None)
- `0x02`: 映像非キーフレーム (InterFrame)
- `0x04`: 音声パケット (AudioFrame)

キーフレーム (`ContFlags == 0x00`) を起点に新規視聴者へのストリーム配信を開始する。

---

## 4.2 ChannelInfo / TrackInfo

チャンネルのメタデータ。

```go
type ChannelInfo struct {
    Name     string // チャンネル名
    URL      string // コンタクト URL
    Desc     string // 説明
    Comment  string // コメント
    Genre    string // ジャンル
    Type     string // コンテンツタイプ ("FLV" など)
    MIMEType string // MIME タイプ ("video/x-flv" など)
    Ext      string // 拡張子 (".flv" など)
    Bitrate  uint32 // ビットレート (kbps)
}

type TrackInfo struct {
    Title   string
    Creator string
    URL     string
    Album   string
}
```

---

## 4.3 ContentBuffer

ストリームデータを保持するバッファ。

### データ構造

```go
type Content struct {
    Pos       uint32    // ストリーム内バイト位置
    Data      []byte
    ContFlags byte      // PeerCastStation 互換ビットフラグ (0x00=None, 0x01=Fragment, 0x02=InterFrame, 0x04=AudioFrame)
    Timestamp time.Time // バッファに書き込まれた時刻 (Overflow 検出・HTTP 出力の順序保証に使用)
}

type ContentBuffer struct {
    header    []byte                    // 最新のストリームヘッダー
    headerPos uint32                    // ヘッダーのストリーム位置
    packets   []Content                 // リングバッファ (サイズはビットレートから自動計算)
    count     int                       // 書き込み総数 (SetHeader で 0 にリセット)
    mu        sync.RWMutex
    sigCh     chan struct{}             // Write ごとに close して差し替える通知チャネル
}
```

### 設計方針

- 位置 (`Pos`, `headerPos`) は 2^32 で一周する (PCP の pkt.pos は 32 bit。PeerCastStation も送信時に `& 0xFFFFFFFF` する)。前後関係は `PosBefore(a, b)` (= `int32(a-b) < 0`、2^31 未満の距離を前方とみなす) で判定し、素の大小比較はしない
- `header`: `SetHeader` で上書き。新規接続時に必ず最初に送る
- `packets`: リングバッファ。サイズはビットレートと `content_buffer_seconds` 設定 (デフォルト 8 秒) から自動計算。最小 64 パケット。満杯時は最古から上書き
- 新規接続は `header` を送信後、`ContFlags` に `InterFrame (0x02)` が含まれない最初のパケット (= キーフレーム) 以降を送る

### インターフェース

```go
// SetHeader はストリームヘッダーを更新し、リングバッファをリセットする。
// 同一バイト列・同一 pos の再送は no-op (false を返す)。
func (b *ContentBuffer) SetHeader(data []byte, pos uint32) bool

// Write はデータパケットを追記し、Signal() 待ちの goroutine を起こす。
func (b *ContentBuffer) Write(data []byte, pos uint32, contFlags byte)

// Signal は次の Write で close されるチャネルを返す。
func (b *ContentBuffer) Signal() <-chan struct{}

// Header は最新のヘッダーとその位置を返す。
func (b *ContentBuffer) Header() (data []byte, pos uint32)

// OldestPos は最古パケットのストリーム位置を返す。バッファ空の場合は 0。
func (b *ContentBuffer) OldestPos() uint32

// NewestPos は最新パケットのストリーム位置を返す。バッファ空の場合は 0。
func (b *ContentBuffer) NewestPos() uint32

// PosBefore は一周を考慮して位置 a が b より前かを返す。
func PosBefore(a, b uint32) bool

// Since は指定位置以降のパケットを PosBefore の順序で返す。
// pos が古すぎる場合は最古パケットから返す。空の場合は nil。
func (b *ContentBuffer) Since(pos uint32) []Content

// PacketsAfter は ref より (Timestamp, Pos) 順で厳密に新しいパケットを返す (Pos の比較は PosBefore)。
// ref.Timestamp がゼロ値なら全パケットを返す。HTTPOutputStream が使用。
func (b *ContentBuffer) PacketsAfter(ref Content) []Content

// ContentPosition は最新コンテンツ末尾のバイト位置を返す (PeerCastStation の Channel.ContentPosition)。
// パケットがなければヘッダー末尾 (SetHeader でパケットは消えるので、あればそれが常に最新)。
// リレー再接続時の x-peercast-pos に使う。
func (b *ContentBuffer) ContentPosition() uint32

// HasData はバッファにパケットが 1 件以上あるか返す。
func (b *ContentBuffer) HasData() bool
```

---

## 4.4 Channel

```go
type OutputStreamType int

const (
    OutputStreamPCP  OutputStreamType = iota // PCPOutputStream (下流リレーノード)
    OutputStreamHTTP                         // HTTPOutputStream (視聴プレイヤー)
)

// OutputStream は PCPOutputStream と HTTPOutputStream の共通インターフェース。
type OutputStream interface {
    NotifyHeader() // ストリームヘッダー変化時に呼ばれる
    NotifyInfo()   // ChannelInfo 変化時に呼ばれる
    NotifyTrack()  // TrackInfo 変化時に呼ばれる
    Close()        // 接続を終了する
    Type() OutputStreamType // PCP / HTTP の種別
    ID() int                // Listener が払い出す接続ID
    RemoteAddr() string     // 接続元アドレス ("host:port")
    SendRate() int64        // 直近 1 秒間の送信バイト数
}

// BcstForwarder は PCP 出力ストリームのみが実装するインターフェース。
// bcst アトム転送とループ防止用のピア識別に使用する。
// HTTPOutputStream はこのインターフェースを実装しない。
type BcstForwarder interface {
    SendBcst(atom *pcp.Atom) // bcst アトムを下流に転送
    PeerID() pcp.GnuID      // リモートピアのセッション ID
}

// ConnectionInfo は接続中の出力ストリームのスナップショット。
type ConnectionInfo struct {
    ID         int
    Type       OutputStreamType
    RemoteAddr string
    SendRate   int64
}

type Channel struct {
    ID        pcp.GnuID
    buffer    *ContentBuffer // private: 委譲メソッド経由でアクセス
    StartTime time.Time

    mu             sync.RWMutex
    broadcastID    pcp.GnuID
    isBroadcasting bool   // true = RTMP ソース、false = リレー
    source         string // 表示用ソース文字列
    upstreamAddr   string // リレーチャンネルの上流 host:port
    info           ChannelInfo
    track          TrackInfo
    outputs        []OutputStream
    numListeners   int // HTTPOutputStream の数
    numRelays      int // PCPOutputStream の数

    // 上流ノード情報 (リレークライアントが oleh 受信時に設定。下流切断時に HOST として返す)
    upstreamSessionID pcp.GnuID
    upstreamIP        uint32
    upstreamPort      uint16

    // BCST HOST 経由で学習した他ノードの情報 (nodes.go)。独自のロックを持つ leaf。
    nodes nodeTable

    // MakeRelayable で退出させた下流ノードの IP → BAN 期限。BAN 中の IP からのリレー要求は即拒否
    banMu   sync.Mutex
    banList map[string]time.Time
}

// nodeTable (channel/nodes.go)
type nodeTable struct {
    mu         sync.RWMutex
    knownHosts []*pcp.Atom            // 観測した Host アトム (最大 32 件、session ID でデデュープ)。リレー満杯時の代替候補
    stats      map[pcp.GnuID]nodeStats // 下流ノードが報告した視聴者数・リレー数。TotalListeners / TotalRelays の合算に使う
}
```

`outputs` への追加・削除は `mu` で保護する。`AddOutput` / `RemoveOutput` 時に `numListeners` / `numRelays` を更新する。

`broadcastID` はブロードキャストチャンネルでは Manager の broadcastID を使用する。リレーチャンネルでは初期値ゼロで生成し、上流から受け取った `chan.bcid` で `SetBroadcastID()` により上書きされる。

`ContentBuffer` は private フィールド `buffer` として保持する。外部からは `Channel` の委譲メソッド (`HasData`, `Header`, `Signal`, `Since`, `OldestPos`, `NewestPos`, `Write`, `SetHeader`) 経由でアクセスする。

`Broadcast` メソッドは `OutputStream` を `BcstForwarder` に型アサーションし、bcst アトム転送を行う。`BcstForwarder` を実装しない HTTPOutputStream はスキップされる。同一ピア（同じ `PeerID()`）の別接続にも転送しない（ループ防止）。

`MakeRelayable` は `OutputStream` を `RelayEvictable` (`IsFirewalled()` / `Evict()`) に型アサーションし、firewalled な PCP 出力ストリームを 1 つ `Evict()` する。退出させたノードの IP は `relayBanDuration` (90 秒) の間 `HasBanned` が true になる。

`SelectSourceHosts` は `knownHosts` から requester の session ID を持つものを除き、次のスコアの降順に並べて最大 `max` 件を返す (`nodeTable.selectSourceHosts`)。Host アトムの最初の ip/port ペアを global endpoint、`flg1` の Relay ビットが落ちていれば relay full、`uphp` を hops として読む。

```
(global endpoint あり ? 16000 : 0) + (requesterIP と同じ IP ? 8000 : 0) +
(relay full でない ? 4000 : 0) + (Recv フラグあり ? 2000 : 0) +
max(10 - hops, 0) * 100 + numr * 10 + rand[0,1)
```

### メソッド

```go
func (c *Channel) BroadcastID() pcp.GnuID
func (c *Channel) SetBroadcastID(id pcp.GnuID)
func (c *Channel) IsBroadcasting() bool
func (c *Channel) Source() string
func (c *Channel) SetSource(s string)
func (c *Channel) UpstreamAddr() string
func (c *Channel) SetUpstreamAddr(addr string)
func (c *Channel) Info() ChannelInfo
func (c *Channel) Track() TrackInfo
func (c *Channel) SetInfo(info ChannelInfo)   // 更新後に全 outputs へ NotifyInfo()
func (c *Channel) SetTrack(track TrackInfo)   // 更新後に全 outputs へ NotifyTrack()
func (c *Channel) SetHeader(data []byte, pos uint32) // buffer 更新後に全 outputs へ NotifyHeader() (同一内容なら no-op)
func (c *Channel) Write(data []byte, pos uint32, contFlags byte)
func (c *Channel) UpstreamNodeInfo() (pcp.GnuID, uint32, uint16)
func (c *Channel) SetUpstreamNodeInfo(sessionID pcp.GnuID, ip uint32, port uint16)
// ContentBuffer 委譲メソッド
func (c *Channel) HasData() bool
func (c *Channel) Header() ([]byte, uint32)
func (c *Channel) Signal() <-chan struct{}
func (c *Channel) Since(pos uint32) []Content
func (c *Channel) PacketsAfter(ref Content) []Content
func (c *Channel) ContentPosition() uint32
func (c *Channel) OldestPos() uint32
func (c *Channel) NewestPos() uint32
// OutputStream 管理
func (c *Channel) AddOutput(o OutputStream)
func (c *Channel) TryAddOutput(o OutputStream, maxRelays, maxListeners int) bool
func (c *Channel) MakeRelayable(maxRelays int) bool // firewalled な下流を 1 つ Evict() して枠を空け、その IP を 90 秒 BAN する
func (c *Channel) Ban(key string, until time.Time)   // key (リモート IP) を until まで BAN
func (c *Channel) HasBanned(key string) bool         // BAN 中か (期限切れは削除して false)
func (c *Channel) RemoveOutput(o OutputStream)
func (c *Channel) NumListeners() int
func (c *Channel) NumRelays() int
func (c *Channel) TotalListeners() int         // 自ノード + 下流ノード報告値
func (c *Channel) TotalRelays() int
func (c *Channel) UpdateNodeStats(sessionID pcp.GnuID, listeners, relays int)
func (c *Channel) RemoveNodeStats(sessionID pcp.GnuID)
func (c *Channel) IsRelayFull(maxRelays int) bool
func (c *Channel) IsDirectFull(maxListeners int) bool
func (c *Channel) CloseAll()                  // 全接続に Close() を呼ぶ
func (c *Channel) UptimeSeconds() uint32
func (c *Channel) Connections() []ConnectionInfo
func (c *Channel) RelayNodes() []RelayNodeEntry // 下流 PCP ピアの一覧 (getChannelRelayTree 用)
func (c *Channel) CloseConnection(id int) bool
func (c *Channel) AddKnownHost(host *pcp.Atom)
func (c *Channel) SelectSourceHosts(max int, requester pcp.GnuID, requesterIP uint32) []*pcp.Atom // requester 自身を除きスコア順に最大 max 件
func (c *Channel) Broadcast(from OutputStream, atom *pcp.Atom) // BcstForwarder 型アサーションで転送
```

---

## 4.5 RelayClient (上流 PCP 接続)

上流 PeerCast ノードへ PCP でストリームを受信し、Channel に書き込む。

### tracker の探索 (FindTracker)

`/pls/` に tip がないとき、`relay.FindTracker(ctx, ypAddrs, channelID, sessionID, ourGlobalIP)` が YP に tracker を問い合わせる (PeerCastStation 互換: `PCPYellowPageClient.FindTracker`)。

```
FindTracker():
  ypAddrs (config の全 [[yp]]) を順に試し、最初に見つかった tracker の host:port を返す。全滅なら ErrTrackerNotFound
  YP 1 件あたり 10 秒 (findTrackerTimeout) で打ち切る

findTrackerAt(yp):
  1. YP に TCP 接続し、リレー要求と同じ GET /channel/<id> (x-peercast-pcp: 1) + helo を送る
  2. HTTP ステータスで分岐
     - 503: YP はリレーしないので、そのチャンネルについて知っている host アトム (tracker の bcst 由来) を oleh の後に送ってくる。
            quit まで読み、cid が一致し Tracker フラグの立った最初の host を採用する。
            接続先アドレスは selectSourceHost と同じ規則 (同一 NAT なら LocalAddr、それ以外は GlobalAddr)
     - 200: YP 自身がリレー可能なので YP を tracker とみなす
     - その他: 失敗
  3. quit を送って切断
```

見つかった tracker で `Manager.StartRelay` を呼ぶ。YP が返す他の host は初期候補として使わない (通常 tracker 自身の bcst しか YP に届いていないため。tracker 接続後の 503 応答や bcst で候補を学習する)。

### 接続フロー

`Run()` が接続先選択と再接続ループを回し、`connectTo()` が 1 接続分の TCP 接続〜切断を担当する。

```
Run():
  1. selectSourceHost() で接続先を決める
     (学習済み HOST 候補を PeerCastStation 互換のスコアで選択、なければ tracker)
  2. connectTo(addr) → 終了理由 (Error / Unavailable / OffAir) に応じて次を決める
     - Unavailable (quit 1003) → その host を 3 分無視して即座に次の候補へ
     - Error / OffAir (非 tracker) → その host を 3 分無視して即座に次の候補へ
     - Error / OffAir (tracker)   → Run 終了
     - 候補なし                   → Run 終了
  3. Run 終了時: Channel.CloseAll() → doneCh close → onStopped (Manager.Stop で削除)

connectTo():
  1. DialContext で TCP 接続 (10 秒タイムアウト、Stop で中断可能)
  2. handshake()
  3. HTTP 503 なら processHosts() で HOST / quit だけ受け取って戻る
  4. HTTP 200 なら bcstHostLoop goroutine を起動し processBody() で受信
  5. 切断時に quit(QUIT+SHUTDOWN) を best-effort で送信

handshake():
  1. HTTP GET /channel/<channelIdHex> HTTP/1.0 を送信
       x-peercast-pcp: 1
       x-peercast-pos: Channel.ContentPosition()   (途中から再開)
  2. helo アトム送信
       agnt = "PeerCast-MI/<version>"
       sid  = SessionID
       ver  = 1218
       port = listenPort
     ※ pcp\n magic は HTTP-upgraded /channel/ リクエストでは送信しない
  3. HTTP ステータス行 + ヘッダーを読む (200 / 503 以外はエラー)
  4. oleh アトム受信 (quit なら終了理由に変換)
     → oleh.sid と接続先 IP:port を Channel.SetUpstreamNodeInfo() に記録

processBody():
     - chan > pkt(type="head") → Channel.SetHeader()
     - chan > pkt(type="data") → Channel.Write()
     - chan > info / trck / bcid → Channel.SetInfo() / SetTrack() / SetBroadcastID()
     - host アトム、bcst 内の host → SourceNodeList に候補として蓄積
     - bcst 内の chan → 上と同じ処理
     - ok → 無視
     - quit アトム受信 → 終了理由に変換して戻る
     - 60 秒無音 → 読み取りタイムアウトでエラー終了

bcstHostLoop():
     - 接続直後、120 秒ごと、および視聴者数/リレー数が変化した時 (5 秒ごとに確認) に
       BCST(grp=TRACKERS) > HOST を上流に送る
```

### 再接続

バックオフはなく、上記のとおり候補ホストへ即時に接続し直す (PeerCastStation 互換。詳細は [decisions/peercaststation-compat.md](../decisions/peercaststation-compat.md))。`Stop()` が呼ばれると context をキャンセルし、接続中の Dial / 読み取りを中断して Run() を終了させる。

ホスト切り替えの間は Channel オブジェクトが維持されるため、下流の PCP リレー接続・HTTP 視聴接続は継続する。ただし、ヘッダーが再送されるまでの間は下流ノードは待機状態になる。Run() が終了 (候補枯渇・tracker 停止) した場合は全出力を閉じ、`onStopped` 経由で Manager から削除される。

### API

```go
func New(trackerAddr string, channelID, sessionID pcp.GnuID, listenPort uint16, ch *channel.Channel) *Client
func (c *Client) Run()                    // 再接続ループ。goroutine として呼ぶ
func (c *Client) Stop()                   // context をキャンセルし、Run() の終了を待つ
func (c *Client) SetGlobalIP(ip uint32)   // YP から取得した globalIP を設定 (接続先選択と BCST HOST に使う)
func (c *Client) SetOnStopped(fn func())  // Run() 終了後に呼ばれるフック (Run 開始前に設定)
```

---

## 4.6 YPClient (COUT 接続)

YP (root server) に PCP コントロール接続 (COUT) を確立し、チャンネル情報を定期ブロードキャストする。

### 接続フロー

```
1. TCP 接続 (YP のホスト:ポート)
2. "pcp\n" アトム + バージョン送信  ← pcp.Dial が自動処理
3. helo 送信
     agnt = "PeerCast-MI/<version>"
     ver  = 1218
     sid  = SessionID
     port = listenPort (peercast_port)
     ping = listenPort (YP からのファイアウォール疎通確認を受ける)
     bcid = BroadcastID
4. oleh 受信 → rip から globalIP を取得し OnGlobalIP を呼ぶ
5. root アトム受信 (任意)
     root.uint: 更新間隔 (秒)。受信した値で updateInterval を上書き
     root.upd : 即時更新要求
6. ok 受信 → ハンドシェイク完了
7. ブロードキャスト中のチャンネルが 1 つもなければ quit(QUIT+SHUTDOWN) を送って切断
   (リレー専用ノードは globalIP を知るためだけに接続する)
8. 初回 bcst 送信 (ブロードキャストチャンネルごとに 1 つ)
9. updateInterval ごとに bcst を繰り返し送信
   - YP からの root(upd) 受信時、および bumpChannel (Bump()) 時は即時送信
   - YP からの quit 受信・読み取りエラーで切断 → 再接続
10. 停止時: quit(QUIT+SHUTDOWN) 送信
```

接続の開始条件 (`shouldConnect`): チャンネルが 1 つ以上存在し、かつ globalIP 未取得またはブロードキャスト中のチャンネルがある場合。

### bcst の構造

```
bcst
  ttl  = 11
  hops = 0
  from = SessionID
  grp  = 0x01  (ROOT のみ)
  cid  = ChannelID
  vers = 1218
  vrvp = 27
  vexp = "MI"
  vexn = <バージョン番号>
  chan
    id   = ChannelID
    bcid = BroadcastID
    info
      name = ChannelInfo.Name
      url  = ChannelInfo.URL
      desc = ChannelInfo.Desc
      cmnt = ChannelInfo.Comment
      gnre = ChannelInfo.Genre
      type = ChannelInfo.Type
      bitr = ChannelInfo.Bitrate
    trck
      titl = TrackInfo.Title
      crea = TrackInfo.Creator
      url  = TrackInfo.URL
      albm = TrackInfo.Album
  host  (pcputil.BuildHostAtom で構築)
    id   = SessionID
    ip   = globalIP  (oleh.rip から取得)   ← 1 組目 (global)
    port = listenPort
    ip   = localIP   (YP 接続のローカル側) ← 2 組目 (LAN)
    port = listenPort
    numl = Channel.TotalListeners()
    numr = Channel.TotalRelays()
    uptm = 稼働秒数
    oldp = Channel.OldestPos()
    newp = Channel.NewestPos()
    cid  = ChannelID
    flg1 = TRACKER | CIN
           | RECV   (Channel.HasData() のとき)
           | RELAY  (IsRelayFull(max_relays) でないとき)
           | DIRECT (IsDirectFull(max_listeners) でないとき)
    ver  = 1218
    vevp = 27
    vexp = "MI"
    vexn = <バージョン番号>
    trkr = 1
    upip / uppt = 上流アドレス (リレーチャンネルのみ)
```

### 再接続

接続が切れた場合は指数バックオフ (初期 5 秒、最大 120 秒) で再接続する。

---

## 4.7 Listener

### 責務

ポート 7144 で TCP 待ち受け。先頭バイト列でプロトコルを識別し、適切な出力ストリームを生成する。

### プロトコル識別

先頭 64 バイトを peek して判定する (接続は消費しない。`GET /channel/<32hex>` を 1 回の peek で切り出すため)。

| 先頭バイト列 | 処理 |
|:---|:---|
| `"GET /channel/"` | PCPOutputStream を生成 (admission 判定 → handshake → streaming) |
| `"GET /stream/"` | 視聴要求 (下記) を解析し、HTTPOutputStream を生成 |
| `"GET /pls/"` | 視聴要求 (下記) を解析し、M3U プレイリストを返す |
| `"pcp\n"` (0x70 0x63 0x70 0x0a) | `handlePing()` — YP ファイアウォール疎通確認 |
| `"POST /api"` / `"OPTIONS /api"` | JSON-RPC API ハンドラーへ転送 (OPTIONS は CORS preflight) |
| その他 | 不明プロトコル → 切断 |

### 視聴要求 (`/pls/`, `/stream/`)

両ハンドラーは `parseViewerRequest` で HTTP リクエストを 1 回だけ読み、同じ規則で解析する (HTTPOutputStream はリクエストを読まない)。

```
パス:    <prefix><32 桁 hex の channelId>[.<拡張子>]
         例: /stream/<id>、/stream/<id>.flv、/pls/<id>
         拡張子は "." の後に "/" と "." を含まない 1 文字以上。それ以外の余分なパス → 400
クエリ:  ?tip=host:port  (省略可)
         net.SplitHostPort で分解でき、host が空でなく、port が 1..65535 なら受理。それ以外 → 400
         tip はそのまま relay client の接続先 (net.Dial) になる外部入力なので、登録済みチャンネルへの要求でも形式検証する

チャンネル解決 (lookupChannel):
  1. Manager.GetByID にあればそれを使う (tip は無視。既存の接続先は変えず、リレーも作り直さない)
  2. なければ送信元を確認する: relay_request_from = "private" (既定) のとき、ループバック・
     プライベート (RFC 1918, fc00::/7)・リンクローカル以外の送信元は 403 (リレーを開始しない)。
     "any" なら制限なし。登録済みチャンネルの視聴 (1.) には適用しない
  3. OnDemandRelay(channelID, tip) を呼ぶ (main.go → tip が空なら relay.FindTracker で YP に問い合わせ → Manager.StartRelay)
     生成と登録は Manager.StartRelay のロック下で 1 回だけ行われるので、同時要求でリレーは重複しない
     Manager.MaxRelayChannels (max_relay_channels) に達していれば ErrRelayChannelLimit → 503
     OnDemandRelay 未設定またはその他の失敗 (tracker 不明、接続先未指定で YP も知らない等) → 404
```

レスポンスは body を書き始める前に決まる HTTP/1.0 のステータス行のみ:

| 状況 | `/pls/` | `/stream/` |
|:---|:---|:---|
| パス・tip の形式不正 | 400 | 400 |
| チャンネル未登録で、送信元がリレー開始を許可されていない | 403 | 403 |
| チャンネル未登録でリレーを開始できない | 404 | 404 |
| チャンネル未登録で、リレーチャンネル数が上限 | 503 | 503 |
| `tryAdmit` 失敗 (視聴数・帯域上限) | — | 503 |
| ChannelInfo が 10 秒以内に届かない (4.9 手順 3) | — | 504 |
| 成功 | 200 + M3U | 200 + ストリーム (4.9) |

`/stream/` は自動リレー開始後もリダイレクトせず、同じ接続で 4.9 のフローに入る (リレー確立待ちは 4.9 の info 待ちと初回データ待機が担う)。

### 接続数制限 (admission)

`admitMu` で直列化し、以下を順に確認する。いずれかに引っかかれば拒否する。

1. `max_relays_total` (全チャンネル合計のリレー数)
2. `max_upstream_kbps` (全チャンネル合計の送信レート)
3. per-channel: `Channel.TryAddOutput(o, max_relays, max_listeners)`

PCP リレーは handshake 前に `canAdmitRelay` で判定して HTTP 200/503 を決め、handshake 後にもう一度 `tryAdmit` で確定する。`canAdmitRelay` は 1, 2 の後、接続元 IP が `Channel.HasBanned` なら拒否し、そうでなければ `Channel.MakeRelayable` で firewalled な下流の退出を試みる (退出させた下流の IP は 90 秒 BAN される)。

---

## 4.8 PCPOutputStream (PCP リレー)

下流 PeerCast ノードへ PCP でストリームを送信する。

### 接続受け付けフロー

`Listener.handlePCPRelay()` が全体フローを制御し、`handshake()` / `sendRelayDenied()` / `runStreaming()` (→ `sendInitial()` + `readLoop()` + `streamLoop()`) に分離されている。

```
Listener.handlePCPRelay():
  1. チャンネル未登録、またはデータ未受信 (HasData() == false) → HTTP 404 で切断
  2. canAdmitRelay() で受け入れ可否を先に判定 (HTTP 200 / 503 の決定に使う)
  3. handshake(admitted)
  4. 拒否 (503) の場合: sendRelayDenied() → 切断
       代替候補 host アトム最大 8 件 (SelectSourceHosts、要求元自身を除きスコア順) → quit(QUIT+UNAVAILABLE)
       (自ノードの host アトムは送らない)
  5. tryAdmit() で Channel に登録 (handshake 中に枠が埋まっていれば 4 と同じく拒否)
  6. runStreaming(startPos)

handshake(admitted):
  0. 全体に 18 秒のデッドラインを設定
  1. HTTP GET /channel/<channel-id> を受け取る
     (x-peercast-pcp: 1, x-peercast-pos: <位置> などのヘッダーを含む場合がある)
  2. HTTP/1.0 200 OK (admitted) または 503 Unavailable レスポンス送信
     Content-Type: application/x-peercast-pcp
     ※ PCP over HTTP では pcp\n マジックは送受信しない
  3. helo アトム受信・バリデーション
     - sid == 自分の SessionID → quit(QUIT+LOOPBACK)
     - sid == ゼロ             → quit(QUIT+NOTIDENTIFIED)
     - ver == 0 / ver < 1200  → quit(QUIT+BADAGENT)
  4. ping (ファイアウォール疎通確認) — helo に ping フィールドがあれば実行
     - 接続元がサイトローカル (10/8, 172.16/12, 192.168/16, 169.254/16) なら成功しても port = 0 扱い
     - ping がなく port フィールドがあればその値を信用する
  5. oleh アトム送信
       agnt = "PeerCast-MI/<version>"
       sid  = SessionID
       ver  = 1218
       rip  = 接続元の IPv4 アドレス (uint32, big-endian で組み立て)
       port = ping 成功時のポート番号 (0 = firewalled)
  6. admitted のときのみ ok(1) を送信

runStreaming():
  1. sendInitial()
  2. readLoop() goroutine を起動 (下流からの bcst/quit 読み取り)
  3. streamLoop(startPos)
  4. 終了時: 下流ノードの nodeStats を削除し接続を閉じる

sendInitial():
  1. chan アトム送信
       id   = ChannelID
       bcid = BroadcastID
       info / trck / pkt(type="head")
  2. host アトム送信 (pcputil.BuildHostAtom で構築)

streamLoop():
  0. startCursor(reqPos) で開始位置を決める (下記)
  1. drainNotifications() — 非ブロッキングで info/track/header/bcst 通知を処理
  2. sendDataPackets() — Channel.Since(pos) で未送信パケットを送信
     - 送信直前に headerCh を非ブロッキングで確認し、ヘッダー変更があれば先に送って位置を取り直す
       (SetHeader + Write の競合で新ストリームのデータを旧ヘッダーの後ろに流さない)
     - 最古の未送信パケットが 5 秒以上前のものなら Overflow とみなし quit(QUIT+SKIP) で切断
     - 15KB を超えるパケットは分割し、2 個目以降に Fragment (0x01) フラグを OR する
  3. 送信待ちがなければ Signal() / 通知チャネル / stallTimer を select
     - 5 秒以上データが来ない場合 → 接続を切断 (quit なし)
     - infoCh 通知時: chan > info を送信 (ブロードキャストチャンネルなら bcst でラップ)
     - trackCh 通知時: chan > trck を送信 (同上)
     - headerCh 通知時: chan > pkt(type=head) を送信し、送信位置を新ヘッダー位置に戻してキーフレーム待ちにする
       (バッファは SetHeader で消えており、位置が巻き戻っている可能性があるため。peercast-yt の streamIndex 変更時と同じ)
     - bcstCh: bcst アトムを下流に転送
     - closeCh (readLoop からの quit / Close()): 上流ノード情報を host アトムで送ってから quit(QUIT+SHUTDOWN)
     - closeCh (MakeRelayable からの Evict()): 代替候補 host アトム最大 8 件 (SelectSourceHosts) を送ってから quit(QUIT+UNAVAILABLE)
```

### x-peercast-pos による開始位置 (startCursor)

位置の前後判定は `channel.PosBefore` (2^32 の一周を考慮)。

- バッファが空: `reqPos` によらずヘッダー位置
- `reqPos == 0` (未指定): `OldestPos()` (= SetHeader 以降の全部。ヘッダー位置は使わない: 同じヘッダーを再送し続ける上流ではヘッダー位置がデータから際限なく離れるため)
- `reqPos` が `OldestPos()` より前 (バッファから溢れている): `OldestPos()`
- `reqPos` が `ContentPosition()` (最新パケット末尾) より後 (別の位置空間から再接続してきた等): `ContentPosition()`。バックログは送らず以降のデータだけ送る
- それ以外: `reqPos`

いずれの場合も最初のキーフレーム (`ContFlags == 0`) まではスキップする。

### data パケットの形式

```
chan
  id  = ChannelID
  pkt
    type = "data"
    pos  = <バイト位置>
    data = <FLV タグバイト列>
    cont = PeerCastStation 互換ビットフラグ (0x00=キーフレーム, 0x02=InterFrame, 0x04=AudioFrame)
```

### bcst 転送ルール

受信した `bcst` を転送する際のルール:

- `ttl` が 0 なら転送しない
- 転送時に `ttl -= 1`、`hops += 1`
- `from` が自分の SessionID なら転送しない (ループ防止)
- `dest` が自分の SessionID なら転送しない (宛先が自分)
- 転送先は同一チャンネルの他の PCP 出力ストリーム。送信元と同じ `PeerID()` を持つ接続は除く
- bcst 内に `host` があれば `Channel.AddKnownHost` (代替候補) と `UpdateNodeStats` (下流の視聴者/リレー数) に反映

---

## 4.9 HTTPOutputStream (HTTP 直接視聴)

メディアプレイヤーへ HTTP でストリームを送信する。

### フロー

```
1. Listener が HTTP GET /stream/<channel-id>[.ext][?tip=host:port] を解析し (4.7 視聴要求)、
   チャンネルを解決 (未登録なら自動リレー開始) して tryAdmit したうえで run() に入る
   (失敗時は Listener が 400/403/404/503 を返す)

2. 読み取り監視 goroutine を起動 (プレイヤー切断をデータ送信がない間も検知する)

3. ChannelInfo.Type が空なら infoCh の通知を最大 10 秒 (infoWaitTimeout) 待つ
   (リレー開始直後は上流の chan info アタムが届くまで info がない。データより先に届くので短い)
   時間切れ → 504 Gateway Timeout を返して切断 (まだ何も書いていないのでエラーを返せる)
   閉じられた → 何も返さず終了

4. HTTP/1.0 200 OK レスポンスを送信
   (リレー確立待ちの間にプレイヤーがタイムアウトしないよう、データ到着前に返す)
   Content-Type: <ChannelInfo.MIMEType> (デフォルト "video/x-flv")
   icy-name:    sanitizeHeaderValue(ChannelInfo.Name)
   icy-genre:   sanitizeHeaderValue(ChannelInfo.Genre)
   icy-url:     sanitizeHeaderValue(ChannelInfo.URL)
   icy-bitrate: <ChannelInfo.Bitrate>
   ※ sanitizeHeaderValue() で CR/LF を除去し HTTP ヘッダーインジェクションを防止

5. Channel.HasData() を確認
   データなし → Signal() で最大 30 秒 (firstDataTimeout) 待機、タイムアウトなら切断
   (200 送信済みなので別のエラーは書かない。リレーチャンネルは残り、視聴者ゼロなら Cleaner が後で削除する)

6. Channel.Header() を送信
   (待機中に届いた headerCh の通知はこの送信で消化済みなので捨てる。捨てないとヘッダーが 2 回送られる)

7. キーフレームを起点にストリームデータを連続送信
   - Channel.PacketsAfter(sent) で最後に送った Content より新しいものだけを取る
   - ContFlags != 0 のパケット (= 非キーフレーム) をスキップ
   - キーフレーム以降は全パケットを順次送信
   - 書き込みタイムアウト: 60 秒 (パケットごとに更新)
   - Channel.Signal() でデータ到着を待機
   - headerCh 通知時: 新ヘッダーを書き込み、sent と keyframe 待ち状態をリセットして継続
```

> **注意**: HTTPOutputStream は `BcstForwarder` インターフェースを実装しない。
> `infoCh` は info 待ち (手順 3) にだけ使い、配信中の info / `trackCh` の通知は無視する。
> ICY メタデータ (`icy-metaint`) は現在未実装。

---

## 4.10 JSON-RPC API (`internal/jsonrpc`)

`POST /api/1` で JSON-RPC 2.0 リクエストを受け付ける管理 API。
`Listener` が `POST /api` 接続を検出して `jsonrpc.Server.Handler()` に転送する。

`jsonrpc.Server` は具象型 `*channel.Manager` ではなく `ChannelManager` インターフェースに依存する。これによりテスト時のモックが容易になる。

```go
type ChannelManager interface {
    IssueStreamKey(accountName, streamKey string) error
    RevokeStreamKey(accountName string) bool
    ListStreamKeys() []channel.StreamKeyEntry
    Broadcast(streamKey string, info channel.ChannelInfo, track channel.TrackInfo) (*channel.Channel, error)
    Stop(channelID pcp.GnuID) bool
    GetByID(channelID pcp.GnuID) (*channel.Channel, bool)
    StreamKeyByID(channelID pcp.GnuID) (string, bool)
    List() []*channel.Channel
}
```

### アクセス制御

`isLocalhost()` でリモートアドレスを検査し、ループバックアドレス以外からのリクエストには Basic 認証 (`admin_user` / `admin_pass`) を要求する。ループバックからのリクエストは認証なしで通す。

### CORS

ループバックからの要求は無認証なので、ブラウザ上の任意のページが利用者のブラウザ経由で API を叩けないよう、`Origin` ヘッダー付きの要求はオリジンを検査する。

- `Origin` なし (curl 等・同一オリジン): CORS ヘッダーを付けずに処理
- ループバックのオリジン (`http://localhost:*`, `http://127.0.0.1:*`, `http://[::1]:*`): 許可 (Web UI の dev サーバー用)
- `allowed_origins` に列挙されたオリジン: 許可
- それ以外: `OPTIONS` / `POST` ともに 403 で拒否

許可したオリジンにはワイルドカードではなくそのオリジンを `Access-Control-Allow-Origin` に返し、`Vary: Origin` を付ける。

API の詳細仕様 (メソッド一覧・リクエスト/レスポンス形式・フィールド説明) は [api/jsonrpc.md](api/jsonrpc.md) を参照。
