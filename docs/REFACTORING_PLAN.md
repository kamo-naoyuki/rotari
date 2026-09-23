# Rotari リファクタリング計画

## 目的

`cmd/rotari` に CLI、server、Web、state persistence、executor、domain logic が混在している状態を解消し、責務と依存方向を明確にする。

- `cmd/rotari` を CLI、表示、薄い orchestration adapter にする
- domain logic、state management、executor 実装を `internal` に移す
- CLI / server / Web が同じ state / result resolution contract を共有する
- compatibility wrapper と重複テストを整理する

## 現状サマリー

2026-09-23 時点。数値は目安であり、変更前に実際の tree とテストを確認する。

- Phase 1: 完了
- Phase 2: 完了。fallback、persistence、lock、project state、attempt path を state API に集約済み
- Phase 3: 完了。planning、execution wave、retry、carry-forward、summary、lifecycle の境界を internal/run に整理済み
- Phase 4: 完了。CLI、server、run、Web の integration test 配置を整理し、executor 詳細テストは internal/executor に分離済み
- Phase 5: 完了。server contract、transport mapping、Web projection、asset composition の境界を整理済み
- Phase 6: 完了。cmd の forwarding wrapper 監査と最終 package / test / documentation 整理を完了

## 目標 package 構成

```text
cmd/rotari/       CLI、表示、server/web orchestration、integration tests
internal/model/   domain types、queue/job/result rules、pure transformations
internal/state/   filesystem、JSON、lock、project/run state、attempt paths
internal/run/     run planning、selection、retry、carry-forward、lifecycle
internal/executor local / scheduler executor、status、wrapper、process control
internal/server/  server protocol と transport-independent contract
internal/web/     Web projection、timeline、asset-facing contract
```

依存方向は外側から内側へ向ける。`internal` package は `cmd/rotari` を import しない。

```text
cmd/rotari        --> internal/server, internal/run, internal/model, internal/state
internal/run      --> internal/executor, internal/state, internal/model
internal/web      --> internal/state, internal/model
internal/server   --> internal/model
internal/state    --> internal/model
internal/executor --> internal/state, internal/model
```

## Phase 1: executor 境界の完成

**ステータス: 完了**

### 目的

executor 本体、scheduler status、wrapper script、executor 固有テストを `internal/executor` に集約する。

### 完了した作業

- [x] scheduler executor 本体と単体テストを `internal/executor` へ移した
- [x] `state.Store` と logger を constructor から注入した
- [x] wrapper と `status.json` の読み書きを executor package に集約した
- [x] scheduler option parser、shell quoting、command error hint を executor package に置いた
- [x] scheduler terminal 判定と exit code 解決を executor package に集約した
- [x] cmd に残る重複 executor 実装を削除し、dispatch / registry を internal API に統一した

### 残作業

- [x] なし

### 完了条件

- executor 本体が `cmd/rotari` に残っていない
- executor 詳細テストが `internal/executor` にある
- `go build ./cmd/rotari` と関連 package test が通る

## Phase 2: state 層の契約固定

**ステータス: 完了**

### 目的

filesystem layout、JSON persistence、lock、attempt path、project state を `internal/state` の契約として統一し、CLI / server / Web が同じ fallback chain を使う状態にする。

### 完了した作業

- [x] metadata default と missing meta file の `collecting` / RFC3339 timestamp 契約を固定した
- [x] `ReadJobTimestamp` の fallback chain を `job.json` / `status.json` に統一した
- [x] `ReadAttemptTimestamp` と `LoadLocalJobResult` を state API に集約した
- [x] CLI `showJob` / `showRun` と Web `loadWebJobs` の timestamp fallback を共有 API に統一した
- [x] terminal result fallback を `internal/executor` の共通 resolver に統一した
- [x] latest / specific attempt directory と path validation を state API に統一した
- [x] `ProjectPaths` を canonical path type として cmd から利用した
- [x] `WriteJSON`、`LoadMeta`、`LoadQueue`、`LoadRunSummary` の production caller を整理した
- [x] `WriteJSON` の newline / file mode contract をテストで固定した
- [x] lock loading、host ownership、process liveness、stale lock cleanup を `internal/state` に移した
- [x] run directory、required state file、interrupted-run の job directory discovery を `internal/state` に移した
- [x] CLI / server / Web の lock 読み込みを `state.LoadLock` に統一した
- [x] state load/write、lock、missing / invalid JSON、timestamp fallback の contract tests を追加した

### 残作業

- [x] なし

### 完了条件

- `showJob` / `showRun` / `loadWebJobs` が同じ fallback chain をたどる
- state load/write の失敗パターンが package 単位でテストされている
- lock、project state、attempt path に cmd 独自の state filesystem 実装が残っていない

## Phase 3: run orchestration の分離

**ステータス: 完了**

### 目的

run の計画、選択、retry、dependency wave、carry-forward、結果確定を `internal/run` に移し、cmd 側には flag 解釈、表示、exit code、server request の組み立てだけを残す。

### 完了した作業

- [x] filtered rerun、partial array、retry pending の計画を `internal/run` に移した
- [x] dependency wave と carry-forward planning を `internal/run` に移した
- [x] `ApplyCarriedOrigins` と `FinalizePendingResults` を追加した
- [x] array expansion と pure value transformation を model / run API に移した
- [x] load sample persistence を state、load average parsing を run に移した
- [x] progress callback を run lifecycle contract と CLI presentation に分離した
- [x] mixed run の execution wave と retry lifecycle を `ExecuteDependencyRetries` / `RunAttempt` に委譲した
- [x] run summary の構築と exit code 解決を `BuildRunSummary` に移した
- [x] run worker の context、sampling、finalize、lock cleanup 順序を `RunWorker` に整理した
- [x] rerun selection の pure logic を `PlanSelection` に集約し、cmd は state loading adapter に限定した

### 残作業

- [x] なし

### 完了条件

- `internal/run` が CLI の print や flag 型を import しない
- filtered rerun、partial array、carry-forward のテストが internal package にある
- cmd 側の run 処理が orchestration adapter として読める
- summary persistence は state、exit code / summary construction は run contract が担当する

## Phase 4: cmd 内のテスト責務整理

**ステータス: 完了**

### 目的

テストを CLI、server、run、Web の責務ごとに配置し、executor / model / Web の詳細テストを各 internal package に置く。

### 完了した作業

- [x] executor、model、Web の pure / unit contract tests を各 internal package へ移した
- [x] scheduler status persistence tests を executor package に移した
- [x] cmd の重複 `QueueToJobs` pure tests を削除した
- [x] `main_test.go` から mixed run integration test を `mixed_run_test.go` に分離した
- [x] fallback contract test を shared API に寄せた
- [x] server operation / command workflow tests を `server_test.go` に集約した
- [x] run lifecycle、retry wave、selection の contract tests を `internal/run` に配置した
- [x] `job_executor_test.go` は executor 詳細ではなく CLI dispatch / queue validation の integration test として整理した

### 残作業

- [x] なし

### 完了条件

- cmd 側のテスト名とファイルから CLI / server / run / Web の責務が判別できる
- executor package のテストだけで executor の詳細を検証できる
- mixed run integration test に重複定義がない
- cmd に残る executor 関連テストが CLI validation / dispatch の範囲に限定されている

## Phase 5: server / Web / CLI の境界整理

**ステータス: 完了**

### 目的

server protocol、command mapping、transport、Web projection、asset composition の境界を明確にする。

### 完了した作業

- [x] Web job / attempt / result projection と timeline を `internal/web` に移した
- [x] queue origin index を `internal/model` に移した
- [x] server operation vocabulary と known/unknown validation を `internal/server` に集約した
- [x] Web asset embed 宣言と JS assembly / template substitution を分離した
- [x] server request / response を `internal/server.Request` / `Response` に集約し、cmd は alias と transport orchestration に限定した
- [x] server operation dispatch は cmd の transport boundary に残し、domain contract と分離した
- [x] CLI / server / Web の fallback / result resolution を shared state / executor contract で検証した
- [x] `web.go` の projection と HTML / JS asset composition を `internal/web` / `web_assets.go` に分離した

### 残作業

- [x] なし

### 完了条件

- internal/server が transport-independent contract を所有する
- cmd が server transport と user-facing command mapping だけを担当する
- Web projection と CLI の表示整形が混在しない
- Web asset の embed / assembly と Web handler の transport が混在しない

## Phase 6: cmd ファイルの統合と最終整理

**ステータス: 完了**

### 目的

残った compatibility wrapper と責務の近い cmd ファイルを、依存関係を確認したうえで整理する。

### 完了した作業

- [x] `pathSet` を `internal/state.ProjectPaths` に統一した
- [x] status / environment / timestamp / result count の forwarding wrapper を削除した
- [x] shell quoting、timeline、project-name、path-element、validated join の wrapper を削除した
- [x] test-only state helper を production code から分離した
- [x] `QueueToJobs` を model API に統一した
- [x] lock loading / process liveness の pure forwarding wrapper を削除し、cmd caller を `internal/state` に統一した
- [x] `jsonStore`、`loadSlurmStatus`、`loadRunQueue`、`formatRunLabel` を監査し、mode injection / validation / user-facing formatting を持つ adapter として整理した
- [x] job projection、job origin、job result、run formatter の統合候補を依存関係に基づいて判断した

### 残作業

- [x] なし

### 完了条件

- cmd production code が CLI / orchestration / 表示に集中している
- production caller のない compatibility wrapper が test helper 以外に残っていない
- 無関係な責務を同じ cmd file に統合していない
- state / executor / model の adapter は、実際の mode・validation・表示責務を持つものだけが残っている

## Phase 7: cmd/rotari 内カーネルの分離(検討の記録)

**ステータス: 見送り**

### 背景

`cmd/rotari` 直下のファイル数を減らしたいという要望を受けて調査した。`cli_spec.go`・`config.go`・`color.go`・`state_lock.go`・`project_state.go`・`run_registry.go`・`registry.go`・`job_executor.go`・`environment.go` は、コマンド固有ではない共有基盤として `resolvePaths`(214箇所)、`printErrorf`(176箇所)、`cliString`(95箇所)など**cmd/rotari のほぼ全ファイルから合計900箇所超**参照されていることが分かった。

### 検討した案と見送った理由

これらを `cmd/rotari/internal/cliutil` に実体ごと移し、呼び出し側900箇所超を `cliutil.Xxx` に書き換える案を検討したが、以下の理由で見送った。

- 動作を一切変えない、純粋なファイル配置の変更であり、機能追加や不具合修正が楽になるといった具体的な効果がない
- 一方でコストは、このリポジトリが path/lock 安全性について特に神経質になっている領域にまで及ぶ900箇所超の書き換えであり、「段階的・検証可能な変更を優先し、大きな未検証の書き換えは避ける」という方針に反する
- 発端の要望も「ファイル数が多い」という見た目の違和感であり、具体的に困っている実害ではなかった

### 代わりに実施したこと

- `server_peercred_linux.go`/`server_peercred_other.go`(`verifyPeerCredential`)は他ファイルに依存しない自己完結コードだったため `internal/server`(`VerifyPeerCredential`)へ移動した
- `state_store.go` は `jsonStore`/`loadSlurmStatus`/`loadRunQueue` という3つの薄い転送関数のみで構成されていたため、`main.go` の `stateDirMode`/`stateFileMode` の隣に統合し、ファイルを削除した
- `internal/run` 等の小さいファイル群は、それぞれ単一責務を持つ実質的なロジックであり(`cancellation.go`/`options.go`/`summary.go`/`worker.go`/`local_lane.go` 等)、統合対象ではないと判断した

同様の大規模な package 再編を再検討する場合は、具体的な困りごと(バグ・テスト困難・機能追加のしにくさ)が先に存在することを確認してから着手すること。


## Compatibility wrapper の扱い

移行期間中の wrapper は許容するが、期限なしで残さない。

### 残してよい条件

- public behavior の一時的な維持に必要
- 移行先 API が未確定
- 段階的な test / integration 移行に必要

### 削除する条件

- production caller がない
- 単純な forwarding である
- test-only compatibility のためだけに残っている
- package 境界を読みにくくしている

## Issue 管理

契約かバグか判断が分かれる挙動は、勝手に仕様を決めず [../ISSUES.md](../ISSUES.md) に記録する。issue を `Resolved` に移すのは、仕様を決定し、実装とテストで確認した後にする。

## 検証手順

```sh
go test ./cmd/rotari -run TestName
go test ./internal/state -run TestName
go test ./cmd/rotari ./internal/executor ./internal/state ./internal/run ./internal/web ./internal/server ./internal/diagnose
go build ./cmd/rotari
gofmt -l cmd/rotari/*.go internal/**/*.go
git diff --check
go test ./...
go vet ./...
```

## 計画の完了条件

- cmd production code が CLI / orchestration / 表示に集中している
- executor 本体が `internal/executor` にある
- state persistence が `internal/state` にある
- run planning が `internal/run` にある
- CLI / server / Web の fallback 契約が一致している
- compatibility wrapper と executor 専用 cmd test file が整理されている
- open issue が仕様決定なしに削除されていない
- package 単位のテストと全体テストが通る
