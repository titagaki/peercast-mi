# TODO / 改善候補

## 機能・設計

- [x] **ContentBuffer のリングバッファサイズを設定可能にする:** `content_buffer_seconds` でバッファ保持秒数を指定（デフォルト 8 秒）。ビットレートからパケット数を自動計算

## PeerCastStation との差異 (視聴・リレー通信)

PeerCastStation のソースコードと比較した結果。対応ファイル: `internal/relay/client.go`, `internal/channel/content.go`, `internal/channel/channel.go`

### 再接続ロジック

- [x] **バックオフの削除:**
  PeerCastStation は再接続時に delay=0 で即座にリトライし、接続可能なホストがなくなったら停止する (NoHost)。
  peercast-mi は 5s→120s の指数バックオフで待機していた。
  → PeerCastStation に合わせて即時再接続 + ホスト枯渇で停止に変更済み。
  - 参照: `SourceStreamBase.StartConnection` → `OnConnectionStopped` → `args.Delay` (PCP では常に 0)
  - 参照: `PCPSourceStream.SelectSourceHost` → null なら `DoStopStream(NoHost)`

- [x] **OffAir / ConnectionError の再接続判断:**
  PeerCastStation では tracker 以外のノードから OffAir や ConnectionError を受けた場合、そのノードを ignore して別ノードに再接続する。tracker から受けた場合のみ停止。
  peercast-mi は OffAir を受けると一律停止していたため、中間リレーノードが落ちただけでリレーチェーン全体が切れていた。
  → PeerCastStation に合わせて修正済み。
  - 参照: `PCPSourceStream.OnConnectionStopped` (ConnectionError/OffAir で `connection.SourceUri != this.SourceUri` なら再接続)

- [x] **`x-peercast-pos` でコンテンツ位置を送信:**
  PeerCastStation は再接続時に `Channel.ContentPosition` (最新パケット末尾のバイト位置) を送り、上流が途中からデータを送れるようにしている。
  peercast-mi は常に 0 を送信していたため、再接続のたびに先頭から受け直していた。
  → `ContentPosition()` メソッドを追加し、handshake で送信するよう変更済み。
  - 参照: `PCPSourceStream.ProcessRelayRequest` → `x-peercast-pos:{Channel.ContentPosition}`

### コンテンツバッファリング

- [x] **再接続時のバッファクリア:**
  PeerCastStation は `AddSourceStream()` で `contentHeader = null` + `contents.Clear()` してからストリームを受信し直す。さらに `streamIndex` をインクリメントして旧ストリームのデータを `ContentCollection.Add()` 内で自動排除する。
  peercast-mi はリレークライアント再接続時にリングバッファをクリアしないため、ヘッダ変更後にストリーム位置が巻き戻ると古いデータと新しいデータが混在する可能性があった。
  → `SetHeader` でリングバッファ (`count`, `headerPos`) をリセットするよう変更済み。新しいヘッダ受信時に旧ストリームのデータパケットが自動的に破棄される。
  - 参照: `Channel.AddSourceStream` → `contentHeader = null; contents.Clear(); streamIndex++`
  - 参照: `ContentCollection.Add()` → `content.Stream < item.Stream` なら旧ストリームとして除去

### 出力ストリームへのコンテンツ配信

- [x] **ソース切断時の出力ストリーム通知:**
  PeerCastStation は `RemoveSourceStream` → 全 sink に `OnStopped` 送信 → sink リストクリアという明示的な通知を行う。
  peercast-mi はリレークライアント再接続中もチャンネル・出力ストリームはそのまま残り、データが来なくなると stall timeout で自然に閉じていた。
  → `Client.Run()` 終了時 (全ホスト枯渇・tracker OffAir) に `defer ch.CloseAll()` で全出力ストリームを即座に閉じるよう変更済み。再接続ループ中 (ホスト切り替え) では呼ばないため、素早い再接続時に視聴を継続できる利点は維持。
  - 参照: `Channel.RemoveSourceStream` → `sinks` に `OnStopped` → `sinks` クリア

### その他の差異 (参考)

- [x] **`Stop()` によるブロッキング I/O の中断:**
  PeerCastStation は `CancellationToken` で handshake 中の読み書きを中断できる。peercast-mi の `Stop()` は `stopCh` を閉じるだけで、`net.Dial` や `pcp.ReadAtom` のブロッキング I/O を直接中断するメカニズムがなかった。
  → `context.Context` を導入。`DialContext` で接続中のキャンセルに対応し、接続確立後は context キャンセル時に `conn.Close()` してブロッキング読み取りを即座に中断するよう変更済み。
