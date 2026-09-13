# 0006: パッケージ間依存はインターフェースで切る

- 状態: 遡及記録 (決定は 2026-03〜04、記録 2026-09-13)

## 背景

`channel.Manager` はほぼ全パッケージから使われる中心的な型で、具象型に直接依存するとテストでモックが作れず、また `channel` ↔ `relay` のような import 循環が起きる。

## 決定

利用側パッケージごとに「必要なメソッドだけ」のインターフェースを定義し、`*channel.Manager` はそれを満たす形で渡す。

| 利用側 | インターフェース | 必要なもの |
|:--|:--|:--|
| `jsonrpc` | `ChannelManager` | キー発行・Broadcast・Stop・GetByID・List 等 |
| `servent` | `ChannelStore` | GetByID・TotalRelays・TotalSendRate |
| `yp` | `ChannelLister` | List |
| `rtmp` | `ChannelManager` | IsIssuedKey・GetByStreamKey・Stop |
| `channel` (relay を指す側) | `RelayHandle` / `RelayFactory` | Run・Stop・SetGlobalIP・SetOnStopped / 生成 |

出力ストリームも同様に、`OutputStream` (共通) と `BcstForwarder` / `RelayEvictable` / `RelayNodeInfo` (PCP 固有) に分け、`Channel` は型アサーションで PCP 固有機能を使う。HTTPOutputStream はそれらを実装しない。

## 結果・影響

- 各パッケージのテストが軽いモックで書ける (`jsonrpc/server_test.go` の `recordingManager` など)
- `channel` → `relay` の依存を `RelayHandle` + `RelayFactory` の注入で回避し、`main` が配線する (0012)
- インターフェースが利用側にあるため、Manager にメソッドを足しても利用側のインターフェースを更新しない限り影響しない

## 参照

- `internal/*/` 各パッケージ冒頭のインターフェース定義
