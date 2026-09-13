# 0012: オンデマンドリレーの組み立ては Manager.StartRelay に置く

- 状態: 採用
- 日付: 2026-09-13

## 背景

`/pls/<id>?tip=host:port` で未登録チャンネルを要求されたときにリレーを自動開始する処理は、`main.go` の closure に書かれていた: Channel 生成 → `relay.New` → globalIP 付与 → 停止時に Manager から削除するフック → `AddRelayChannel` → `go Run()`。globalIP も `main` の `atomic.Uint32` と `Listener` で二重に持っていた。`main` にテストできない業務ロジックがあり、GetByID の存在確認と登録が非アトミックでもあった。

## 決定

`channel.Manager` に `StartRelay(channelID, upstreamAddr)` を追加し、上記の手順をロック下で一括して行う。`channel` → `relay` の import 循環は `RelayFactory` (relay クライアントのコンストラクタ) を `main` から `Manager.NewRelay` に注入することで避ける。globalIP は `Manager.SetGlobalIP` が保持し、既存の relay client への伝播と新規 client への付与を担う。

`main` の役割は「config を読む・各コンポーネントを生成する・コールバックを結線する」だけに限定する。

## 結果・影響

- `main.go` の `OnDemandRelay` は `mgr.StartRelay` を呼ぶだけになった
- 既存チャンネルがある場合に二重起動しないことがロックで保証される
- `RelayHandle` インターフェースに `Run()` / `SetOnStopped()` が増えた

## 参照

- `internal/channel/manager.go` (`StartRelay`, `RelayFactory`), `main.go`
- [0006](0006-interface-segregation.md)
