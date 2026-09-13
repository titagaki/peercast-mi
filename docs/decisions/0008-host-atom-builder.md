# 0008: Host アトム構築を pcputil.BuildHostAtom に集約

- 状態: 遡及記録 (決定は 2026-04 頃、記録 2026-09-13)

## 背景

PCP の Host アトムは YP への bcst、下流への初期送信・切断時案内、上流への BCST HOST の 3 箇所で組み立てていて、フラグ (`flg1`) の意味やフィールドの並びが場所ごとに微妙に違っていた。特に PeerCastStation は ip/port を 2 組 (global / local) 期待するが、`peercast-pcp` の `HostPacket.BuildAtom()` は 1 組しか出さない。

## 決定

`internal/pcputil.BuildHostAtom(HostAtomParams)` に一本化し、手動でアトムを組み立てる。

- `HostAtomParams` で YP 固有 (`TrackerAtom`)・リレー固有 (`UphostIP/Port/Hops`) の差分を表現する
- `flg1` は `IsReceiving` / `RelayFull` / `DirectFull` / `IsTracker` / `HasGlobalIP` から導出し、「枠が満杯なら Relay / Direct ビットを落とす」「データを受信していなければ Recv を立てない」を全箇所で統一する

## 結果・影響

- 3 箇所の実装差がなくなり、フラグの変更が 1 箇所で済む
- `peercast-pcp` の `HostPacket` は使わない (2 組の ip/port を出せないため)

## 参照

- `internal/pcputil/host.go`
- [peercaststation-compat.md](peercaststation-compat.md) 「Host atom flags1 の slot 状態反映」
