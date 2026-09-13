# タスク

未着手・進行中・保留の作業を書く。終わったら「完了」に移し、コミットや decisions への参照を添える。
設計判断が必要なものは着手時に [decisions/](decisions/README.md) に記録する。

書式: `- [ ] 内容 — 補足 (関連ファイル / 参照)`

## 未着手

### 互換性・検証

- [ ] peercast-yt から移行したとき同じ ChannelID を保つ必要があるかを判断する。必要なら StreamKey の連結をやめる (または StreamKey が空なら連結しない) 設計変更を ADR に記録する — [decisions/0001](decisions/0001-channel-id-algorithm.md) 検証済み: アルゴリズム自体は peercast-yt と一致するが入力が異なる。PeerCastStation は別方式 (SHA512+MD5) なので一致しない

### 機能

- [ ] HTTP 直接視聴で 200 を先に返す方式のままで良いか (PeerCastStation はデータ到着を待ってから 200 / 504) — [decisions/0014](decisions/0014-stream-on-demand-relay.md)。リレー直後は `icy-name` が空、`Content-Type` が既定値になる問題も同根
- [ ] HTTP 直接視聴の ICY メタデータ (`icy-metaint`) — [spec/components.md 4.9](spec/components.md)
- [ ] push (GIV) 接続の対応範囲を再検討するかどうか — 現状は対象外 ([decisions/0003](decisions/0003-no-push-connection.md))

### 保守

- [ ] `Channel.StartTime` が exported なフィールドのまま — 他と同様にアクセサ経由にするか検討
- [ ] `relay.Client.handshake` が 503 のときも `slog.Info("relay: connected")` を出す — ログレベル・文言の見直し

## 進行中

(なし)

## 保留

- [ ] Web UI (`ui/`) を LAN 上の別ホストから使う場合の `allowed_origins` 設定を UI 側のセットアップ手順に書く — [decisions/0011](decisions/0011-cors-policy.md)。UI の配布方法が決まってから

## 完了

- [x] 2026-09-14 オンデマンドリレーの開始要求を送信元で制限 (`relay_request_from`、既定はプライベートのみ) し、`max_relay_channels` を追加。接続先の制限は設けない — [decisions/0016](decisions/0016-relay-request-source-policy.md)
- [x] 2026-09-14 ストリーム位置が 4 GiB で一周したときの扱い — `Since` の再送ループで下流が切断されていた。ヘッダー変更時の位置巻き戻りと RTMP のヘッダー位置も同時に修正 — [decisions/0015](decisions/0015-stream-position-wrap.md)
- [x] 2026-09-14 `x-peercast-pos` の `reqPos == 0` を「未指定」と同一視する点の妥当性確認 — 変更不要。PeerCastStation・peercast-yt とも実質同じ扱い ([decisions/0002](decisions/0002-x-peercast-pos-resume.md) に検証結果を追記)
- [x] 2026-09-14 `/stream/<id>[.flv]?tip=` での自動リレー開始 (peca-live 互換) と、リレー初回ヘッダーの二重送信バグ修正 — [decisions/0014](decisions/0014-stream-on-demand-relay.md)
- [x] 2026-09-13 `bumpChannel` の名前指定 params 対応 (peca-live 互換) — [decisions/0010](decisions/0010-bump-channel-named-params.md)
- [x] 2026-09-13 エンコーダーが `broadcastChannel` より先に接続すると FLV ヘッダーが載らないバグ — `internal/rtmp/server.go` (`headerAppliedTo`)
- [x] 2026-09-13 JSON-RPC の CORS ワイルドカード廃止 — [decisions/0011](decisions/0011-cors-policy.md)
- [x] 2026-09-13 オンデマンドリレーの組み立てを `main.go` から `Manager.StartRelay` へ — [decisions/0012](decisions/0012-relay-lifecycle-in-manager.md)
- [x] 2026-09-13 `Channel` から下流ノード情報を `nodeTable` (`channel/nodes.go`) に分離
- [x] 2026-09-13 仕様書を実装に合わせて全面更新、docs を spec / decisions / reference / tasks に再編 — [decisions/0013](decisions/0013-docs-follow-code.md)
- [x] 2026-09-13 環境依存で遅い・落ちるテストの修正 (`relay`、`yp` の `127.0.0.1:1` 依存と `Stop()` の 3 秒待ち)
