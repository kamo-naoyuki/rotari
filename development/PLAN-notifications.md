# 計画: 通知設定と通知イベントの統一

## 目的

- Webhook 通知とブラウザー通知で、通知対象、通知に載せる項目、集約の規則を共通化する。
- Webhook とブラウザーは同じ設定項目を使うが、設定値はチャネルごとに独立させる。
- run 完了だけでなく、ジョブの最終的な成功・失敗も通知できるようにする。
- 同時期に発生したイベントをまとめ、ジョブごとに大量の通知を送らないようにする。
- rule-based diagnosis を失敗通知の主要な情報として扱う。
- 通知設定を通常のコマンド既定値から分離し、CLI と Web UI の両方から生成・編集できるようにする。

## 決定事項

1. 通知設定は専用の `notifications.toml` に置く。
2. Webhook とブラウザーは共通の設定スキーマを使うが、`[webhook]` と `[browser]` で値を独立させる。
3. 通知対象は次の4項目で制御する。
   - `job_failure` (既定値 `true`)
   - `job_success` (既定値 `false`)
   - `run_failure` (既定値 `true`)
   - `run_success` (既定値 `true`)
4. ジョブ通知は retry 中の attempt 失敗ではなく、ジョブの最終結果だけを対象にする。
5. 一定時間内に発生したジョブ成功、ジョブ失敗、run 完了は1件の通知にまとめる。
6. Webhook とブラウザーは同じフィールド名を使う。選択するフィールドはチャネルごとに独立させる。
7. diagnosis の名前と suggestion は既定の通知項目に含める。evidence などの詳細は選択可能だが既定では含めない。
8. `webhook.on` は廃止する。後方互換、移行処理、非推奨警告は設けない。
9. 通知設定の細目は環境変数に展開しない。Webhook URL の secret 注入だけは環境変数で上書きできるようにする。
10. Webhook は run 開始時の設定を使い、実行中の設定変更の影響を受けない。
11. ブラウザー通知は Web サーバーが保持する現在の設定を使い、`notifications.toml` の保存または reload 後に反映する。
12. ブラウザー通知の権限、ON/OFF の localStorage、初回表示時に過去のイベントを通知しない挙動は維持する。
13. Web UI では `notifications.toml` の生成、編集、保存、reload を提供する。
14. blocked、cancelled、開始前 cancel は `job_failure` に含める。
15. Webhook はリンクを載せない。ブラウザー通知だけが対象の Web 画面へ遷移できる。
16. Webhook の永続的な送信台帳と自動再試行は初版では実装しない。
17. 集約 window は最初のイベントから10秒とし、イベントが続いても延長しない。run 終了時は直ちに flush する。
18. ブラウザー通知の本文は1000文字を上限とする。
19. Web UI は notification 専用の構造化フォームにする。

## 設定ファイル

### 配置

既存の config と同じ3階層に置く。

| scope | path |
| --- | --- |
| global | rotari の global config directory の `notifications.toml` |
| basedir | `<basedir>/notifications.toml` |
| project | `<basedir>/projects/<project>/notifications.toml` |

解決順は project、basedir、global とし、最初に見つかった1ファイルだけを丸ごと使う。階層間の merge は行わない。

### 形式

```toml
[webhook]
url = "https://example.invalid/hook"
format = "slack"
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = [
  "project",
  "run_id",
  "run_name",
  "run_status",
  "job_id",
  "job_name",
  "job_status",
  "exit_code",
  "success_count",
  "failure_count",
  "duration",
  "diagnosis_name",
  "diagnosis_suggestion",
]
max_jobs = 20

[browser]
job_failure = true
job_success = false
run_failure = true
run_success = true
fields = [
  "project",
  "run_name",
  "run_status",
  "job_name",
  "job_status",
  "exit_code",
  "diagnosis_name",
  "diagnosis_suggestion",
]
max_jobs = 10
```

### 環境変数

- `ROTARI_WEBHOOK_URL`: `notifications.toml` の Webhook URL を上書きする。
- URL 以外のイベント条件、fields、件数上限などはファイルだけで設定する。
- `ROTARI_WEBHOOK_ON` は削除する。
- JSON、Slack、Teams、Discord を選ぶ既存の `ROTARI_WEBHOOK_FORMAT` は削除し、`format` は `notifications.toml` だけで設定する。

## 通知フィールド

### 識別情報

- `project`
- `run_id`
- `run_name`
- `job_id`
- `job_name`
- `stage`
- `array_task_id`
- `attempt_id`

### 結果

- `run_status`
- `job_status`
- `exit_code`
- `error`
- `success_count`
- `failure_count`
- `total_count`

### 時間

- `started_at`
- `finished_at`
- `duration`

### 実行情報

- `executor`
- `hosts`
- `working_directory`
- `command`

`working_directory` と `command` は機密情報を含む可能性があるため既定では無効にする。環境変数と executor options は通知フィールドに含めない。

### diagnosis

- `diagnosis_status`
- `diagnosis_name`
- `diagnosis_evidence`
- `diagnosis_suggestion`
- `diagnosis_rules`
- `diagnosis_outdated`

`diagnosis_name` と `diagnosis_suggestion` は既定で有効にする。`diagnosis_evidence` は長文やパスを含む可能性があるため既定では無効にする。複数の diagnosis がある場合は、通知対象となったジョブにすべて紐付ける。

### 誘導情報

- `link`
  - ブラウザー通知で対象 run または job の URL を開くために使う。
  - Webhook では出力しない。

### 常に含めるメタデータ

- event の種類
- 通知生成時刻
- payload の schema version

これらは `fields` の対象外とする。該当イベントに存在しない選択フィールドは出力しない。

## 通知イベントと集約

### 共通イベントモデル

`internal/notification` を新設し、次を所有させる。

- `Settings`: 4つの通知条件、fields、`max_jobs`
- `Event`: project、run、job、結果、時刻、diagnosis を持つチャネル非依存のイベント
- `Batch`: 一定期間にまとめられたイベント群
- フィールド名、検証、既定値、並び順
- イベントが設定対象かを判定する関数
- Batch からチャネル非依存の表示モデルを作る関数

Webhook の HTTP 送信と各サービス固有の encoder は adapter として分離する。ブラウザー側には共通モデルと設定を JSON で渡し、JS は同じフィールド語彙と集約結果を表示する。

### ジョブイベント

- ジョブが retry を終えて最終的に success または failed になった時点でイベントを生成する。
- retry 待ちの attempt failure は生成しない。
- blocked、開始前 cancel、明示的 cancel は最終 failure としてイベントを生成する。
- ジョブ名などは `JobSpec`、結果と diagnosis は最終 `JobResult`、時刻は attempt の状態から取得する。

### run イベント

- summary が確定した後に run success または run failure イベントを生成する。
- run 完了が集約 window 内に入れば、その run の直前のジョブイベントと同じ通知にまとめる。

### 集約

- 集約は project と run ごとに行い、異なる run のイベントは混ぜない。
- 成功と失敗は同じ Batch に含め、通知本文の中で分類する。
- `max_jobs` を超えたジョブは省略件数を表示する。
- ブラウザー通知は本文を1000文字で切り詰め、省略表示を末尾に付ける。Webhook は各 format のサービス上限を超えないよう adapter ごとに切り詰める。
- run 終了時には pending Batch を直ちに flush し、run 完了を含む最後の通知を送る。
- 通知失敗は run や job の結果を変更しない。
- Webhook は scheduler を止めない送信キューを使い、run 終了前に pending 送信を flush する。
- ブラウザーはポーリングで検出した最終結果を同じ Batch 表示モデルに変換する。

最初のイベントで10秒の固定 window を開始し、その間のイベントをまとめる。後続イベントで期限を延長しないため、ジョブが連続して完了しても通知が無期限に遅れない。run 終了時には window の残り時間を待たず即座に flush する。

## Webhook

- 既存の JSON、Slack、Teams、Discord encoder を共通 Batch モデルに対応させる。
- 1ジョブごとの HTTP POST は行わず、集約した Batch ごとに送る。
- run 開始時に解決済みの Webhook 設定を作り、run の実行プロセスで保持する。
- config file は run の既存 config snapshot と同様に snapshot する。
- URL が環境変数由来の場合、URL 自体を run directory に保存しない。実行プロセス内だけで保持する。
- 初版では永続的な送信台帳と自動再試行を設けない。実行中の送信 queue だけが Batch を所有し、HTTP 送信に失敗したら警告を記録して破棄する。
- supervisor 以外が cancellation を finalize する経路でも、run 開始時の設定を使える構造にする。

## ブラウザー通知

- `rotari web` 起動時に有効な `notifications.toml` を読み、Web サーバーの notification manager に保持する。
- 保存または reload が成功したら manager の値を atomic に差し替える。
- parse や validation に失敗した場合は現在の設定を維持し、Web UI にエラーを返す。
- 開いているページは API の settings revision を確認し、次のポーリングで新しい設定を反映する。
- Job activity とメイン画面の独立した通知実装を共通 JS モジュールへ統合する。
- API projection に、retry 待ちの attempt failure と最終 failure を区別できる情報を追加する。
- localStorage の通知 ON/OFF は server 設定より利用者側の上書きとして維持する。
- 初回 poll は baseline の構築だけを行い、既存の完了イベントを通知しない。
- static export はサーバーが存在せず、ファイル保存、reload、状態ポーリングができないため、notification editor と live browser notifications を表示しない。

## CLI と Web UI での設定管理

### CLI

既存の `config` コマンドを拡張し、通常 config と notification config を明示的に選べるようにする。

想定する操作:

- `rotari config generate notifications ...`
- `rotari config show notifications ...`
- `rotari config validate notifications ...`

正確な CLI syntax は既存の config command の構造を読んで決め、CLI spec、Python client、CLI reference を同じ定義から更新する。

### Web UI

既存の Config editor を拡張する。

- `config.toml` と `notifications.toml` を切り替える tabs
- global、basedir、project の保存先選択
- `notifications.toml` がない場合の Generate ボタン
- notification 専用の構造化フォーム
- Save ボタン
- 外部で変更されたファイルを再読込する Reload ボタン
- 保存前の parse と validation
- 保存時の atomic write
- 保存成功後の browser notification settings の即時 reload

Webhook URL は secret として扱う。少なくとも次を満たす。

- API response、HTML、ログへ不用意に URL を出さない。
- 既存値をマスクし、変更時だけ新値を送る。

書き込み API は既存どおり `--allow-control` を要求する。読み取りについても URL を含むため、Web 認証なしで remote host に公開できないようにする。

## 設定読み込みと reload

### notification manager

Web サーバー側に次の責務を持つ manager を置く。

```go
type Manager interface {
    Current(project string) Settings
    Revision() string
    Reload() error
}
```

- global、basedir、project のファイルを解決・検証する。
- reload 全体が成功した場合だけ snapshot を差し替える。
- 読み取りは immutable snapshot に対して行い、各 request でファイルを再読込しない。
- basedir 切り替えがあるため、basedir ごとの設定解決を扱う。

Webhook は Web の manager を参照せず、run 開始時に同じ resolver を使って独立した snapshot を作る。

## 実装段階

### 1. 共通モデルと設定 resolver

- `internal/notification` を追加する。
- 設定型、既定値、field registry、validation、event filtering、Batch 表示モデルを実装する。
- global、basedir、project の `notifications.toml` path と解決規則を実装する。
- config template を追加する。
- 単体テストを追加する。

### 2. 最終ジョブイベント

- run engine に最終結果だけを通知する hook を追加する。
- `projectrun.Observer` へ job finished hook を追加し、sync run と supervisor の両方から呼ばれるようにする。
- retry 途中の failure が通知されないテストを追加する。
- cancellation、blocked、array job の扱いをテストする。

### 3. Webhook adapter と集約

- 既存 encoder を Batch 対応へ移す。
- 非同期 queue、固定集約 window、run 終了時 flush を実装する。
- `webhook.on` と旧環境変数を削除する。
- run 開始時の config snapshot と環境変数 URL の扱いを実装する。
- JSON、Slack、Teams、Discord のテストを更新する。

### 4. ブラウザー通知

- Web process 起動時に browser settings を読み込む。
- API へ設定と revision を追加する。
- 最終ジョブ結果を判別できる projection を追加する。
- メイン画面と Job activity の通知コードを統合する。
- 4つの通知条件、fields、`max_jobs`、集約を適用する。
- localStorage と初回 baseline の既存挙動を維持するテストを追加する。

### 5. CLI と Web UI の設定管理

- `config` command に notifications の generate、show、validate を追加する。
- Web Config editor に file type の切り替え、Generate、Save、Reload を追加する。
- notification manager の atomic reload を実装する。
- secret の表示・保存規則と read-only/auth behavior をテストする。

### 6. docs、contracts、conformance

- contracts に通知イベント、集約、snapshot、reload、secret の規則を追加し、ID を付ける。
- conformance で CLI と Web API から設定生成・保存・reload を確認する。
- docs と architecture を更新する。

## テスト

### 単体テスト

- notification settings の既定値、validation、field selection
- global、basedir、project の解決
- job success/failure と run success/failure の filter
- retry 中の failure の抑制と最終結果イベント
- Batch の grouping、flush、`max_jobs`
- diagnosis の複数件表示と選択 field
- Webhook encoder の各 format
- Webhook の送信失敗と run 終了時 flush
- Web manager の reload 成功、失敗時 rollback、revision
- browser JS の success/failure、初回 baseline、localStorage、settings revision
- Web API の Generate、Save、Reload、read-only rejection

### 実行順

1. `go test ./internal/notification`
2. 変更した package の focused tests
3. `go test ./cmd/rotari ./internal/projectrun ./internal/run ./internal/web ./internal/webui`
4. `go test -count=1 ./internal/archtest`
5. `scripts/check.sh --short`
6. `go test ./conformance`
7. `scripts/check.sh`

## contracts と docs

更新対象:

- `contracts/01-resolution-and-config.md`
- `contracts/02-run-lifecycle-and-execution.md`
- `contracts/05-web-assets-and-static-export.md`
- `contracts/README.md`
- `docs/ARCHITECTURE.md`
- `docs/CONFIGURATION.md`
- `docs/NOTIFICATIONS.md`
- `docs/ENVIRONMENT_VARIABLES.md`
- `docs/CLI_REFERENCE.md`
- `docs/FAQ.md`

CLI reference と environment variable docs は generator を使って更新する。README の変更が必要な場合は `docs/GETTING_STARTED.md` を編集し、`python3 scripts/sync_readme.py` を実行する。

## コミットの分け方

1. notification package、専用設定ファイル、resolver、CLI generate/validate
2. job finished hook と Webhook のイベント集約
3. ブラウザー通知の共通化と設定反映
4. Web Config editor の generate/save/reload
5. contracts、conformance、docs

各段階で focused tests を通してから次へ進む。作業ツリーにある今回と無関係な変更は stage しない。

## 今回やらないこと

- Service Worker や Web Push による、ページを閉じている間のブラウザー通知。
- 任意の外部 notification provider を plugin としてロードする仕組み。
- 通知本文への環境変数や executor options の掲載。
- 通知設定をすべて CLI flags または環境変数で表現すること。

## 進捗

- [x] 要件整理
- [x] 現行 Webhook とブラウザー通知の調査
- [x] 未決事項の確定
- [ ] 共通 notification package
- [ ] `notifications.toml` resolver と template
- [ ] CLI generate/show/validate
- [ ] 最終ジョブイベント hook
- [ ] Webhook の集約と encoder 更新
- [ ] ブラウザー通知の共通化
- [ ] Web Config editor の拡張と reload
- [ ] contracts と conformance
- [ ] docs
- [ ] full validation
