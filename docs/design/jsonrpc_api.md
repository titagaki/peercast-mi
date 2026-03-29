# JSON-RPC API 実装リスト

peercast-mm に実装する JSON-RPC API の一覧。

エンドポイント: `POST /api/1`（PeerCastStation / peercast-yt 互換）

---

## 実装対象メソッド

### サーバー情報

| メソッド | 優先度 | 概要 |
|---|---|---|
| `getVersionInfo` | 高 | エージェント名・バージョンを返す |
| `getSettings` | 中 | ポート等の基本設定を返す |

### チャンネル管理

| メソッド | 優先度 | 概要 |
|---|---|---|
| `getChannels` | 高 | アクティブなチャンネル一覧を返す |
| `getChannelInfo` | 高 | チャンネルの info / track / yellowPages を返す |
| `getChannelStatus` | 高 | チャンネルの状態・リレー数・リスナー数を返す |
| `setChannelInfo` | 高 | チャンネルの info / track を更新する |
| `stopChannel` | 高 | チャンネルを停止する |
| `bumpChannel` | 中 | チャンネルをバンプする（YP への再通知） |

### 接続管理

| メソッド | 優先度 | 概要 |
|---|---|---|
| `getChannelConnections` | 高 | チャンネルの接続一覧（リレー・ダイレクト・ソース）を返す |
| `stopChannelConnection` | 高 | 指定した接続を切断する |

### YP

| メソッド | 優先度 | 概要 |
|---|---|---|
| `getYellowPages` | 高 | config.toml に設定された YP 一覧を返す |

### リレーツリー

| メソッド | 優先度 | 概要 |
|---|---|---|
| `getChannelRelayTree` | 低 | チャンネルのリレーツリーを返す（自ノードのみ） |

---

## 未実装（対象外）

| メソッド | 理由 |
|---|---|
| `fetch` | RTMP push 前提のアーキテクチャと設計が合わない。別途検討 |
| `getLog` / `clearLog` / `getLogSettings` / `setLogSettings` | ログ管理機能なし |
| `getNewVersions` / `getNotificationMessages` / `getPlugins` | 該当機能なし |
| `getYPChannels` / `getYellowPageProtocols` / `removeYellowPage` | YP からの受信・操作機能なし |
| `getSourceStreams` | 複数ソース管理は対象外 |

---

## 各メソッドの仕様

### `getVersionInfo`

**パラメータ:** なし

**返却値:**
```json
{
  "agentName": "peercast-mm/0.1.0"
}
```

---

### `getSettings`

**パラメータ:** なし

**返却値:**
```json
{
  "serverPort": 7144,
  "rtmpPort": 1935
}
```

---

### `getChannels`

**パラメータ:** なし

**返却値:** チャンネル情報オブジェクトの配列（`getChannelInfo` + `getChannelStatus` の内容を含む）

```json
[
  {
    "channelId": "0123456789abcdef0123456789abcdef",
    "status": {
      "status": "Receiving",
      "source": "rtmp://localhost/live",
      "totalDirects": 1,
      "totalRelays": 2,
      "isBroadcasting": true,
      "isRelayFull": false,
      "isDirectFull": false,
      "isReceiving": true
    },
    "info": {
      "name": "チャンネル名",
      "url": "https://example.com",
      "desc": "説明",
      "comment": "",
      "genre": "ジャンル",
      "type": "FLV",
      "bitrate": 500
    },
    "track": {
      "title": "",
      "creator": "",
      "url": "",
      "album": ""
    },
    "yellowPages": [
      { "yellowPageId": 0, "name": "0yp" }
    ]
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
    "desc": "説明",
    "comment": "",
    "genre": "ジャンル",
    "type": "FLV",
    "bitrate": 500
  },
  "track": {
    "title": "",
    "creator": "",
    "url": "",
    "album": ""
  },
  "yellowPages": [
    { "yellowPageId": 0, "name": "0yp" }
  ]
}
```

---

### `getChannelStatus`

**パラメータ:** `[channelId: string]`

**返却値:**
```json
{
  "status": "Receiving",
  "source": "rtmp://localhost/live",
  "totalDirects": 1,
  "totalRelays": 2,
  "isBroadcasting": true,
  "isRelayFull": false,
  "isDirectFull": false,
  "isReceiving": true
}
```

---

### `setChannelInfo`

**パラメータ:** `[channelId: string, info: object, track: object]`

`info` / `track` の構造は `getChannelInfo` の返却値と同じ。

**返却値:** `null`

---

### `stopChannel`

**パラメータ:** `[channelId: string]`

**返却値:** `null`

---

### `bumpChannel`

**パラメータ:** `[channelId: string]`

YP への bcst を即時送信する。

**返却値:** `null`

---

### `getChannelConnections`

**パラメータ:** `[channelId: string]`

詳細仕様は [../../peca-docs/docs/protocol/jrpc_connections.md](参照)。

**返却値:** 接続情報オブジェクトの配列（先頭がソース接続）

```json
[
  {
    "connectionId": -1,
    "type": "source",
    "status": "Receiving",
    "sendRate": 0,
    "recvRate": 150000,
    "protocolName": "RTMP",
    "remoteEndPoint": "127.0.0.1:52000"
  },
  {
    "connectionId": 1,
    "type": "relay",
    "status": "Connected",
    "sendRate": 65000,
    "recvRate": 0,
    "protocolName": "PCP",
    "remoteEndPoint": "203.0.113.5:7144"
  }
]
```

---

### `stopChannelConnection`

**パラメータ:** `[channelId: string, connectionId: int]`

**返却値:** `boolean`（`true` = 切断成功、`false` = 対象なし）

**制約:** `type = "relay"` の接続のみ対象（ダイレクト接続は切断不可）。

---

### `getYellowPages`

**パラメータ:** なし

config.toml の `[[yp]]` エントリを返す。

**返却値:**
```json
[
  {
    "yellowPageId": 0,
    "name": "0yp",
    "uri": "pcp://yayaue.me/",
    "announceUri": "pcp://yayaue.me/",
    "channelCount": 1
  }
]
```

---

### `getChannelRelayTree`

**パラメータ:** `[channelId: string]`

詳細仕様は [../../peca-docs/docs/protocol/jrpc_connections.md](参照)。

自ノード（ブロードキャストノード）のみのツリーを返す。YP 連携している場合は下流ノードも含まれる可能性があるが、現状は未対応。

**返却値:** ノードオブジェクトの配列（`children` は常に空配列）
