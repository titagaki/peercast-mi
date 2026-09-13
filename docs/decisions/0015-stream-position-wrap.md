# 0015: ストリーム位置は 32 bit で一周する 1 つの空間として扱う

- 状態: 採用
- 日付: 2026-09-14

## 背景

ストリーム位置 (`Content.Pos`, `headerPos`, `x-peercast-pos`) は PCP の pkt.pos と同じ 32 bit のバイト位置で、2 Mbps なら 4.7 時間程度で 2^32 を超えて一周する。PeerCastStation は内部で `long` に持ち、送信時に `& 0xFFFFFFFF` でマスクするので、上流から届く位置も一周する。

peercast-mi は `ContentBuffer.Since(pos)` の `p.Pos >= pos`、`streamLoop` の `reqPos >= OldestPos()`、`ContentPosition` の `headerPos > newest.Pos` をすべて素の uint32 比較でやっていた。一周をまたぐと `Since` が一周前のパケットを何度も返し直し (→ 5 秒以上前のパケットとして Overflow 判定 → 下流を QUIT+SKIP で切断)、再接続の `x-peercast-pos` も溢れ扱いになって同じことを繰り返す。

調べる過程で同根の問題が 2 つ見つかった。

- ヘッダー変更で位置が巻き戻っても (エンコーダー再接続、上流切り替え) `PCPOutputStream` の送信位置はそのままで、新しい位置のデータが `Since` に引っかからず 5 秒の stall で切断していた。HTTP 出力は Timestamp 順 (`PacketsAfter`) で対処済みだったが PCP 出力は未対処
- RTMP はヘッダーを常に位置 0 に置き、データも 0 から数えていた (ヘッダーがデータと同じ位置に重なる)。エンコーダー再接続でも 0 から数え直すため、同じヘッダーなら `SetHeader` が no-op になって前セッションのデータが残り、位置は巻き戻っていた

## 決定

- 位置の前後判定は `channel.PosBefore(a, b)` (= `int32(a-b) < 0`) に統一する。2^31 未満の距離を前方とみなす。バッファは数 MB しか持たないので十分
- `PCPOutputStream.startCursor` が `x-peercast-pos` をバッファの範囲に丸める: 最古より前なら最古から、最新末尾 (`ContentPosition`) より後ならバックログを飛ばして最新末尾から。peercast-yt の `findOldestPos` ([safePos, lastPos] に丸める) と、PeerCastStation の `AddContentSink` (requestPos より前のバックログを送らないだけで以降は流す) の両方に合う
- 未指定 (`reqPos == 0`) はヘッダー位置ではなく最古のパケットから (バッファが空ならヘッダー位置)。`SetHeader` でバッファは消えるので「ヘッダー以降の全部」に変わりはないが、同じヘッダーを再送し続ける上流 (peercast-yt) ではヘッダー位置がデータから 2^31 以上離れうるため
- ヘッダー変更の通知を処理したら送信位置を新ヘッダー位置に戻し、キーフレーム待ちにする (peercast-yt: `streamIndex` が変わると `streamPos = headPack.pos`)。送信直前にも通知を確認する (HTTP 出力と同じ)
- RTMP はヘッダーを現在位置に置いて `streamPos` をヘッダー長だけ進め、データはその後に続ける (FLV ファイルと同じ並び。`ContentPosition` と一致する)。同じセッション内で同一ヘッダーなら `SetHeader` を呼ばない。セッションの最初のヘッダーは `Channel.ContentPosition()` から始めるので、エンコーダー再接続で位置は巻き戻らず、同じヘッダーでも新しい位置で適用される
- `ContentPosition` は `SetHeader` でパケットが消えることを前提に「パケットがあればその末尾、なければヘッダー末尾」に単純化する (PeerCastStation の `header.Position > content.Position` 判定は、contents が同期的に消えない PeerCastStation 側の都合)

## 却下した案

- **位置を内部で 64 bit に持つ (PeerCastStation 方式)**: 上流から届く位置は 32 bit なので、受信側で一周を検出して上位ビットを補う処理が要る。比較関数 1 つで済む方が小さい
- **`Since` を Timestamp 順にする (HTTP 出力と同じ)**: `x-peercast-pos` の再開位置が位置ベースなので、位置比較は結局残る

## 結果・影響

- 4 GiB を超える配信でも PCP 下流が切断されない
- エンコーダー再接続で PCP 下流が 5 秒で切れなくなった
- RTMP 由来の `x-peercast-pos` / `ContentPosition` がヘッダー長ぶんずれる (ヘッダーの範囲を数えるようになった)。位置は相対的にしか使わないので互換性への影響はない
- [0002](0002-x-peercast-pos-resume.md) の開始位置の規則はこの記録で更新した (0002 自体の判断 = `x-peercast-pos` で再開する・0 は未指定、は維持)

## 参照

- `internal/channel/content.go` (`PosBefore`, `Since`, `PacketsAfter`, `ContentPosition`)、`internal/servent/pcp.go` (`startCursor`, `sendHeaderUpdate`)、`internal/rtmp/server.go` (`rebuildHeader`)
- [spec/components.md](../spec/components.md) 4.1 ストリーム位置、4.3 ContentBuffer、4.8 x-peercast-pos による開始位置
- PeerCastStation `PCPOutputStream.cs` `CreateContentBodyPacket` (`& 0xFFFFFFFF`)、`Channel.cs` `AddContentSink`; peercast-yt `chanpacket.cpp` `findOldestPos`、`servent.cpp` `sendPCPChannel`
