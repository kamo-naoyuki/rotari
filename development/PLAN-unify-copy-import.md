# 計画: copy と import の挙動の統一

## 目的

- import で作った queue も、copy で作った queue と同じ判定で実行する。判定の実装は1本にする。
- Force(ジョブを変更すると前回の結果を捨てる仕組み)を廃止する。
- status の書き換えは、manifest でも `change --status` でもできるようにする。
- 実行するかどうかは、常に `run` のフィルタで決まるようにする。

## 決定事項

1. フィルタなしの `run` は、import した queue でも全ジョブを実行する。
2. Force は廃止する。command、env、working directory を変えても、前回の結果は残る。
3. フィルタ付きの `run`(`retry` と `-j` も含む)では、実行されるジョブの下流を、
   `--depends-on` と `--depends-on-finished` のどちらでもすべて再実行する。
4. status は success、failed、cancelled、unfinished のどれにでも書き換えられる。
   書き換えた status は、フィルタの判定と結果の引き継ぎに使う。
5. `import --dry-run` の表示は execute、reuse、accept をやめ、queue に入る status を表示する。

## 新しいデータモデル(state version 2)

- `QueuedCommand` から `Force`、`TaskForce`、`Accepted`、`TaskAccepted` を削除し、
  代わりに `MarkedStatus`(ジョブ全体)と `TaskMarkedStatus`(array のタスクごと)を追加する。
- `Queue.WorkflowImport` を削除する。
- 旧版(v1)のファイルは `state.ReadQueueFile` で読み込み時に変換する。
  - `force` → `unfinished`(`force` と `accepted` が両方あれば `force` を優先。今の判定と同じ)
  - `accepted` → `success`
  - `workflow_import` は捨てる
- 変換の対応表は contracts 04 の version policy の項に記録する。過去の run のファイルは書き換えない。
- run summary の `JobResult.Accepted` はそのまま残す。`success (accepted)` の表示に使っているため。

## status を書き換えたときの意味(`model.MarkResult`)

| 書き換え後の status | 判定と引き継ぎで使う結果 |
| --- | --- |
| なし、または実際と同じ | 記録されている結果をそのまま使う |
| `unfinished` | 結果なし。前回の結果は引き継がない |
| `success` | 終了コード 0 で `Accepted` を付ける(今の accept と同じ) |
| `failed` | 終了コードが 0 なら 1 にし、エラー文を `marked failed` にする |
| `cancelled` | 終了コードが 0 なら 1 にし、エラー文を `cancelled (marked)` にする(既存の cancelled 判定に合わせた文) |

- 結果が1つもないジョブを success、failed、cancelled にした場合は、plan を作る時点でエラーにする。
  今の accept と同じ扱い。
- 出どころ(`Origin`)は、書き換えても元の attempt を指したまま残す。

## コードの変更

### model

- `ResultStatus(result, finished)`、`IsCancelledError`、`ValidMarkedStatus`、`MarkedStatusOf`、
  `MarkResult`、`QueuedStatusText`(queue 表示用)を追加する。
- `workflow/export.go` の `resultStatus` と、`run/cancellation.go` の cancelled 判定を、
  上の関数を使う形にまとめる。

### state

- `ReadQueueFile` で v1 を変換する。

### run/rerun.go

- `planImportedWorkflow`、`applyImportedCommandPlan`、`acceptImportedResult`、
  `expandImportedDownstream` を削除する。
- `jobResult` は、Force を見る代わりに `MarkResult` を通す。
- `expandFinishedDownstream` を `expandDownstream` にし、すべての依存を対象にする。

### queueops/change.go

- Force と WorkflowImport の処理を削除する。
- `Mutation` に `Status` と `ClearStatus` を追加する。array には全体に設定し、タスクごとの status は消す。

### queueops/add.go、queueedit/copy.go、project/edit.go、state/run_files.go、cmd/rotari/reset.go

- Force と WorkflowImport に関わる処理を削除する。
- copy では `MarkedStatus` を消す。書き換えた status は、それが使われた run の結果に
  すでに反映されているため。

### workflow/reconcile.go

- Force と Accepted を書く代わりに、manifest の status が実際と違うときだけ `MarkedStatus` を書く。
- 変更されたジョブも、元のジョブと対応が付けば出どころを残す(Force 廃止に合わせる)。
- 新しいジョブには何も書かない(出どころがないので自然に unfinished になる)。

### cmd/rotari

- `change` に `--status STATUS` と `--clear-status` を追加する。`cli_spec` にも登録する。
- `import --dry-run` の表示を status にする。`--json` の plan は、`action` を `status` に変えて
  version を 2 に上げる。
- `show` の queue 表示にある SOURCE STATUS 列に、書き換えた status を反映する(`QueuedStatusText`)。

### Web

- queue 表のジョブの `Source status` に、書き換えた status を反映する(JS の小さな修正)。
- `/api/change` は今も timeout や retry を扱っていないので、status も追加しない。

## ドキュメントと contract

- contracts 02: Force の項、import の判定、下流の再実行、
  「import のジョブは参照 run を見ない」の項を書き換える。
- contracts 04: state version 2 の変換を記録する。
- contracts/README の Contract status 表を、実際の conformance テストに合わせる。
- docs: RUNNING.md(change の節、run と retry の節)、WORKFLOW_MANIFESTS.md、
  CONCEPTS.md(依存の節)、FAQ.md、CLI_REFERENCE.md(`scripts/generate_cli_reference.py` で再生成)。
- `conformance/testdata/golden/schema.json` を再生成する。

## テスト

- 単体テスト: `run/plan_test`(書き換えた status、下流の再実行、import した queue も
  フィルタなしなら全実行)、`queueops` の change、`workflow/reconcile_test`、`state` の v1 変換、
  `model.MarkResult`、import の plan 表示。
- conformance: `TestSelectorTable` の "changed" の行(Force が前提)を新しい contract に合わせて直す。
  これは contract 自体の変更なので、書き換えてよいケース。
- 実行順: 関係するパッケージのテスト → `scripts/check.sh --short` → `go test ./conformance`
  → `scripts/check.sh`。

## コミットの分け方

1. 判定の統一と Force の廃止(model、state、run、reconcile、import の表示、contract と docs)
2. `change --status` の追加
3. 下流の再実行をすべての依存に広げる

作業ツリーには、別の作業による ISSUES.md などの変更がある。コミットにはそれらを含めない。

## 今回やらないこと

- fingerprint による対応付け。この統一が終わった後に、別の作業として行う。

## 進捗

- [ ] model: フィールドの置き換えと `StateVersion = 2`
  (着手済み。`internal/model/model.go` のみ変更。まだビルドは通らない)
- [ ] model: status 関連の関数
- [ ] state: v1 の変換
- [ ] run: 判定の統一と `MarkResult`
- [ ] queueops、queueedit、project、reset、add
- [ ] workflow/reconcile
- [ ] cmd/rotari: change、import の表示、show
- [ ] Web の表示
- [ ] 下流の再実行
- [ ] テスト
- [ ] contract と docs
- [ ] check.sh と conformance
