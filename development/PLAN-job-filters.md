# 計画: ジョブフィルターの一般化

## 位置づけ

現在の rotari がジョブを結果で選ぶ手段は、終了状態の result filter（`--failed`、`--unfinished`、`--success`）だけである。
この計画では、ジョブを選ぶ条件を「述語の集合」として一般化し、次を追加する。

- 結果の細分化（exit code、失敗の種類）
- rule-based diagnosis による分類
- command の正規表現マッチ
- 実行ホスト、開始・終了時刻、継続時間
- ジョブ定義の変更（fingerprint）
- 宣言した入出力ファイルの鮮度（make 的な判定）
- 汎用の否定 `--not`
- `cancel`、`suspend`、`resume` へのフィルター適用

対象は CLI と、CLI の schema から生成される Python client である。
Web UI / Web API への反映は別計画とする。
cron scheduling は将来の検討事項とし、この計画では扱わない。

## 現状

### ジョブを選ぶオプションの種類

[contracts/06-selectors.md](../contracts/06-selectors.md) の SEL-8 は、ジョブを選ぶオプションを 3 種類に分けている。

| 種類 | オプション | 組み合わせ |
| --- | --- | --- |
| Direct | `--job-id`、`--job-name`、位置引数の job ID | 名指ししたジョブだけ |
| Result filter | `--failed`、`--unfinished`、`--success` | 互いに OR |
| Scope | `--stage`、`--matrix` | result filter を AND で絞る |

Direct selector は result filter と scope のどちらとも組み合わせられない。

### 実装

- result filter は comma 区切りの文字列として扱われる。
  - [internal/model/selection.go](../internal/model/selection.go) の `ResultSelection` と `ResultSelectionMatches`
- scope は [internal/model/command_selector.go](../internal/model/command_selector.go) の `CommandSelector` である。
- 使用箇所:
  - `show`: [cmd/rotari/show.go](../cmd/rotari/show.go) の `showJobFilter`
  - `run`、`retry`: [cmd/rotari/project_run.go](../cmd/rotari/project_run.go) の `planRerunSelection` から [internal/run/rerun.go](../internal/run/rerun.go) の `PlanRerun`。`expandDownstream` が実行するジョブの下流を追加する。
  - `copy`: [internal/queueedit/copy.go](../internal/queueedit/copy.go) の `CopyRequest.Selection` と `Scope`
  - Web: [internal/webui/webui.go](../internal/webui/webui.go) の `/api/copy` が `selection` を受け付ける。
- ジョブ制御:
  - CLI: [cmd/rotari/job_control.go](../cmd/rotari/job_control.go) の `cmdCancel`、`cmdJobSignal`
  - 解決: [internal/resolve/resolve.go](../internal/resolve/resolve.go) の `JobSelection`
  - 実行: [internal/jobcontrol/jobcontrol.go](../internal/jobcontrol/jobcontrol.go) の `Controller.Cancel`、`CancelJobs`、`Control`、`expandArrays`
  - 選択を何も指定しないと、`cancel` は run 全体を cancel する。
  - 確認プロンプトは [cmd/rotari/copy.go](../cmd/rotari/copy.go) の `confirmQueueOverwrite` にしかない。
- CLI の flag は Go 標準の `flag` package に [cmd/rotari/cli_spec.go](../cmd/rotari/cli_spec.go) の `cliBool`、`cliString`、`cliChoiceValue` を重ねたもの。Python client は [cmd/rotari/schema.go](../cmd/rotari/schema.go) の schema から `scripts/generate_python_cli.py` で生成される。
- 時刻の解釈は [internal/joblist/joblist.go](../internal/joblist/joblist.go) の `ParseSince`（Go の duration）しかない。

### フィルターが使えるデータ

- `JobResult`（[internal/model/model.go](../internal/model/model.go)）: `ExitCode`、`Error`、`Command`、`Hosts`、`Diagnoses`、`DiagnosisStatus`、`DiagnosisRules`
- `JobOrigin`: 引き継いだジョブの元の `RunID`、`JobID`、`SubmittedAt`、`FinishedAt`
- attempt ごとのファイル:
  - `submitted_at`、`finished_at`
  - `status.json`（[internal/executor/wrapper_status.go](../internal/executor/wrapper_status.go) の `WrapperStatus`）: `started_at` と `hosts`。すべての executor の wrapper が書く。
  - `scheduler_status.json`: scheduler の state（`timeout`、`out_of_memory`、`cancelled` など）
- 読み出し:
  - [internal/state/attempts.go](../internal/state/attempts.go) の `ReadJobTimestamp`
  - [internal/jobstatus/times.go](../internal/jobstatus/times.go) の Origin への fallback
- 失敗の種類の手がかり:
  - timeout: `executor.TimeoutExitCode`（124）
  - cancel: `model.IsCancelledError`
  - blocked: `jobstatus.Job.Blocked`
  - oom、timeout: scheduler の state
- diagnosis:
  - [internal/diagnose/default_rules.go](../internal/diagnose/default_rules.go) のルールには表示名（`Name`）しかなく、安定した ID がない。
  - `diagnose.Diagnose(errorText, log, rules)` でログから再計算できる。
  - `RulesVersion` はルール集合のハッシュである。
- fingerprint:
  - [internal/model/fingerprint.go](../internal/model/fingerprint.go) の `Fingerprint`、`MatchFingerprintJobsByMode`
  - [internal/projectrun/origins.go](../internal/projectrun/origins.go) の `MatchFingerprintQueue`、`FingerprintReferenceRun`
- 依存関係: `QueuedCommand.DependsOn`、`DependsOnFinished`

## 決定事項

### 組み合わせ

- 同じオプションを繰り返したときは、肯定値の OR をとる。
- 否定値（後述の `--not`）は、そのすべてに一致しないことを条件とする（AND NOT）。
  - 例: `--diagnosis a --diagnosis b --not --diagnosis c` は「(a または b) かつ c でない」。
- 異なるオプションどうしは AND で組み合わせる。
- result filter どうしは従来どおり OR。
- Direct selector は、この計画で追加するものを含むすべてのフィルターと排他にする（SEL-8 の延長）。
- 時刻と継続時間のオプションは繰り返せない。繰り返すとエラーにする。

### 属性がないジョブ

属性が定義されないジョブは、肯定の条件にも否定の条件にも一致しない。

- success のジョブには diagnosis がないので、`--diagnosis oom` にも `--not --diagnosis oom` にも一致しない。`--not --diagnosis oom` は、OOM と診断されなかった**失敗**ジョブだけを選ぶ。
- 投入されていないジョブには host と開始時刻がないので、`--host` にも時刻の条件にも一致しない。
- 出力を宣言していないジョブは、`--outdated` にも `--missing-output` にも一致しない。

### どの attempt で判定するか

- 最新の attempt で判定する。ジョブの結果と同じ attempt である。
- 参照 run で実行されず引き継いだジョブは、元の run（`JobOrigin`）の attempt の時刻、host、ログで判定する。

### 否定

- 汎用の `--not` が、直後の 1 つの条件を反転する。
  - 例: `--not --stage prepare`、`--not --diagnosis oom`
- 同じ意味の `--not-<name>` 形式も受け付ける。
  - 例: `--not-stage prepare`、`--not-diagnosis oom`
  - help と CLI reference には `--not` と `--not-<name>` の両方を載せる。
  - Python client は `--not-<name>` を `not_<name>=` として使う。kwargs では順序に依存する `--not` を表せないためである。
- `--not` を付けられるのは、この計画で追加する属性フィルターと、既存の `--stage`、`--matrix` である。
  - result filter と時刻・継続時間のオプションには付けられない。後者は after / before などの対があるため不要である。
- `--not` の直後が否定できないオプションか、引数の末尾ならエラーにする。

### 結果の細分化

- `--exit-code N`
  - 終了したジョブの、解決済みの exit code に一致する。繰り返すと OR。
- `--failure-kind KIND`
  - KIND は `timeout`、`cancelled`、`blocked`、`oom`、`signal`、`error`。
  - 失敗したジョブだけが対象。1 つのジョブが複数の種類に該当してよい（例: `signal` かつ `oom`）。
  - 判定:

    | 種類 | 判定 |
    | --- | --- |
    | `timeout` | exit code 124、または scheduler の state が timeout |
    | `cancelled` | `model.IsCancelledError`、または scheduler の state が cancelled |
    | `blocked` | `jobstatus.Job.Blocked` |
    | `oom` | scheduler の state が `out_of_memory` / `oom` |
    | `signal` | exit code が 128 を超える |
    | `error` | 上のどれにも該当しない失敗 |

  - 判定する関数は `internal/jobstatus` に置き、`show` の表示と共有できるようにする。

### diagnosis

- `--diagnosis VALUE` は、**現行のルール**で最新 attempt のログから再計算した結果で判定する。
  - run に保存された diagnosis は使わない。
- 失敗したジョブだけが対象。
  - ログがなく診断できない（unavailable）ジョブと、どのルールにも一致しない（no_match）ジョブは、属性がないものとして扱う。
  - no_match と unavailable を選ぶ値は用意しない。
- 各ルールに安定した slug ID（例: `cuda-oom`、`python-import`）を追加する。
  - ID は `RulesVersion` のハッシュに含めない。ルールの照合内容は変わらないためである。
  - ID が一意であることを単体テストで確かめる。
  - [docs/LOCAL_DIAGNOSIS.md](../docs/LOCAL_DIAGNOSIS.md) の表に ID の列を加える。
- VALUE の照合:
  1. slug ID の完全一致を優先する。
  2. 一致しなければ、表示名に大文字小文字を区別せず部分一致させる。
  3. 複数のルールに一致した場合は、それらの OR とする。
  4. どのルールにも一致しなければエラーにする。

### command の正規表現

- `--command-regex RE`
  - Go の `regexp`（RE2）で、argv を空白で連結した文字列に照合する。
- ジョブの定義だけを見る条件なので、`show`、`copy`、`run`、`retry` に加えて `change`、`remove` でも使える。
  - `change` と `remove` では `--stage`、`--matrix` と AND で組み合わせる。`--all` と job selector とは排他にする。
  - `--stage` と `--matrix` が互いに排他であることは変えない。

### host

- `--host PATTERN`
  - glob（`path.Match`）で、最新 attempt の hosts のどれかに一致すればよい。

### 時刻と継続時間

- 使う時刻:
  - 開始: `status.json` の `started_at`。なければ `submitted_at`。
  - 終了: `finished_at`
  - 継続時間: 終了 − 開始。実行中のジョブは「現在時刻 − 開始」。
- オプション:
  - `--started-after`、`--started-before`
  - `--finished-after`、`--finished-before`
  - `--longer-than`、`--shorter-than`
- 境界: after と longer-than は「以上」、before と shorter-than は「未満」。
- 時刻の書式:
  - 絶対時刻: `2006-01-02`、`2006-01-02T15:04`、`2006-01-02T15:04:05`、RFC3339。タイムゾーンを省略するとローカル時刻。
  - 相対時刻: Go の duration に `d`（24 時間）を加えたもの。現在時刻から遡った時刻を表す（例: `--finished-after 2d`）。
- 現在時刻は、コマンドの開始時に 1 回だけ固定する。
- 時間帯を表す `--active-during` は設けない。「22:00〜02:00 に動いていた」は `--started-before` と `--finished-after` の組み合わせで書く。

### ジョブ定義の変更

- `--changed`
  - 参照 run に対応するジョブがあり、fingerprint が異なるジョブ。
- `--new`
  - 参照 run に対応するジョブがないジョブ。
- 対応付けと参照 run の決め方は、既存の `--match-by` と `FingerprintReferenceRun` に従う。
- 使えるコマンドは `run`、`retry`、`show`（queue の表示）。
  - `copy` は run から選ぶコマンドなので対象外。

### 入出力ファイルの鮮度

- `add --input PATH` と `add --output PATH` で、ジョブの入出力を宣言する。
  - 繰り返し指定できる。
  - `change` でも編集できる。編集の書式は、既存の `change --env` の形に合わせる。
- パスの解釈:
  - ジョブの working directory からの相対パス
  - `${VAR}` を、明示された env、matrix の値、`ROTARI_ARRAY_TASK_ID` で展開する。
  - 展開後に glob を適用する。
- 判定は mtime で行う（make と同じ）。
  - `--outdated`: 出力が 1 つも存在しない、または最も古い出力より新しい入力がある。
  - `--missing-output`: 宣言した出力のどれかが存在しない。
  - 入力が 0 件（パスが存在しない、または glob が何にも一致しない）ならエラーにする。
- 評価は呼び出し元のホストで行う。
  - SSH や scheduler の executor では、ファイルが共有 FS で見えることが前提となる。この前提は文書に明記する。
- 入出力の宣言は fingerprint に含める。
  - 宣言が空のジョブでは含めない。こうすると既存の fingerprint は変わらない。
- 入出力の宣言から依存関係を推論することはしない。宣言はジョブを選ぶためだけに使う。
  - [TODO.md](TODO.md) で「ジョブ間の出力受け渡し」をスコープ外にしているのと整合させる。

### cancel、suspend、resume

- 受け付けるフィルター:
  - `--stage`、`--matrix`、`--job-name`
  - `--command-regex`、`--host`
  - `--started-after`、`--started-before`、`--longer-than`、`--shorter-than`
  - `--pending`、`--running`（互いに OR）
  - `--not`
- `suspend` と `resume` は実行中のジョブしか扱わないので、`--pending` を受け付けない。
- 一致が 0 件ならエラーにする。run 全体の cancel にはしない。
- 確認:
  - TTY では対象の一覧を表示して確認する。
  - `--yes` で確認を省略する。
  - TTY でないときは `--yes` を必須にする。
- 確認の前に評価した集合を、そのまま `CancelJobs` に渡す。確認の後で評価し直さない。
- フィルターを指定しないときの既存の動作は変えない。
- `cancel --wait` はフィルターと排他にする。

### 各コマンドで使えるフィルター

| フィルター | `show` | `copy` | `run`、`retry` | `change`、`remove` | `cancel` | `suspend`、`resume` |
| --- | --- | --- | --- | --- | --- | --- |
| result filter（既存） | ○ | ○ | ○ | – | – | – |
| `--stage`、`--matrix`（否定を追加） | ○ | ○ | ○ | ○ | ○ | ○ |
| `--job-name`（Direct） | ○ | ○ | ○ | ○ | ○ | ○ |
| `--command-regex` | ○ | ○ | ○ | ○ | ○ | ○ |
| `--exit-code`、`--failure-kind` | ○ | ○ | ○ | – | – | – |
| `--diagnosis` | ○ | ○ | ○ | – | – | – |
| `--host` | ○ | ○ | ○ | – | ○ | ○ |
| `--started-*`、`--longer-than`、`--shorter-than` | ○ | ○ | ○ | – | ○ | ○ |
| `--finished-*` | ○ | ○ | ○ | – | – | – |
| `--changed`、`--new` | ○（queue の表示） | – | ○ | – | – | – |
| `--outdated`、`--missing-output` | ○ | ○ | ○ | – | – | – |
| `--pending`、`--running` | – | – | – | – | ○ | `--running` のみ |

`show` は、run の結果を見るフィルターが指定されると、`--failed` と同じように queue を読み飛ばす。

## 設計

### `internal/jobfilter` package

フィルターの評価を新しい package `internal/jobfilter` にまとめ、`show`、`run.PlanRerun`、`queueedit.Copy`、`jobcontrol` が同じ評価を使うようにする。

```text
cmd/rotari ──> internal/run, internal/queueedit, internal/jobcontrol ──> internal/jobfilter ──> internal/model
                                   │
                                   └──> jobstatus / state の adapter が jobfilter.Facts を供給する
```

- `Filter`
  - result filter、scope、述語のリストを持つ。
  - 述語ごとに肯定値と否定値を持つ。
  - 評価の前に組み合わせを検証する（Direct selector との排他、繰り返しの禁止など）。
- `Facts`
  - ジョブ単位（array のタスク単位）の属性。
  - 定義（command、env、working directory、stage、matrix、入出力の宣言）
  - 結果（finished、exit code、失敗の種類）
  - 実行（hosts、開始・終了時刻、実行状態）
  - 高価な属性（diagnosis の再計算、ファイルの stat）は遅延して読み込む。
- 評価の順序: 安い述語 → diagnosis → ファイル。
- `jobfilter` は `state` や `executor` に依存しない。`Facts` は `jobstatus` と `state` の側の adapter が作る。

既存の `model.ResultSelection`、`ResultSelectionMatches`、`CommandSelector` は `jobfilter` に移すか、`jobfilter` を呼ぶ薄い wrapper にする。
`CopyRequest.Selection` などの文字列 selection は `jobfilter.Filter` に置き換える。
Web の `/api/copy` はこの計画では変えず、今の `selection` の値を adapter で `Filter` に変換する。

### CLI の否定

- `--not` を、次に解析される条件に反転を伝える共有状態を持つ `flag.Value` として実装する。
- 否定できる各オプションに対応する `--not-<name>` を自動で登録する。
- Go の `flag` package はオプションを出現順に `Set` するので、順序に依存する `--not` を実装できる。
  - ただし、位置引数を並べ替える処理がある場合に出現順が保たれるかを、Phase 1 の最初に確認する。
  - 保たれない場合は、`--not-<name>` の形だけにするかを相談する。

## 実装の手順

各 phase は独立してコミットし、コミットごとにテストが通る状態を保つ。

### Phase 1: 土台（外から見える動作は変えない）

1. `internal/jobfilter` を新設し、`Filter`、`Facts`、評価関数、組み合わせの検証を実装する。
2. `showJobFilter`、`PlanRerun`、`CopyRequest`、`planRerunSelection` を `jobfilter.Filter` に置き換える。
3. `--not` と `--not-<name>` の仕組みを `cli_spec.go` に追加し、`--stage` と `--matrix` の否定を有効にする。
4. `go test ./conformance` が、テストを変更せずに通ることを確認する。

### Phase 2: 定義系

5. `--command-regex` を追加する（`show`、`copy`、`run`、`retry`、`change`、`remove`）。

### Phase 3: 結果の細分化

6. `jobstatus` に失敗の種類を判定する関数を追加する。
7. `--exit-code` と `--failure-kind` を追加する。

### Phase 4: 実行の属性

8. `started_at` を読む関数を `state` か `jobstatus` に追加する（Origin への fallback を含む）。
9. 時刻の解釈を `jobfilter` に実装する。
10. `--host`、`--started-*`、`--finished-*`、`--longer-than`、`--shorter-than` を追加する。

### Phase 5: diagnosis

11. ルールに slug ID を追加する。
12. `--diagnosis` を追加する（現行のルールで再計算する）。

### Phase 6: ジョブ定義の変更

13. `--changed` と `--new` を追加する。

### Phase 7: ジョブ制御

14. `cancel`、`suspend`、`resume` にフィルター、`--pending`、`--running`、`--yes`、確認プロンプトを追加する。

### Phase 8: 入出力の宣言

15. `QueuedCommand` と `JobSpec` に入出力を追加し、`add` と `change` のオプション、fingerprint への反映を実装する。
16. `--outdated` と `--missing-output` を追加する。

### 各 phase で行うこと

- [contracts/06-selectors.md](../contracts/06-selectors.md) を更新する。
  - 属性フィルターの kind、組み合わせの規則、属性がないジョブの規則を書く。
  - 新しいルールに SEL-10 以降の ID を振る。
- [contracts/README.md](../contracts/README.md) の Contract status の表を、conformance テストの `covers` 呼び出しと一致させる。
- 利用者向けの文書を更新する。
  - [docs/RUNNING.md](../docs/RUNNING.md): フィルターの使い方
  - [docs/LOCAL_DIAGNOSIS.md](../docs/LOCAL_DIAGNOSIS.md): slug ID
  - [docs/ARCHITECTURE.md](../docs/ARCHITECTURE.md): `internal/jobfilter`
  - 必要に応じて [docs/FAQ.md](../docs/FAQ.md)、[docs/CONCEPTS.md](../docs/CONCEPTS.md)
- 生成物を再生成する。
  - `scripts/generate_cli_reference.py`（[docs/CLI_REFERENCE.md](../docs/CLI_REFERENCE.md)）
  - `scripts/generate_python_cli.py`（Python client）
- この計画で扱った項目を [TODO.md](TODO.md) から取り除き、スコープ外の記述を見直す。

## テスト

- `internal/jobfilter` の単体テスト
  - 組み合わせ（OR、AND、否定）
  - 属性がないジョブが肯定・否定のどちらにも一致しないこと
  - 時刻の解釈と境界
- [internal/run/plan_test.go](../internal/run/plan_test.go) と [internal/queueops/copy_test.go](../internal/queueops/copy_test.go) に、新しいフィルターによる execute と carry の判定を追加する。
- conformance
  - [conformance/06-selectors/selector_cases_test.go](../conformance/06-selectors/selector_cases_test.go) に行を追加し、`covers(t, "SEL-n")` で契約の ID と対応させる。
  - [conformance/06-selectors/job_control_test.go](../conformance/06-selectors/job_control_test.go) に、フィルター付きの cancel、確認、`--yes` のテストを追加する。一致が 0 件のときに run 全体を cancel しないことのテストは必須。
- `--outdated` と `--missing-output` は、一時ディレクトリで mtime を操作する単体テストで確認する。
- 仕上げに pre-commit、`scripts/check.sh --short`、`scripts/check.sh` の順に実行する。

## スコープ外

- Web UI / Web API へのフィルターの反映（別計画）
- cron scheduling
- `--active-during`
- 入出力の宣言からの依存関係の推論
- result filter と時刻・継続時間のオプションの否定
- 保存済みの diagnosis による判定
- ユーザー定義の述語コマンド（`--where-cmd`）。ジョブごとにシェルコマンドを実行して exit code で選ぶ案だったが、quoting がわかりにくく、ジョブ数だけプロセスを起動して遅く、入出力の宣言と用途が重なるため見送った。
