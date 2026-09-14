# タスク

未着手・進行中・保留の作業を書く。終わったら「完了」に移し、コミットや decisions への参照を添える。
設計判断が必要なものは着手時に [decisions/](decisions/README.md) に記録する。

書式: `- [ ] 内容 — 補足 (関連ファイル / 参照)`

## 未着手

### 互換性・検証

- [ ] PCS/yt/mi を配信元・中継・下流として入れ替え、3 段以上の BCST、満杯時退出、bump、長いバックログ、音声のみ FLV、YP 配信停止を実機相互接続で検証する — [修正記録](reviews/2026-09-14-comparison-corrections.md)。Go 回帰テストと静的比較は実施済み
- [ ] IPv6 の多段 PCP/YP・dual stack・疎通確認を PCS/yt と実機検証する — [ADR 0018](decisions/0018-comparison-corrections.md)。ワイヤー形式・候補抽出の単体テストと IPv4 loopback の ping は実施済み
- [ ] peercast-yt から移行したとき同じ ChannelID を保つ必要があるかを判断する。必要なら StreamKey の連結をやめる (または StreamKey が空なら連結しない) 設計変更を ADR に記録する — [decisions/0001](decisions/0001-channel-id-algorithm.md) 検証済み: アルゴリズム自体は peercast-yt と一致するが入力が異なる。PeerCastStation は別方式 (SHA512+MD5) なので一致しない

### 機能

- [ ] X アプリ・公開 HTTPS / RTMPS を設定し、実ログイン、OBS → サイト視聴、他ノードへの公開 PCP 中継を結合検証する — [導入手順](spec/site.md)、[実装記録](reviews/2026-09-14-site-implementation.md)。秘密情報・公開 URL は利用環境で設定する
- [ ] 視聴サイトを拡充する: 外部 YP 一覧、掲示板内蔵閲覧、お気に入り同期、HLS とスマホ実機検証、通知 — [比較表](reviews/2026-09-14-peca-live-viewing-auth-design.md)。初期実装は自ノード一覧・FLV プレイヤー・本人の配信管理
- [ ] 公開運用に合わせて利用者別レート制限 / 帯域割当、アカウント停止と既存配信接続の切断を設計する — [ADR 0019](decisions/0019-authenticated-site.md)。現在は X ログイン成功者が利用可、視聴の同時接続数を制限

### 保守

- [ ] UI の既存開発依存 (Vite など) の audit 指摘を精査・更新し、ビルドとブラウザーテストを再実行する — [UI 改善記録](reviews/2026-09-14-ui-improvements.md)。2026-09-14 時点で 9 件 (high 6 / moderate 2 / low 1)

## 進行中

(なし)

## 保留

- [ ] Web UI (`ui/`) を LAN 上の別ホストから使う場合の `allowed_origins` 設定を UI 側のセットアップ手順に書く — [decisions/0011](decisions/0011-cors-policy.md)。UI の配布方法が決まってから

## 完了

- [x] 2026-09-14 X 認証付き視聴・配信サイトの初期実装、RTMP のキー入りログ除去、キー保存の並行更新・失敗時処理を修正 — [実装・検証記録](reviews/2026-09-14-site-implementation.md)、[ADR 0019](decisions/0019-authenticated-site.md)。公開 PCP は維持。実環境の認証・配信検証は未着手欄
- [x] 2026-09-14 3 実装の比較と理由のない差異の修正 — [修正・検証結果](reviews/2026-09-14-comparison-corrections.md)、[ADR 0018](decisions/0018-comparison-corrections.md)。実機相互接続は未着手欄に分離

- [x] 2026-09-14 `Channel.StartTime` を unexport (外部参照なし、`UptimeSeconds()` のみ)
- [x] 2026-09-14 relay の 503 時ログを整理 — 「connected」「connection error」ではなく「upstream full, collecting alternative hosts」「host full, trying next」を Info で出す。`QUIT+UNAVAILABLE` はエラー扱いしない
- [x] 2026-09-14 HTTP 直接視聴の ICY メタデータ (`icy-metaint`) — 対象外として閉じる。peercast-yt の MP3 + WinAmp 向け機能で PeerCastStation も非対応、FLV には挿入できない ([spec/components.md 4.9](spec/components.md))
- [x] 2026-09-14 push (GIV) 接続の再検討 — 対象外を維持。PeerCastStation も非対応 ([decisions/0003](decisions/0003-no-push-connection.md) に追記)
- [x] 2026-09-14 HTTP 視聴は ChannelInfo を最大 10 秒待ってから 200 (時間切れは 504)。リレー直後の `icy-name` 空・`Content-Type` 既定値も解消 — [decisions/0017](decisions/0017-http-wait-for-info.md)
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
