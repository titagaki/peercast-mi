# peercast-mm

Go 製 PeerCast ノード。**ブロードキャストノード**（RTMP → PCP）と**リレーノード**（上流 PeerCast ノードから受け取って中継）の両方に対応する。

複数チャンネルを同時に扱える。チャンネルは起動時ではなく JSON-RPC API で動的に作成する。

## 必要なもの

- Go 1.22 以上
- RTMP 対応エンコーダー (OBS Studio など)

## インストール

```sh
git clone https://github.com/titagaki/peercast-mm.git
cd peercast-mm
go build -o peercast-mm .
```

## 起動

あらかじめ `config.toml` で YP とポートを設定しておく。

```sh
./peercast-mm
```

### オプション

| フラグ | デフォルト | 説明 |
|---|---|---|
| `-yp` | config.toml の先頭エントリ | 使用する YP 名 |
| `-config` | `config.toml` | 設定ファイルのパス |

### config.toml

```toml
rtmp_port     = 1935
peercast_port = 7144

# ログレベル: "debug" | "info" | "warn" | "error"  (デフォルト: "info")
# log_level = "info"

[[yp]]
name = "moe"
addr = "pcp://yp.pcmoe.net/"

[[yp]]
name = "local"
addr = "pcp://localhost:7144/"
```

### ログ

ログは標準エラー出力 (stderr) に出力される。ファイルに保存したい場合はリダイレクトする。

```sh
./peercast-mm 2>> peercast.log
```

## チャンネルの作成と配信

チャンネルは JSON-RPC API を使って動的に作成する。

### 1. ストリームキーを発行する

```sh
curl -s -X POST http://localhost:7144/api/1 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"issueStreamKey","params":[],"id":1}'
```

```json
{"jsonrpc":"2.0","id":1,"result":{"streamKey":"sk_a1b2c3d4e5f6..."}}
```

ストリームキーはプロセスが終了するまで有効。チャンネルを止めても失効しない。

### 2. エンコーダーを接続する

OBS Studio の設定:

- **サービス:** カスタム
- **サーバー:** `rtmp://localhost/live`
- **ストリームキー:** 手順 1 で取得したキー (`sk_a1b2c3d4e5f6...`)

エンコーダーの接続はこの時点で行ってもよいし、手順 3 の後でもよい。
ストリームキーが発行済みであれば RTMP 接続は受け付けられる。

### 3. チャンネルを開始する

```sh
curl -s -X POST http://localhost:7144/api/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0","method":"broadcastChannel","params":[{
      "sourceUri": "rtmp://127.0.0.1:1935/live/sk_a1b2c3d4e5f6...",
      "info": {
        "name":    "テスト配信",
        "genre":   "ゲーム",
        "url":     "https://example.com",
        "desc":    "",
        "comment": "",
        "bitrate": 3000
      },
      "track": {"title":"","creator":"","album":"","url":""}
    }],"id":2}'
```

```json
{"jsonrpc":"2.0","id":2,"result":{"channelId":"0123456789abcdef0123456789abcdef"}}
```

同じパラメータで再度呼び出すと同じ `channelId` が返る（決定論的生成）。

### 4. チャンネルを停止する

```sh
curl -s -X POST http://localhost:7144/api/1 \
  -H 'Content-Type: application/json' \
  -d '{"jsonrpc":"2.0","method":"stopChannel","params":["0123456789abcdef0123456789abcdef"],"id":3}'
```

チャンネルが停止してもストリームキーは残るため、手順 3 から繰り返せる。

## リレーチャンネルの開始

他の PeerCast ノードからストリームを受け取って中継する。チャンネル ID と上流ノードのアドレスを指定する。

```sh
curl -s -X POST http://localhost:7144/api/1 \
  -H 'Content-Type: application/json' \
  -d '{
    "jsonrpc":"2.0","method":"relayChannel","params":[{
      "upstreamAddr": "192.168.1.10:7144",
      "channelId":    "0123456789abcdef0123456789abcdef"
    }],"id":1}'
```

```json
{"jsonrpc":"2.0","id":1,"result":{"channelId":"0123456789abcdef0123456789abcdef"}}
```

接続失敗時は指数バックオフ（5 秒〜120 秒）で自動再接続する。`stopChannel` で停止できる。

## 視聴・リレー

peercast-mm はポート 7144 で待ち受ける。

| URL | 用途 |
|---|---|
| `http://localhost:7144/stream/<channelId>` | メディアプレイヤーで直接視聴 |
| `http://localhost:7144/channel/<channelId>` | PeerCast ノードからのリレー接続 |

`channelId` は `broadcastChannel` / `relayChannel` の返却値、または `getChannels` で確認できる。

## JSON-RPC API

詳細仕様は [docs/api/jsonrpc.md](docs/api/jsonrpc.md) を参照。

| メソッド | 説明 |
|---|---|
| `issueStreamKey` | ストリームキーを発行する |
| `broadcastChannel` | ブロードキャストチャンネルを開始する |
| `relayChannel` | リレーチャンネルを開始する |
| `getChannels` | チャンネルの一覧 |
| `getChannelInfo` | チャンネル情報を取得 |
| `getChannelStatus` | チャンネルステータスを取得 |
| `setChannelInfo` | チャンネル情報を更新 |
| `stopChannel` | チャンネルを停止 |
| `bumpChannel` | YP への bcst を即時送信 |
| `getChannelConnections` | 接続一覧を取得 |
| `stopChannelConnection` | 特定の接続を切断 |
| `getYellowPages` | YP 一覧を取得 |
| `getChannelRelayTree` | リレーツリーを取得 |
| `getVersionInfo` | エージェント名を取得 |
| `getSettings` | ポート設定を取得 |
