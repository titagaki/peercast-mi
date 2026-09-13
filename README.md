# peercast-mi

Go 製 PeerCast ノード。Root モード以外（ブロードキャスト・リレー）に対応。

- Go 製でマルチプラットフォーム（Linux / macOS / Windows）
- RTMP → PCP のブロードキャスト配信
- ストリームキーによる RTMP 接続の認証
- JSON-RPC API / Web UI によるチャンネル管理

## 必要なもの

- Go 1.25 以上
- RTMP 対応エンコーダー (OBS Studio など)

## インストール

```sh
git clone https://github.com/titagaki/peercast-mi.git
cd peercast-mi
go build -o peercast-mi .
```

## 起動

あらかじめ `config.toml` で YP とポートを設定しておく。

```sh
./peercast-mi
```

### オプション

| フラグ | デフォルト | 説明 |
|---|---|---|
| `-yp` | config.toml の先頭エントリ | 配信の掲載先にする YP 名 (視聴時の tracker 探索は全 `[[yp]]` に問い合わせる) |
| `-config` | `config.toml` | 設定ファイルのパス |

### config.toml

```toml
rtmp_port     = 1935
peercast_port = 7144

# ログレベル: "debug" | "info" | "warn" | "error"  (デフォルト: "info")
# log_level = "info"

# 直下に接続できるリレーノード数の上限 (デフォルト: 0 = 無制限)
# max_relays = 4

# 全チャンネル合計のリレーノード数の上限 (デフォルト: 0 = 無制限)
# max_relays_total = 16

# 直下に接続できる HTTP 視聴者数の上限 (デフォルト: 0 = 無制限)
# max_listeners = 4

# 全チャンネル合計の上り帯域上限 kbps (デフォルト: 0 = 無制限)
# max_upstream_kbps = 0

# コンテンツリングバッファが保持する秒数 (デフォルト: 0 = 8秒)
# ビットレートから必要なパケット数を自動計算する
# content_buffer_seconds = 8

# 視聴・リレーのないリレーチャンネルを自動削除するまでの分数 (デフォルト: 20、0 = 無効)
# channel_cleanup_minutes = 20

# 非 localhost からの JSON-RPC アクセスに要求する Basic 認証 (どちらか未設定なら非 localhost を拒否)
# admin_user = "admin"
# admin_pass = "secret"

# JSON-RPC への CORS 要求を許可するオリジン。localhost / 127.0.0.1 のオリジンは常に許可される。
# Web UI を LAN 上の別ホストから開く場合などに追加する
# allowed_origins = ["http://192.168.1.10:5173"]

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
./peercast-mi 2>> peercast.log
```

## チャンネルの作成と配信

`ui/` ディレクトリに React ベースの Web UI がある。

```sh
cd ui
npm install
npm run dev
# → http://localhost:5173/
```

バックエンド (ポート 7144) が起動していれば、そのまま操作できる。

### 1. ストリームキーを発行する

Web UI の **Stream Keys** タブでアカウント名とストリームキーを入力し、**Issue** ボタンを押す。
ストリームキーはプロセスが終了しても有効（ファイルに永続化される）。

### 2. エンコーダーを接続する

OBS Studio の設定:

- **サービス:** カスタム
- **サーバー:** `rtmp://localhost/live`
- **ストリームキー:** 手順 1 で発行したキー

エンコーダーの接続はこの時点で行ってもよいし、手順 3 の後でもよい。
ストリームキーが発行済みであれば RTMP 接続は受け付けられる。

### 3. チャンネルを開始する

**Channels** タブの **Broadcast** ボタンを押し、ストリームキー・チャンネル名などを入力して **Start Broadcast** を押す。

### 4. チャンネルを停止する

**Channels** タブのチャンネル一覧から対象チャンネルの **Stop** ボタンを押す。
チャンネルが停止してもストリームキーは残るため、手順 3 から繰り返せる。

## 視聴・リレー

peercast-mi はポート 7144 で待ち受ける。

| URL | 用途 |
|---|---|
| `http://localhost:7144/stream/<channelId>` | メディアプレイヤーで直接視聴 |
| `http://localhost:7144/channel/<channelId>` | PeerCast ノードからのリレー接続 |

`channelId` は `broadcastChannel` の返却値、または `getChannels` で確認できる。

## JSON-RPC API

メソッド一覧・パラメータ・返却値の詳細は [docs/spec/api/jsonrpc.md](docs/spec/api/jsonrpc.md) を参照。

## ドキュメント

仕様・設計判断・参照資料・タスクは [docs/](docs/README.md) を参照。
