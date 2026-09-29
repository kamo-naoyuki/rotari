# 計画: ジョブフィルターの強化

## 位置づけ

現在の rotari がジョブを結果で選ぶ手段は、終了状態の result filter（`--failed`、`--unfinished`、`--success`）だけである。
この計画は**フィルター機能の強化**である。ジョブを選ぶ条件を「述語の集合」として一般化し、次を追加する。
どのジョブを実行するかは引き続き利用者が選び、rotari が自動で決めたり、依存関係を推論したりはしない。

- 結果の細分化（exit code、失敗の種類）
- rule-based diagnosis による分類
- command の正規表現マッチ
- 実行ホスト、開始・終了時刻、継続時間
- ジョブ定義の変更（fingerprint）
- 宣言したファイル（`--require-file`、`--produce-file`）の鮮度（make 的な判定）
- 否定（`--filter-not-*`）
- `cancel`、`suspend`、`resume` へのフィルター適用

フィルターのオプションはすべて `--filter-*` という名前にする。

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

### オプションの命名

- フィルターのオプションはすべて `--filter-<key>` という名前にする。
  - 将来 `--host` や `--timeout` のような設定用のオプションを足しても衝突しない。
  - 接頭辞で、フィルターのオプションだとひと目でわかる。
  - `>` や `!` を使わないので、シェルで quote する必要がない。
  - 普通の flag なので、既存の補完と `--help` の仕組みがそのまま使える。
  - Python client では `filter_exit_code=3` のように kwargs で書ける。
- 否定は `--filter-not-<key>` とする。汎用の `--not` は設けない。
  - 例: `--filter-not-host node12`、`--filter-not-diagnosis cuda-oom`
  - Python client では `filter_not_host=`。
- 既存の result filter と scope にも `--filter-*` の形を用意し、既存のオプションは省略形として残す。これで「フィルターはすべて `--filter-*` で書ける」という規則に例外がなくなる。

  | 省略形（既存） | `--filter-*` |
  | --- | --- |
  | `--failed`、`--unfinished`、`--success` | `--filter-result failed\|unfinished\|success` |
  | `--stage NAME` | `--filter-stage NAME` |
  | `--matrix NAME` | `--filter-matrix NAME` |

  - `--filter-not-stage` と `--filter-not-matrix` は `--filter-*` の形にだけ置く。
  - 省略形と `--filter-*` の形を両方指定したときは、同じオプションを繰り返したものとして扱う。
- Direct selector（`--job-id`、`--job-name`）はフィルターではないので、名前を変えない。

### help

- `--help` では、`--filter-*` を「Filters」という見出しにまとめて表示する。そのコマンドで使えるものだけを載せる。
  - [cmd/rotari/cli_spec.go](../cmd/rotari/cli_spec.go) にオプションをグループ分けする仕組みがなければ追加する。
- CLI reference と shell 補完は、同じ CLI spec から生成されるので自動的に対応する。
  - CLI reference でも Filters をまとめて表示する。

### 組み合わせ

- 同じオプションを繰り返したときは、値の OR をとる。
- `--filter-not-<key>` の値は、そのすべてに一致しないことを条件とする（AND NOT）。
  - 例: `--filter-diagnosis a --filter-diagnosis b --filter-not-diagnosis c` は「(a または b) かつ c でない」。
- 異なるオプションどうしは AND で組み合わせる。
  - 例外: result filter どうし（`--failed --unfinished`）は従来どおり OR。`--filter-result` の繰り返しと同じ意味である。
- Direct selector は、この計画で追加するものを含むすべてのフィルターと排他にする（SEL-8 の延長）。
- 時刻と継続時間のオプションは繰り返せない。繰り返すとエラーにする。

### 否定できるオプション

- `--filter-not-<key>` があるのは次のオプション。
  - `stage`、`matrix`、`command`、`exit-code`、`failure-kind`、`diagnosis`、`host`、`state`
  - `changed`、`new`、`outdated`、`unproduced`（真偽の条件。例: `--filter-not-outdated`）
- `result` と時刻・継続時間には否定を設けない。
  - `result` は値の組み合わせで表せる。時刻と継続時間は after / before などの対がある。

### 属性がないジョブ

属性が定義されないジョブは、肯定の条件にも否定の条件にも一致しない。

- success のジョブには diagnosis がないので、`--filter-diagnosis cuda-oom` にも `--filter-not-diagnosis cuda-oom` にも一致しない。`--filter-not-diagnosis cuda-oom` は、そう診断されなかった**失敗**ジョブだけを選ぶ。
- 投入されていないジョブには host と開始時刻がないので、`--filter-host` にも時刻の条件にも一致しない。
- produce-file を宣言していないジョブは、`--filter-outdated` にも `--filter-not-outdated` にも一致しない。

### どの attempt で判定するか

- 最新の attempt で判定する。ジョブの結果と同じ attempt である。
- 参照 run で実行されず引き継いだジョブは、元の run（`JobOrigin`）の attempt の時刻、host、ログで判定する。

### 結果の細分化

- `--filter-exit-code N`
  - 終了したジョブの、解決済みの exit code に一致する。繰り返すと OR。
- `--filter-failure-kind KIND`
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

- `--filter-diagnosis VALUE` は、**現行のルール**で最新 attempt のログから再計算した結果で判定する。
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

- `--filter-command RE`
  - Go の `regexp`（RE2）で、argv を空白で連結した文字列に照合する。
- ジョブの定義だけを見る条件なので、`show`、`copy`、`run`、`retry` に加えて `change`、`remove` でも使える。
  - `change` と `remove` では stage、matrix の条件と AND で組み合わせる。`--all` と job selector とは排他にする。
  - `change` と `remove` で stage と matrix が互いに排他であることは変えない。

### host

- `--filter-host PATTERN`
  - glob（`path.Match`）で、最新 attempt の hosts のどれかに一致すればよい。

### 時刻と継続時間

- 使う時刻:
  - 開始: `status.json` の `started_at`。なければ `submitted_at`。
  - 終了: `finished_at`
  - 継続時間: 終了 − 開始。実行中のジョブは「現在時刻 − 開始」。
- オプション:
  - `--filter-started-after`、`--filter-started-before`
  - `--filter-finished-after`、`--filter-finished-before`
  - `--filter-longer-than`、`--filter-shorter-than`
- 境界: after と longer-than は「以上」、before と shorter-than は「未満」。
- 時刻の書式:
  - 絶対時刻: `2006-01-02`、`2006-01-02T15:04`、`2006-01-02T15:04:05`、RFC3339。タイムゾーンを省略するとローカル時刻。
  - 相対時刻: Go の duration に `d`（24 時間）を加えたもの。現在時刻から遡った時刻を表す（例: `--filter-finished-after 2d`）。
- 現在時刻は、コマンドの開始時に 1 回だけ固定する。
- 時間帯を表す `--active-during` は設けない。「22:00〜02:00 に動いていた」は `--filter-started-before` と `--filter-finished-after` の組み合わせで書く。

### ジョブ定義の変更

- `--filter-changed`
  - 参照 run に対応するジョブがあり、fingerprint が異なるジョブ。
- `--filter-new`
  - 参照 run に対応するジョブがないジョブ。
- 対応付けと参照 run の決め方は、既存の `--match-by` と `FingerprintReferenceRun` に従う。
- 使えるコマンドは `run`、`retry`、`show`（queue の表示）。
  - `copy` は run から選ぶコマンドなので対象外。

### ファイルの鮮度

- `add --require-file PATH` と `add --produce-file PATH` で、ジョブが読むファイルと作るファイルを宣言する。
  - 繰り返し指定できる。
  - `change` でも編集できる。編集の書式は、既存の `change --env` の形に合わせる。
  - `--input` / `--output` は使わない。`add --output` は既に stdout の出力先であり、stdin / stdout とも紛らわしいためである。
  - 内部のフィールド名は `RequireFiles`、`ProduceFiles`。
  - `--require-file` はフィルターの判定にだけ使い、ジョブの実行前に存在を検査しない。名前から検査されると誤解されないよう、help と文書に明記する。
- パスの解釈:
  - ジョブの working directory からの相対パス
  - `${VAR}` を、明示された env、matrix の値、`ROTARI_ARRAY_TASK_ID` で展開する。
  - 展開後に glob を適用する。
- 判定は mtime で行う（make と同じ）。
  - `--filter-outdated`: produce-file が 1 つも存在しない、または最も古い produce-file より新しい require-file がある。
  - `--filter-unproduced`: 宣言した produce-file のどれかが存在しない。
  - require-file が 0 件（パスが存在しない、または glob が何にも一致しない）なら、フィルターの評価をエラーにする。
- 評価は呼び出し元のホストで行う。
  - SSH や scheduler の executor では、ファイルが共有 FS で見えることが前提となる。この前提は文書に明記する。
- require-file と produce-file の宣言は fingerprint に含める。
  - 宣言が空のジョブでは含めない。こうすると既存の fingerprint は変わらない。
- 宣言から依存関係を推論することはしない。宣言はジョブを選ぶためだけに使う。
  - [TODO.md](TODO.md) で「ジョブ間の出力受け渡し」をスコープ外にしているのと整合させる。
  - 古いジョブの下流は、既存の `expandDownstream` が `--depends-on` をたどって実行対象に加える。`run --filter-outdated` だけで、更新されたデータに影響するジョブとその下流が再実行される。

### cancel、suspend、resume

- 受け付けるオプション:
  - Direct selector として `--job-name` を追加する。
  - `--stage`、`--matrix`（省略形）と `--filter-stage`、`--filter-matrix`
  - `--filter-command`、`--filter-host`
  - `--filter-started-after`、`--filter-started-before`、`--filter-longer-than`、`--filter-shorter-than`
  - `--filter-state running|pending`（繰り返すと OR）
  - 上記のうち否定できるものの `--filter-not-*`
- `suspend` と `resume` は実行中のジョブしか扱わないので、`--filter-state pending` を受け付けない。
- 一致が 0 件ならエラーにする。run 全体の cancel にはしない。
- 確認:
  - TTY では対象の一覧を表示して確認する。
  - `--yes` で確認を省略する。
  - TTY でないときは `--yes` を必須にする。
- 確認の前に評価した集合を、そのまま `CancelJobs` に渡す。確認の後で評価し直さない。
- フィルターを指定しないときの既存の動作は変えない。
- `cancel --wait` はフィルターと排他にする。

### 各コマンドで使えるフィルター

表の `--filter-` は省略している。否定できるものは、同じコマンドで `--filter-not-*` も使える。

| フィルター | `show` | `copy` | `run`、`retry` | `change`、`remove` | `cancel` | `suspend`、`resume` |
| --- | --- | --- | --- | --- | --- | --- |
| `result`（省略形 `--failed` など） | ○ | ○ | ○ | – | – | – |
| `stage`、`matrix`（省略形 `--stage` など） | ○ | ○ | ○ | ○ | ○ | ○ |
| `command` | ○ | ○ | ○ | ○ | ○ | ○ |
| `exit-code`、`failure-kind` | ○ | ○ | ○ | – | – | – |
| `diagnosis` | ○ | ○ | ○ | – | – | – |
| `host` | ○ | ○ | ○ | – | ○ | ○ |
| `started-*`、`longer-than`、`shorter-than` | ○ | ○ | ○ | – | ○ | ○ |
| `finished-*` | ○ | ○ | ○ | – | – | – |
| `changed`、`new` | ○（queue の表示） | – | ○ | – | – | – |
| `outdated`、`unproduced` | ○ | ○ | ○ | – | – | – |
| `state` | – | – | – | – | ○ | `running` のみ |

Direct selector の `--job-name` は、この表のすべてのコマンドで使える（`cancel`、`suspend`、`resume` では新規）。

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
  - 定義（command、env、working directory、stage、matrix、require-file と produce-file の宣言）
  - 結果（finished、exit code、失敗の種類）
  - 実行（hosts、開始・終了時刻、実行状態）
  - 高価な属性（diagnosis の再計算、ファイルの stat）は遅延して読み込む。
- 評価の順序: 安い述語 → diagnosis → ファイル。
- `jobfilter` は `state` や `executor` に依存しない。`Facts` は `jobstatus` と `state` の側の adapter が作る。

既存の result selection（文字列）と scope（`model.CommandSelector`）はそのまま残し、`jobfilter.Filter` はその上に AND で重なる条件だけを持つ（Phase 1 で実装）。

- `run.PlanRerun`、`projectrun.Runner.PlanSelection`、`projectrun.Options`、`server.Request`、`queueedit.CopyRequest` に `Filter` を追加した。
- `--filter-stage` と `--filter-matrix` は既存の scope と同じく 1 つの値だけを取る。`--stage` と両方指定するときは同じ値でなければならない。
- stage や matrix を持たないジョブは、`--filter-not-stage` と `--filter-not-matrix` で除外されない。定義の属性では「ない」ことも 1 つの値として扱う。
- Web の `/api/copy` はこの計画では変えない。

### CLI への登録

- オプションの表（名前、値の形、説明、使えるコマンド）は CLI の関心なので `cmd/rotari/job_filter_flags.go` に置き、`jobfilter` は条件の意味だけを持つ。
- 各コマンドは `cliJobFilterOptions` で省略形と `--filter-*` をまとめて登録する。
  - help、CLI reference、shell 補完、Python 用の schema は、すべて CLI spec から生成される。
  - `--help` は `filter-` で始まるオプションを「Filters」の見出しの下に表示する。

## 実装の手順

各 phase は独立してコミットし、コミットごとにテストが通る状態を保つ。

### Phase 1: 土台（実装済み）

1. `internal/jobfilter` を新設し、`Filter` と評価関数を実装する。
2. `PlanRerun`、`CopyRequest`、`show` の表示に `jobfilter.Filter` を通す。
3. `--filter-*` を登録する仕組みと、help の「Filters」の見出しを追加する。
4. `--filter-result`、`--filter-stage`、`--filter-matrix`、`--filter-not-stage`、`--filter-not-matrix` を追加し、既存のオプションをその省略形にする（`show`、`copy`、`run`、`retry`）。
5. 既存の conformance テストが変更なしで通り、新しい行（SEL-11）も通ることを確認する。

`change` と `remove` への `--filter-*` は、`queueops.Editor.Change` と `Remove` の選択の受け取り方を変える必要があるので Phase 2 で扱う。

### Phase 2: 定義系

6. `--filter-command` を追加する（`show`、`copy`、`run`、`retry`、`change`、`remove`）。

### Phase 3: 結果の細分化

7. `jobstatus` に失敗の種類を判定する関数を追加する。
8. `--filter-exit-code` と `--filter-failure-kind` を追加する。

### Phase 4: 実行の属性

9. `started_at` を読む関数を `state` か `jobstatus` に追加する（Origin への fallback を含む）。
10. 時刻の解釈を `jobfilter` に実装する。
11. `--filter-host`、`--filter-started-*`、`--filter-finished-*`、`--filter-longer-than`、`--filter-shorter-than` を追加する。

### Phase 5: diagnosis

12. ルールに slug ID を追加する。
13. `--filter-diagnosis` を追加する（現行のルールで再計算する）。

### Phase 6: ジョブ定義の変更

14. `--filter-changed` と `--filter-new` を追加する。

### Phase 7: ジョブ制御

15. `cancel`、`suspend`、`resume` に `--job-name`、フィルター、`--filter-state`、`--yes`、確認プロンプトを追加する。

### Phase 8: ファイルの宣言

16. `QueuedCommand` と `JobSpec` に `RequireFiles` と `ProduceFiles` を追加し、`add` と `change` の `--require-file`、`--produce-file`、fingerprint への反映を実装する。
17. `--filter-outdated` と `--filter-unproduced` を追加する。

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
- `--filter-outdated` と `--filter-unproduced` は、一時ディレクトリで mtime を操作する単体テストで確認する。
- 省略形と `--filter-*` の形が同じ結果になることを確認する。
- help に「Filters」の見出しが出て、そのコマンドで使えるフィルターだけが載ることを確認する。
- 仕上げに pre-commit、`scripts/check.sh --short`、`scripts/check.sh` の順に実行する。

## スコープ外

- Web UI / Web API へのフィルターの反映（別計画）
- cron scheduling
- `--active-during`
- require-file と produce-file の宣言からの依存関係の推論
- make 的な機能のうち、この計画で入れるのは「古いジョブを選ぶフィルター」だけである。その先の段階は次のとおり扱う。
  - 宣言したファイルと `--depends-on` の食い違い（B の require-file が A の produce-file と一致するのに依存がない）を警告する検査。将来の候補として [TODO.md](TODO.md) に記録する。
  - 宣言したファイルから依存関係を推論すること、`run` が既定で古いジョブだけを実行すること、ターゲットの指定やパターンルール。rotari を workflow 言語にするものなので扱わない。
- ジョブの実行前に require-file の存在を検査すること
- result filter と時刻・継続時間のオプションの否定
- 汎用の `--not`。直後の条件を反転する案だったが、引数の順序に依存し、Python の kwargs で表せないため `--filter-not-*` にした。
- `--filter KEY=VALUE` のような 1 つのオプションへの集約。オプションが探しにくく、`>` や `!` の quote が必要になるため `--filter-*` にした。
- 保存済みの diagnosis による判定
- ユーザー定義の述語コマンド（`--where-cmd`）。ジョブごとにシェルコマンドを実行して exit code で選ぶ案だったが、quoting がわかりにくく、ジョブ数だけプロセスを起動して遅く、ファイルの宣言と用途が重なるため見送った。
