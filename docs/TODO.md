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

### PCP 出力ストリーム (PCPOutputStream)

- [ ] **ハンドシェイク後に PCP_OK を送信:**
  PeerCastStation は helo/oleh 交換後、リレー受け入れ時に `PCP_OK (1)` を送信する。
  peercast-mi は oleh 送信後すぐに `sendInitial()` に進み、`PCP_OK` を送信していない。
  - 参照: `PCPOutputStream.cs` DoHandshake → `stream.WriteAsync(new Atom(Atom.PCP_OK, (int)1))`

- [ ] **リレー満杯時に 503 + HOST リスト + QUIT を返す:**
  PeerCastStation は `MakeRelayable()` で空きを作れない場合、HTTP 503 を返し、helo/oleh 交換後に代替ホストリストを送信して `QUIT + UNAVAILABLE` で切断する。さらに BAN リスト (90秒) に追加する。
  peercast-mi は `tryAdmit()` が false の場合、単にコネクションを閉じるだけで代替ホストの案内がない。
  - 参照: `PCPOutputStream.cs` DoHandshake → `isRelayFull` 時に `SelectSourceHosts` → `SendHost` → `HandshakeErrorException(UnavailableError)`
  - 参照: `PCPOutputStream.cs` BeforeQuitAsync → `channel.Ban(90秒)` + `SendHost`

- [ ] **劣勢リレー接続の強制切断 (MakeRelayable):**
  PeerCastStation はリレー枠が満杯でも、firewalled またはリレー能力がない (RelayFull かつ LocalRelays < 1) 下流ノードを切断して枠を空ける。
  peercast-mi は単純な数値比較のみで、品質の低いリレーを蹴り出す機構がない。
  - 参照: `Channel.cs` MakeRelayable → `IsFirewalled || (IsRelayFull && LocalRelays < 1)` な sink を `OnStopped(UnavailableError)` で切断

- [ ] **Overflow (送信遅延) 検出:**
  PeerCastStation はキューの先頭と新メッセージのタイムスタンプ差が 5 秒を超えると Overflow として `StopReason.SendTimeoutError` → `QUIT + SKIP` を送信して切断する。
  peercast-mi は stall timer (5秒間データが来なかった場合) でタイムアウトし、QUIT コードは SHUTDOWN。「送信が追いつかない」ケース（データは来ているが書き込みが遅い）の検出方法が異なる。
  - 参照: `PCPOutputStream.cs` Enqueue → `(msg.Timestamp-nxtMsg.Timestamp).TotalMilliseconds > 5000` → Overflow → `SendTimeoutError`

- [ ] **大きいコンテンツパケットの分割送信:**
  PeerCastStation は 15KB を超えるコンテンツパケットを 15KB 単位に分割し、2番目以降に `Fragment` フラグを付けて送信する。
  peercast-mi はパケットをそのまま送信しており、分割ロジックがない。
  - 参照: `PCPOutputStream.cs` CreateContentBodyPacket → `MaxBodyLength = 15*1024` で分割、`PCPChanPacketContinuation.Fragment` 付与

- [ ] **シャットダウン時に上流ノード情報を返す:**
  PeerCastStation はシャットダウン等で下流ノードを切断する際、自分が接続していた上流ノードの情報を HOST として返す。これにより下流は直接上流に接続し直せる。
  peercast-mi は `QUIT + SHUTDOWN` を送信するだけで上流ノード情報は返さない。
  - 参照: `PCPOutputStream.cs` BeforeQuitAsync → `StopReason.UserShutdown` 時に上流の `RemoteEndPoint`/`RemoteSessionID` を HOST として送信

- [ ] **ハンドシェイクタイムアウト:**
  PeerCastStation はハンドシェイク全体に 18 秒のタイムアウトを設けている。
  peercast-mi はハンドシェイクに明示的なタイムアウトがなく TCP レベルのデフォルトに依存。
  - 参照: `PCPOutputStream.cs` `PCPHandshakeTimeout = 18000` → `handshakeCT.CancelAfter`

- [ ] **ChannelInfo/Track の bcst 送信 (配信チャンネルのリレー):**
  PeerCastStation は `IsBroadcasting` かつヘッダー送信済みの場合、ChannelInfo/Track 変更時に下流に `PCP_BCST` で wrapped した `PCP_CHAN` (info+track) を送信する。
  peercast-mi は `PCP_CHAN` (bcst 無し) で直接送信する。
  - 参照: `PCPOutputStream.cs` SendRelayBody → `ChannelInfo`/`ChannelTrack` 時に `BcstChannelInfo()`

- [ ] **ping 時のサイトローカルアドレス判定:**
  PeerCastStation は ping 成功してもリモートアドレスが `IsSiteLocal()` の場合は `remote_port = 0` (ポート未開放扱い) にする。
  peercast-mi はサイトローカル判定がなく、ping 成功すれば無条件にポートを設定する。
  - 参照: `PCPOutputStream.cs` OnHandshakePCPHelo → `remoteEndPoint.Address.IsSiteLocal()` なら `remote_port = 0`

- [ ] **チャンネル Status チェック:**
  PeerCastStation はリレーリクエスト時に `channel.Status != SourceStreamStatus.Receiving` ならば 404 を返す。
  peercast-mi は���ャンネルが存在すればデータの有無に関わらず受け入れる。
  - 参照: `PCPOutputStream.cs` Invoke → `channel.Status != SourceStreamStatus.Receiving` → NotFound

### HTTP 出力ストリーム (HTTPOutputStream)

- [ ] **ヘッダー変更時の挙動:**
  PeerCastStation は HTTP ストリームでヘッダーが変更されると新しいヘッダーを送信してストリームを継続する。
  peercast-mi はヘッダー変更時に「HTTP/FLV では途中でヘッダーを差し替えられない」として切断する。
  - 参照: `HTTPOutputStream.cs` StreamHandler → `ContentHeader` 時に `WriteAsync(packet.Content.Data)` で新ヘッダーを送信して継続

- [ ] **Content の Timestamp ベース順序保証:**
  PeerCastStation は HTTP ストリームで `content.Timestamp > sent.body.Timestamp` で順序を保証し、古いコンテンツの再送を防ぐ。
  peercast-mi は位置 (pos) ベースの線形走査のみで Timestamp による重複排除はない。
  - 参照: `HTTPOutputStream.cs` StreamHandler → `c.Timestamp > sent.Value.body.Timestamp || (同一 Timestamp && Position > sent.body.Position)`

### その他の差異 (参考)

- [x] **`Stop()` によるブロッキング I/O の中断:**
  PeerCastStation は `CancellationToken` で handshake 中の読み書きを中断できる。peercast-mi の `Stop()` は `stopCh` を閉じるだけで、`net.Dial` や `pcp.ReadAtom` のブロッキング I/O を直接中断するメカニズムがなかった。
  → `context.Context` を導入。`DialContext` で接続中のキャンセルに対応し、接続確立後は context キャンセル時に `conn.Close()` してブロッキング読み取りを即座に中断するよう変更済み。
