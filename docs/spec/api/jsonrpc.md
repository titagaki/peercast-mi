# JSON-RPC API 仕様

peercast-mi が提供する JSON-RPC 2.0 API の実装仕様。

実装: `internal/jsonrpc/server.go`

---

## エンドポイント

```
POST /api/1
```

ポート 7144 で受け付ける。PCP・HTTP 視聴リクエストと同居している。

### リクエスト形式

JSON-RPC 2.0 仕様に準拠する。パラメータは原則として **位置指定配列** (`"params": [...]`) のみ対応。
例外として `bumpChannel` は名前指定オブジェクト (`"params": {"channelId": ...}`) も受け付ける（PeerCastStation / peca-live 互換のため）。

```json
{
  "jsonrpc": "2.0",
  "method": "<メソッド名>",
  "params": [...],
  "id": 1
}
```

### レスポンス形式

成功時:
```json
{ "jsonrpc": "2.0", "id": 1, "result": <値> }
```

エラー時:
```json
{ "jsonrpc": "2.0", "id": 1, "error": { "code": <コード>, "message": "<説明>" } }
```

### エラーコード

| コード | 意味 |
|---|---|
| `-32700` | JSON パースエラー |
| `-32601` | メソッドが存在しない |
| `-32602` | パラメータが不正 |
| `-32603` | 内部エラー（チャンネル未発見等） |

---

## 実装メソッド一覧

| メソッド | パラメータ | 返却値 |
|---|---|---|
| `issueStreamKey` | `[accountName, streamKey]` | `null` |
| `revokeStreamKey` | `[accountName]` | `null` |
| `listStreamKeys` | なし | `[{ accountName, streamKey }, ...]` |
| `broadcastChannel` | `[{ streamKey, info, track }]` | `{ channelId }` |
| `getVersionInfo` | なし | `{ agentName }` |
| `getSettings` | なし | `{ serverPort, rtmpPort }` |
| `getChannels` | なし | チャンネルオブジェクトの配列 |
| `getChannelInfo` | `[channelId]` | `{ info, track }` |
| `getChannelStatus` | `[channelId]` | status オブジェクト |
| `setChannelInfo` | `[channelId, info, track]` | `null` |
| `stopChannel` | `[channelId]` | `null` |
| `bumpChannel` | `[channelId]` または `{channelId}` | `null` |
| `getChannelConnections` | `[channelId]` | 接続情報の配列 |
| `stopChannelConnection` | `[channelId, connectionId]` | `boolean` |
| `getYellowPages` | なし | YP オブジェクトの配列 |
| `updateYPChannels` | なし | YP 掲載番組の配列（自ノード一覧とは別） |
| `getChannelRelayTree` | `[channelId]` | ノードオブジェクトの配列 |

`channelId` は 32 文字の hex 文字列（大文字・小文字どちらも可）。

---

## 各メソッドの仕様

### `issueStreamKey`

**パラメータ:** `[accountName: string, streamKey: string]`

アカウントに対してストリームキーを登録する。pcgw-0yp がキーを生成して呼び出す想定。
同じ `accountName` で再呼び出しすると旧キーが無効化され新キーに差し替えられる（再発行）。
登録内容はキャッシュファイル (`stream_keys.json`) に永続化される。

他のアカウントが使用しているキーの登録は `-32603` (`stream key already assigned`)。保存エラーも `-32603` となり、以前のマッピングを維持する。サイトの所有者名 `site:x:<X ID>` も同じキー保存に含まれる。サイト利用者にはこの管理 API を公開しない。

**返却値:** `null`

**エラー条件:**
- `accountName` または `streamKey` が空 → `-32602`
- キャッシュファイルへの書き込み失敗 → `-32603`

---

### `revokeStreamKey`

**パラメータ:** `[accountName: string]`

アカウントのストリームキーを無効化する。そのキーで放送中のチャンネルは停止しない。

**返却値:** `null`

**エラー条件:**
- `accountName` が未登録 → `-32603`

---

### `listStreamKeys`

**パラメータ:** なし

発行済みのストリームキー一覧を `accountName` の昇順で返す。

**返却値:**
```json
[
  { "accountName": "alice", "streamKey": "sk_a1b2c3..." },
  { "accountName": "bob",   "streamKey": "sk_d4e5f6..." }
]
```

---

### `broadcastChannel`

**パラメータ:** `[{ streamKey, info, track }]`

指定したストリームキーでチャンネルを開始する。

```json
[{
  "streamKey": "sk_a1b2c3d4e5f6...",
  "info": {
    "name":    "チャンネル名",
    "genre":   "ジャンル",
    "url":     "https://example.com",
    "desc":    "説明",
    "comment": "",
    "bitrate": 3000
  },
  "track": {
    "title":   "",
    "creator": "",
    "album":   "",
    "url":     ""
  }
}]
```

| フィールド | 説明 |
|---|---|
| `streamKey` | `issueStreamKey` で登録済みのストリームキー (**必須**、空文字列不可) |
| `info.name` | チャンネル名 (**必須**、空文字列不可) |
| `info.bitrate` | ビットレート (kbps)。`0` でもよい (RTMP の onMetaData で上書きされる) |

**返却値:**
```json
{ "channelId": "0123456789abcdef0123456789abcdef" }
```

ChannelID は入力パラメータから決定論的に生成される。同じ `streamKey` と `info` で再呼び出しすると同じ `channelId` が返る。

**エラー条件:**
- `info.name` が空 → `-32602`
- `streamKey` が空 → `-32602`
- ストリームキーが未登録 (`issueStreamKey` で登録していない) → `-32602`
- そのストリームキーで既に放送中 (`stopChannel` してから再呼び出しする) → `-32602`

---

### `getVersionInfo`

**パラメータ:** なし

**返却値:**
```json
{ "agentName": "PeerCast-MI/0.0.1" }
```

---

### `getSettings`

**パラメータ:** なし

**返却値:**
```json
{ "serverPort": 7144, "rtmpPort": 1935 }
```

config.toml の `peercast_port` / `rtmp_port` の値を返す。

---

### `getChannels`

**パラメータ:** なし

**返却値:** チャンネルオブジェクトの配列。`broadcastChannel` で開始中のチャンネルが全て含まれる。

```json
[
  {
    "channelId": "0123456789abcdef0123456789abcdef",
    "status": {
      "status": "Receiving",
      "source": "rtmp://127.0.0.1:1935/live/sk_a1b2c3...",
      "uptime": 120,
      "localRelays": 2,
      "localDirects": 1,
      "totalRelays": 2,
      "totalDirects": 1,
      "isBroadcasting": true,
      "isRelayFull": false,
      "isDirectFull": false,
      "isReceiving": true
    },
    "info": {
      "name": "チャンネル名",
      "url": "https://example.com",
      "genre": "ジャンル",
      "desc": "説明",
      "comment": "",
      "bitrate": 500,
      "contentType": "FLV",
      "mimeType": "video/x-flv"
    },
    "track": {
      "title": "",
      "genre": "",
      "album": "",
      "creator": "",
      "url": ""
    }
  }
]
```

---

### `getChannelInfo`

**パラメータ:** `[channelId: string]`

**返却値:**
```json
{
  "info": {
    "name": "チャンネル名",
    "url": "https://example.com",
    "genre": "ジャンル",
    "desc": "説明",
    "comment": "",
    "bitrate": 500,
    "contentType": "FLV",
    "mimeType": "video/x-flv"
  },
  "track": {
    "title": "",
    "genre": "",
    "album": "",
    "creator": "",
    "url": ""
  }
}

---

### `getChannelStatus`

**パラメータ:** `[channelId: string]`

**返却値:**
```json
{
  "status": "Receiving",
  "source": "rtmp://127.0.0.1:1935/live/sk_a1b2c3...",
  "uptime": 120,
  "localRelays": 2,
  "localDirects": 1,
  "totalRelays": 2,
  "totalDirects": 1,
  "isBroadcasting": true,
  "isRelayFull": false,
  "isDirectFull": false,
  "isReceiving": true
}
```

| フィールド | 説明 |
|---|---|
| `status` | `"Receiving"` または `"Idle"`。`Channel.IsReceiving()` に基づく |
| `source` | ブロードキャストチャンネル: `rtmp://127.0.0.1:<rtmpPort>/live/<streamKey>`。リレーチャンネル: 上流ノードの `host:port` |
| `uptime` | チャンネル開始からの経過秒数（`Channel.UptimeSeconds()`） |
| `localRelays` | 自ノードの PCP リレー接続数（`Channel.NumRelays()`） |
| `localDirects` | 自ノードの HTTP 直接視聴接続数（`Channel.NumListeners()`） |
| `totalRelays` | `localRelays` + 下流ノードが BCST HOST で報告したリレー数の合計（`Channel.TotalRelays()`） |
| `totalDirects` | `localDirects` + 下流ノードが BCST HOST で報告した視聴者数の合計（`Channel.TotalListeners()`） |
| `isBroadcasting` | ブロードキャストチャンネル (RTMP ソース) なら `true`、リレーチャンネルなら `false` |
| `isRelayFull` | チャンネル別リレー数・全体リレー数・全体送信帯域のいずれかが上限なら `true` |
| `isDirectFull` | チャンネル別視聴数または全体送信帯域が上限なら `true` |
| `isReceiving` | 最後の data 受信から 30 秒未満なら `true`。ソース切断・チャンネル停止で即 false。残存バッファとは独立 |

---

### `setChannelInfo`

**パラメータ:** `[channelId: string, info: object, track: object]`

`info` / `track` の構造は `getChannelInfo` の返却値と同じ。

`info.bitrate` が `0` の場合は現在値を維持する（`type` フィールドは書き込み不可）。

**返却値:** `null`

---

### `stopChannel`

**パラメータ:** `[channelId: string]`

全出力接続を即時切断する（`Channel.CloseAll()`）。RTMP ソース接続は切断しない。

**返却値:** `null`

---

### `bumpChannel`

**パラメータ:** 次のどちらの形式でも指定できる。

- 位置指定配列: `[channelId: string]`
- 名前指定オブジェクト: `{ "channelId": string }` （PeerCastStation 互換。peca-live はこの形式を使用する）

```json
{ "jsonrpc": "2.0", "id": 1, "method": "bumpChannel", "params": ["0123456789abcdef0123456789abcdef"] }
```

```json
{ "jsonrpc": "2.0", "id": 1, "method": "bumpChannel", "params": { "channelId": "0123456789abcdef0123456789abcdef" } }
```

どちらの形式でも同じチャンネルが対象となり、処理内容は同一。
リレーチャンネルの場合、対象の上流接続試行を中断して再選択・再接続を要求する。チャンネルと下流接続は維持する。`null` は要求受理を意味し、再接続の成立までは待たない。再接続可能なソースがなければ `-32603` を返す。

ブロードキャストチャンネルの場合は YP への bcst を即時送信する (`YPClient.Bump()`)。送信対象はブロードキャスト中の全チャンネル。YP 未設定の場合は no-op。外部エンコーダーの RTMP 接続は再起動しない。

**返却値:** `null`

**エラー条件:**
- `channelId` が指定されていない、または文字列以外 → `-32602`
- 該当チャンネルが存在しない → `-32603`

---

### `getChannelConnections`

**パラメータ:** `[channelId: string]`

**返却値:** 接続情報オブジェクトの配列。先頭要素が常にソース接続（`connectionId: -1`）。

```json
[
  {
    "connectionId": -1,
    "type": "source",
    "status": "Receiving",
    "sendRate": 0,
    "recvRate": 0,
    "protocolName": "RTMP",
    "remoteEndPoint": "127.0.0.1:1935"
  },
  {
    "connectionId": 3,
    "type": "relay",
    "status": "Connected",
    "sendRate": 65000,
    "recvRate": 0,
    "protocolName": "PCP",
    "remoteEndPoint": "203.0.113.5:7144"
  },
  {
    "connectionId": 5,
    "type": "direct",
    "status": "Connected",
    "sendRate": 65000,
    "recvRate": 0,
    "protocolName": "HTTP",
    "remoteEndPoint": "198.51.100.9:54321"
  }
]
```

| フィールド | 説明 |
|---|---|
| `connectionId` | 接続 ID。ソースは常に `-1`、出力接続は `Listener` が採番した正の整数 |
| `type` | `"source"` / `"relay"` / `"direct"` |
| `status` | ソースは `"Receiving"` または `"Idle"`（`IsReceiving()` に基づく）、出力接続は `"Connected"`（固定値） |
| `sendRate` | bytes/sec（`OutputStream.SendRate()`）。ソースは常に `0` |
| `recvRate` | bytes/sec。現実装では常に `0` |
| `protocolName` | ブロードキャストチャンネルのソースは `"RTMP"`、リレーチャンネルのソースは `"PCP"`、下流 PCP リレーは `"PCP"`、HTTP 直接は `"HTTP"` |
| `remoteEndPoint` | `"IP:port"` 文字列 |

---

### `stopChannelConnection`

**パラメータ:** `[channelId: string, connectionId: int]`

**返却値:** `boolean`（`true` = 切断成功、`false` = 対象なし または対象外）

**制約:** `type: "relay"`（PCPOutputStream）のみ切断可。`type: "direct"` および `type: "source"` は対象外で `false` を返す。

---

### `getYellowPages`

**パラメータ:** なし

**返却値:**
```json
[
  {
    "yellowPageId": 0,
    "name": "0yp",
    "uri": "pcp://yayaue.me/",
    "announceUri": "pcp://yayaue.me/"
  }
]
```

| フィールド | 説明 |
|---|---|
| `yellowPageId` | config.toml の `[[yp]]` エントリの 0 始まりインデックス |
| `name` | `[[yp]].name` |
| `uri` / `announceUri` | `[[yp]].addr`（`pcp://` スキームがなければ自動付与） |

### `updateYPChannels`

**パラメータ:** なし。省略、`null`、空配列が使える。他の引数は参照しない。

`[[yp]].channels_url` が設定された全 YP から一覧を取得する。`-yp` による PCP 掲載先の選択とは独立。URL 未設定の YP は取得しない。管理 API の既存の認証・Origin 制限が適用され、サイト利用者にはこの管理 API を公開しない。

**返却値:** 次のオブジェクトの配列。ID は大文字の 32 桁 hex。文字列は HTML エンティティをデコードする。数値の空欄・不正値は `null`、非表示数の `-1` は維持する。

```json
[{"yellowPage":"SP","name":"番組","channelId":"0123456789ABCDEF0123456789ABCDEF","tracker":"8.8.8.8:7144","contactUrl":"","genre":"ゲーム","description":"説明","comment":"","bitrate":1500,"contentType":"FLV","trackTitle":"","album":"","creator":"","trackUrl":"","listeners":-1,"relays":-1,"uptime":5400}]
```

`creator` はトラックの Artist、`uptime` は Duration の `H:MM` を秒に変換した値（単一整数も秒として受理）。ゼロ ID の告知行と複数 YP の重複 ID はこの API では残す。チャンネルを作成・中継する操作ではない。

キャッシュはサイトと共通で 60 秒。更新中の要求は同じ取得完了を待つ。取得全体は最大 5 秒、並行 HTTP 取得は最大 4、各応答は最大 4 MiB、行は 64 KiB 未満、各 YP は最大 10,000 行。UTF-8 の index.txt を解析し、通常 19 フィールド、末尾省略の 10 フィールド以上も受理する。不正 ID 行はスキップし、短すぎる行・HTML・不正 UTF-8・サイズ超過・HTTP 非 200 はその YP の取得失敗となる。

YP 単位の失敗は RPC エラーにはせず、最終成功から 5 分未満ならその一覧、以後はその YP の空一覧を使用する。設定なし / 初回全失敗なら `[]`。失敗状態はサイトの `/site/api/directory` に含める。取得処理自体が完了できない場合の RPC エラーは `-32603` / `YP directory update failed`。戻り値の採用範囲は [ADR 0021](../../decisions/0021-yp-channel-directory.md)。

---

### `getChannelRelayTree`

**パラメータ:** `[channelId: string]`

自ノードのリレーツリーを返す。

- **ブロードキャストチャンネル:** 自ノードをルートとする単一要素配列。`isTracker: true`。
- **リレーチャンネル:** 接続先があれば上流ノードをルート、その子に自ノードを置く。
- 自ノード以下は直下の実接続と 180 秒以内の HOST 報告 (最大 32 件) を使い、`upip/uppt` と global/local endpoint の一致で子孫を構築する。親不明・循環は自ノード配下に救済し、同一 SID を重複させない。
- HOST があるノードの人数・Receiving・空き枠・firewall・バージョンは報告値。未報告の直下ノードは handshake 情報を使い、未知の値はゼロ/false。上流は接続先と SID、受信状態を表示し、未取得の属性はゼロ/false。
- 自ノードの IP・firewall は共有疎通状態 (リレーは接続先 family、配信は IPv4)、人数は実接続、空き枠は全体制限込み。未確認のポートは firewalled と表示する。

**返却値（ブロードキャストチャンネルの場合）:**
```json
[
  {
    "sessionId": "aabbccdd...",
    "address": "",
    "port": 7144,
    "isFirewalled": false,
    "localRelays": 2,
    "localDirects": 1,
    "isTracker": true,
    "isRelayFull": false,
    "isDirectFull": false,
    "isReceiving": true,
    "isControlFull": false,
    "version": 1218,
    "versionString": "PeerCast-MI/0.0.1",
    "children": []
  }
]
```

**返却値（リレーチャンネルの場合）:**
```json
[
  {
    "sessionId": "",
    "address": "192.168.1.10",
    "port": 7144,
    "isTracker": false,
    "isReceiving": true,
    "children": [
      {
        "sessionId": "aabbccdd...",
        "address": "",
        "port": 7144,
        "isTracker": false,
        "localRelays": 1,
        "localDirects": 0,
        "isReceiving": true,
        "children": []
      }
    ]
  }
]
```

`address` は空文字列（グローバル IP の取得は YPClient の `oleh.rip` 経由のみであり、API サーバーからは参照不可）。上流ノードの `sessionId` は未取得のため空文字列。

---

## 実装上の制約・注意事項

- ストリームキーはキャッシュファイル (`stream_keys.json`) に永続化される。プロセス再起動後も有効。
- `revokeStreamKey` はキーを無効化するが、そのキーで放送中のチャンネルは停止しない。
- 同じストリームキーで放送中に再度 `broadcastChannel` を呼ぶとエラー。`stopChannel` してから再呼び出しする。
- `broadcastChannel` より先に RTMP push が来ても受け付ける（ストリームキーが発行済みであれば）。チャンネルが作成されるまでの RTMP データは静かにドロップされる。
- `channelId` の照合は大文字・小文字を区別しない。
- `getChannelStatus.status` は `"Receiving"` (データ受信中) または `"Idle"` (未受信)。
- `getChannelConnections` の `recvRate` は常に `0`（受信レートの計測は未実装）。
- `getChannelRelayTree` の自ノード `address` は当該 family のグローバル IP 未取得時のみ空文字列。
- BroadcastID は `broadcast_id` に永続化され、同じ配信パラメータ・StreamKey なら再起動後も ChannelID を維持する。導入前のランダム ID は復元できない。

## サイトの管理者用入口

サイト有効時の `POST <base_path>/admin/api/1` は、許可されたX管理者のセッション・Origin・CSRFを検証したうえで本APIに中継する。メソッド・引数・結果・JSON-RPCエラーは同じ。入口での拒否はHTTPエラー。[サイト仕様](../site.md#管理パネル)を参照。直接 `/api/1` の認証規則は変更しない。
