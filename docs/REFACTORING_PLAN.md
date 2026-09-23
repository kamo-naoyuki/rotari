# Rotari リファクタリング計画

## 目的

`cmd/rotari` に CLI、サーバー、Web、状態永続化、実行基盤、ドメイン処理が混在しており、機能を探すために多数のファイルを横断する必要がある。

このリファクタリングの目的は、単に関数を短くすることではない。

- `cmd/rotari` を CLI と orchestration の薄い層にする
- ドメイン処理、状態管理、executor 実装を `internal` 配下に移す
- 同じ責務の小ファイルを適切な package 内へ統合する
- CLI、server、Web が同じ状態解決・結果解決の契約を共有する
- 移行途中の compatibility wrapper を最終的に減らす

## 現状

2026-09-23 時点で、executor 実装の移行を開始済み。

- `cmd/rotari` の production Go ファイル: 37
- `internal/executor` の Go ファイル: 17
- scheduler executor 本体: `internal/executor` へ移行済み
- executor 単体テスト: `internal/executor` へ移行済み
- cmd に残る executor テスト: server/run の統合テストのみ
- `internal/state`、`internal/model`、`internal/run`、`internal/web`、`internal/server`、`internal/diagnose` は既に存在

現在の変更は未コミットであり、この文書の数値は目安として扱う。ファイル統合を進める際は、必ず実際の tree とテストを確認する。

## 目標 package 構成

```text
cmd/rotari/
  CLI parsing and command dispatch
  server/web command orchestration
  user-facing formatting and exit codes
  integration tests for command workflows

internal/model/
  domain types
  queue/job/result rules
  dependency and environment validation
  pure value transformations

internal/state/
  filesystem layout and path safety
  JSON persistence and file modes
  locks and run/project state
  attempt directory resolution
  context, metadata, queue, summary accessors

internal/run/
  run contracts and options
  run selection and carry-forward planning
  environment preparation
  run lifecycle orchestration that is independent of CLI output

internal/executor/
  local executor
  Slurm/PBS/LSF/SSH executors
  scheduler option parsing
  wrapper scripts and scheduler status
  executor-specific metadata and process control

internal/server/
  server protocol and transport-independent server contracts

internal/web/
  web projection models and web-facing contracts

internal/diagnose/
  diagnosis rules and provider adapters
```

## 守る依存方向

依存方向は、外側から内側へ向ける。

```text
cmd/rotari
   |
   +--> internal/server
   +--> internal/web
   +--> internal/run
   +--> internal/executor
   +--> internal/state
   +--> internal/model

internal/run      --> internal/executor, internal/state, internal/model
internal/executor --> internal/state, internal/model
internal/web      --> internal/state, internal/model
internal/server   --> internal/model
internal/state    --> internal/model
```

`internal` package が `cmd/rotari` を import してはいけない。特に次のものを内側の package から参照しない。

- CLI の `pathSet`
- CLI の flag 型
- CLI の exit code や print helper
- `cmd` package の executor registry
- `testing.Testing()` による production 動作の切り替え

## 移行フェーズ

### Phase 1: executor 境界の完成

完了済みまたは実施中。

- scheduler executor 本体を `internal/executor` へ移す
- `state.Store` と logger を constructor から注入する
- scheduler wrapper と `status.json` の読み書きを executor package に集約する
- scheduler option の shell parser と command error hint を executor package に置く
- executor 単体テストを `internal/executor` へ移す
- `cmd/rotari` に残すのは registry、CLI validation、server/run integration のみにする

完了条件:

- `cmd/rotari/executor_lsf.go` などの executor 本体ファイルが存在しない
- executor 単体テストが `internal/executor` にある
- `go build ./cmd/rotari` が通る
- `go test ./cmd/rotari ./internal/executor` が通る

### Phase 2: cmd 内のテスト責務整理

- executor 専用の cmd test file を削除する
- server 操作のテストを `server_test.go` に集約する
- run orchestration のテストを `main_test.go` または run 用の既存 test file に集約する
- Web projection のテストは `web_test.go` に残す
- helper の定義場所を、利用責務に合わせて整理する

完了条件:

- `cmd/rotari/executor*_test.go` が存在しない
- cmd 側のテスト名から、CLI/server/run/web のどの責務か判別できる
- executor package のテストだけで executor の詳細を検証できる

### Phase 3: state 層の完成

- `cmd/rotari/state_store.go` の読み書き wrapper を縮小する
- `state.WriteJSON` の shared/private mode 契約を決める
- `LoadMeta`、`LoadQueue`、`LoadRunSummary`、`LoadContext` のエラー契約を文書化する
- lock、attempt path、project state の実装を `internal/state` に揃える
- CLI/server/Web で同じ fallback chain を使う

関連する未解決事項は [../ISSUES.md](../ISSUES.md) に残す。契約を変更するときは CLI、server、Web の全経路とテストを確認する。

### Phase 4: run orchestration の分離

- `mixed_run.go` の実行計画・carry-forward・環境準備を `internal/run` へ移す
- cmd 側には flag の解釈、進捗表示、exit code、server request の組み立てだけを残す
- `run_selection.go` の pure な選択ロジックを `internal/run` または `internal/model` に集約する
- `run_context.go` の load/save と load sampling の責務を分離する

完了条件:

- `internal/run` が CLI の print や flag 型を import しない
- filtered rerun、partial array、carry-forward のテストが internal/run にある
- cmd 側の run 処理が orchestration adapter として読める

### Phase 5: server / Web / CLI の境界整理

- server protocol は `internal/server` に置き、cmd は transport と command mapping を担当する
- Web projection の計算は `internal/web` に集約する
- CLI の表示整形と Web の JSON projection を混ぜない
- server、CLI、Web で状態や結果の fallback chain を変更しない
- `web.go` が持つ巨大な HTML/JS 文字列は、既存の asset 生成・埋め込み境界に合わせて整理する

### Phase 6: cmd ファイルの統合と最終整理

責務が近い小ファイルを、同じ package 内で統合する。単にファイル数を減らすために無関係な責務を混ぜない。

統合候補:

- executor registry と executor settings
- path と attempt path helper
- job projection、job origin、job result helper
- state store の薄い compatibility 部分
- run の表示 formatter と wait の adapter

統合しないもの:

- CLI flag parsing と domain logic
- server protocol と Web projection
- executor 本体と CLI command dispatch
- state persistence と user-facing formatting

## Compatibility wrapper の扱い

移行期間中の wrapper は許容するが、期限なしで残さない。

wrapper を残す条件:

- public behavior を一時的に維持する必要がある
- 移行先 package の API がまだ安定していない
- テストや外部 integration の段階的移行に必要

wrapper を削除する条件:

- production caller がなくなった
- wrapper が単純な 1 行 forwarding になっている
- test-only compatibility のためだけに残っている
- package 境界を読みにくくしている

## Issue 管理

リファクタリング中に、契約かバグか判断が分かれる挙動を見つけた場合は、勝手に仕様を決めず [../ISSUES.md](../ISSUES.md) に記録する。

記録する内容:

- 観測した現在の挙動
- どの caller / package に影響するか
- 意図的な契約である可能性
- バグまたは情報隠蔽である可能性
- 判断や修正に必要な追加確認

issue を `Resolved` に移すのは、仕様を決定し、実装とテストで確認できた後にする。

## 検証手順

小さい変更では、次の順で確認する。

```sh
go test ./cmd/rotari -run TestName
go test ./internal/executor -run TestName
go test ./cmd/rotari ./internal/executor ./internal/state ./internal/run ./internal/web ./internal/server ./internal/diagnose
go build ./cmd/rotari
gofmt -l cmd/rotari/*.go internal/**/*.go
git diff --check
```

広い変更では、最後に次を実行する。

```sh
go test ./...
go vet ./...
```

socket を使う server test や sandbox の Go cache 問題がある場合は、[AGENTS.md](../AGENTS.md) と [build-test-notes.md](/memories/repo/build-test-notes.md) の指示に従う。

## 完了の定義

この計画は、次の状態になったとき完了とする。

- cmd の production code が CLI/orchestration/表示に集中している
- executor 本体が `internal/executor` にある
- state persistence が `internal/state` にある
- run planning が `internal/run` にある
- CLI/server/Web の fallback 契約が一致している
- compatibility wrapper と executor 専用 cmd test file が整理されている
- Open issue が仕様決定なしに削除されていない
- package 単位のテストと全体テストが通る
