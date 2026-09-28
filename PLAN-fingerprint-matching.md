# 計画: fingerprint によるジョブ対応付け

## 位置づけ

これは [PLAN-unify-copy-import.md](PLAN-unify-copy-import.md) の次の phase の計画である。

phase 1 では、import、copy、change、run の status 判定と結果の引き継ぎを統一した。
この phase では、異なる run 間で Job ID が変わっても、同じジョブの結果を対応付けられるようにする。

Job ID は引き続きランダムな一意識別子として扱い、ジョブの内容的な同一性を Job ID に持たせない。

## 決定事項

- fingerprint 対応付けは、新しい queue から run を作るときに使う。`import` と `copy` 自体の対応付け方法は変更しない。
- 対応付けモードは `job-id`、`fingerprint`、`job-id + fingerprint` を選べるようにする。デフォルトは `job-id + fingerprint` とする。
- `job-id + fingerprint` では、まず Job ID / Origin で対応付け、未対応の実行単位だけを fingerprint の対象にする。
- `latest` は、その project の最新の確定済み run を指す。active または interrupted の run は除外する。明示した run ID は active / interrupted でも fingerprint 比較の対象にできる。
- fingerprint は queue と run のどちらにも保存せず、現在 queue と過去 run の `commands.json` から毎回再計算する。
- 過去 run の status に関係なく、fingerprint が一致すれば対応付ける。結果を引き継ぐかどうかは既存の filter と status 判定で決める。
- 過去 run にしか存在しない実行単位は無視する。

## 目的

- Job ID や job name が変わった新しい queue と過去 run のジョブを対応付ける。
- 同じスクリプトを複数回実行したとき、成功済みジョブを結果ごと引き継ぐ。
- コマンドの変更を検出し、変更されたジョブとその下流を再実行する。
- 既存の `run`、`retry`、`copy` の引き継ぎ処理と fingerprint 対応付けを共存させる。
- 対応付け方法を変えても、結果の表示、status の書き換え、`Origin` の扱いを二重実装しない。

## 基本方針

### 対応付けと実行判定を分離する

fingerprint は「どの過去ジョブが同じジョブか」を決めるためだけに使う。
実行するか、結果を引き継ぐかは、対応付け後に既存の run フィルターと status 判定で決める。

```text
新しい queue
  -> fingerprint を計算
  -> 過去 run の候補と対応付け
  -> 一致したジョブに Origin を付与
  -> MarkedStatus / ResultStatus を適用
  -> 既存の run フィルターで execute / carry を決定
```

fingerprint 対応付け専用の execute / carry 判定を新設しない。

### 明示的な出どころを優先する

`job-id + fingerprint` の対応付け順は次のとおりとする。

1. 既存の `Origin` と Job ID による対応
2. 同一 queue 内の Job ID による対応
3. 未対応の実行単位に対する fingerprint による対応
4. 対応なし。新しい unfinished job として扱う

fingerprint の一致だけで、明示された `Origin` を上書きしてはならない。

### 既存 run の引き継ぎとの共存

| 用途 | 対応付け | 結果の扱い |
| --- | --- | --- |
| 通常の `run`、`retry` | Job ID / Origin | 現在の carry と同じ |
| `copy`、明示的な source を持つ import | Origin / Job ID を優先し、必要なら fingerprint | 現在の carry と同じ |
| 新しい queue と過去 run の比較 | fingerprint | 一致した過去結果を Origin として既存処理へ渡す |

`job-id` モードでは fingerprint を使わず、`fingerprint` モードでは Job ID を使わない。
Job ID と fingerprint の不一致は、`job-id + fingerprint` の対応付け結果を変更しない。
既存の `Origin` / `TaskOrigins` は、どの match mode でも保護し、fingerprint で上書きしない。

## Fingerprint の構成

### ジョブ固有の入力

fingerprint に含める値は次のとおりとする。

- command と argv。argv の各要素をそのまま使い、前後空白の除去や shell としての再解釈はしない。
- `add --env` で明示された job 固有の environment の最終値。executor や呼び出し元の環境は含めない。
- `add --working-directory` で明示された working directory。`.` と `..` は正規化するが、実行時の current directory と結合しない。
- 展開後の matrix parameter
- 展開後の array task 番号

次の値は fingerprint に含めない。

- Job ID
- job name
- executor
- executor options
- timeout
- retry 設定
- array / matrix の範囲全体や定義全体
- DAG、依存関係、親 job の fingerprint
- 呼び出し元プロセスの環境変数
- 呼び出し元の current working directory

executor や timeout の変更は、同じジョブの実行方法の変更であり、別ジョブとは扱わない。
ただし、実行設定の変更を再実行理由として表示する必要があるかは別途検討する。

### DAG の扱い

ユーザーが `add` するたびに DAG の位置まで意識しなくて済むよう、DAG は fingerprint に含めない。
依存関係の変更による再実行は、既存の依存関係と run filter の規則で扱う。

canonical 表現は、Go の struct のデフォルト JSON や map の反復順に依存させない。
hash は SHA-256 とする。JSON には完全な値、人間向け表示には短縮値を出す。
fingerprint は保存しないため、実装を変更した場合は過去 run も現在の実装で再計算される。

## 重複ジョブの対応付け

同じ fingerprint のジョブが複数ある場合、fingerprint だけでは対応付けられない。
候補を次のキーで管理する。

```text
(fingerprint, occurrence index)
```

Job ID で対応した実行単位を現在 queue と過去 run の候補から先に除外し、残った実行単位の queue 内の出現順で occurrence index を付与する。

同じ fingerprint の候補数が新旧 run で異なる場合は、その fingerprint の対象を全て新規 job とする。
余った過去結果は無視し、余った新 job だけを新規扱いにすることもしない。
曖昧な候補を推測して対応付けない。

## マッチングの処理単位

初期実装では job と array task を分けて扱う。

- 通常 job: job fingerprint で対応付ける。
- array: 範囲全体ではなく、展開後の task 番号を含め、task ごとに対応付ける。
- matrix: matrix の定義全体ではなく、展開後の parameter 値を含め、leaf ごとに対応付ける。
- matrix group の親 job: group 全体の source を無理に 1 件へまとめず、leaf の対応を優先する。

対応付けた過去結果は、現在の `JobOrigin` / `TaskOrigins` に変換する。
その後の `MarkResult`、status 表示、run summary は既存実装を使う。

## 実行フィルターとの関係

fingerprint 対応付け後も、実行可否を fingerprint の一致だけで決定しない。

- fingerprint に一致した成功結果は、既存の carry 条件に従って引き継ぐ。
- fingerprint に一致した failed / cancelled / unfinished 結果は、既存の filter と status の規則に従う。
- 親が変わっても fingerprint 自体は変わらない。依存関係による下流再実行は既存の規則で扱う。
- 明示的な `--depends-on` または `--depends-on-finished` による下流再実行の規則は維持する。
- `change --status` と manifest の status mark は fingerprint の対応付け後に適用する。

## CLI / API の案

初期案では、対応付け方式をユーザーが選択できるようにする。
候補は共通の `--match-by id|fingerprint|id-and-fingerprint` オプションとする。

過去 run の選択を省略した場合は、既存の `latest` / reference run の選択規則を使う。

JSON の plan には、少なくとも次を含める。

- 対応付け方式
- 新しい job ID
- source run ID / job ID / attempt ID
- fingerprint（保存値ではなく、その場で計算した値）
- `matched`、`unmatched`、個数不一致の結果
- 実行計画上の最終 status

## 実装段階

### Phase 2.1: canonical fingerprint のみ

- `internal/model` または専用 package に canonical payload を定義する。
- 通常 job、array、matrix の入力を正規化する。
- array / matrix の展開後 parameter を計算する。
- 同じ入力から常に同じ fingerprint が得られる unit test を追加する。

この段階では既存の実行動作を変更しない。

### Phase 2.2: 候補の構築とマッチング

- 過去 run の `commands.json` から fingerprint と result / attempt を毎回計算する。
- `(fingerprint, occurrence index)` の候補 index を作る。
- unique match、unmatched、個数不一致を区別する。
- マッチ結果を `Origin` / `TaskOrigins` に変換する。
- Job ID / Origin の既存対応を先に確定し、対応済み実行単位を fingerprint 候補から除外する。

### Phase 2.3: dry-run と表示

- `run` に対応付け方式を選ぶオプションを追加する。デフォルトは `id-and-fingerprint` とする。
- `run` の dry-run 相当の計画表示に match の理由を表示する。
- JSON plan に fingerprint と source を出力する。
- `show` と Web UI で fingerprint match を確認できるようにするか決める。

### Phase 2.4: run 計画への接続

- fingerprint で Origin を付与した queue を既存の run planner に渡す。
- success carry、failed retry、status mark、下流再実行を既存規則で検証する。
- filterless run と filter 付き run の挙動を分けてテストする。

### Phase 2.5: conformance とドキュメント

- fingerprint の canonical 化と再計算方針を contract に記録する。
- CLI、import/export、copy、run のユーザー向け説明を更新する。
- Job ID が変わる同一 script、重複 command、親変更、依存種別変更の conformance を追加する。

## テストケース

### 対応付け

- Job ID と job name が変わっても同じ command が match する。
- command、job-specific environment、working directory が変わると match しない。
- executor、timeout、retry のみ変わっても match する。
- 同じ command が2つある場合、occurrence 順で対応する。
- 新旧で同じ command の個数が違う場合、余分な job を新規扱いにする。
- 個数不一致の候補は勝手に match しない。

### 展開と依存関係

- array の範囲を広げたとき、既存 task と追加 task を正しく扱う。
- matrix の値を追加したとき、既存 leaf と追加 leaf を正しく扱う。
- 通常 job、array task、matrix leaf を同じ fingerprint 集合として扱う。
- 依存関係を変更しても fingerprint 自体は変わらず、既存の下流再実行規則が働く。
- cycle や不正な依存関係は既存 validation で拒否する。

### 既存機能との共存

- 明示的な Origin が fingerprint 候補より優先される。
- fingerprint match 後の success carry が既存 carry と同じ結果になる。
- marked `success`、`failed`、`cancelled`、`unfinished` がそのまま機能する。
- fingerprint で未対応付けの job は新しい unfinished job になる。
- filterless run と retry / `-j` の結果が仕様どおりに分かれる。
- source attempt のログ、summary、show、Web 表示が正しい。

## 進捗

- [x] canonical fingerprint の計算と model 単体テスト
- [x] Job ID 優先、fingerprint fallback、候補除外、個数不一致の純粋 matcher
- [x] `run` request への `--match-by` 伝播
- [x] reference run の queue snapshot から Origin を付与する接続
- [x] `latest` と RUN-4 の契約追加
- [ ] dry-run / JSON plan の match 情報
- [ ] array / matrix と filter の conformance
- [ ] user-facing docs と CLI reference の更新

## 未決事項

1. JSON plan の field 名。
2. occurrence index の厳密な queue 出現順の定義。
3. fingerprint match の結果を `JobOrigin` だけで表現できるか、match metadata が必要か。
4. dry-run の人間向け表示でどこまで fingerprint の詳細を出すか。

## 完了条件

- Job ID と job name が異なる同一 script を、`id-and-fingerprint` で安全に対応付けられる。
- 重複 command と array / matrix の個数差が誤対応付けを起こさない。
- 対応付け後の結果引き継ぎが既存の `Origin`、status、run filter と同じ規則で動く。
- unmatched / 個数不一致が plan で区別される。
- 既存の Job ID / Origin ベースの `run`、`retry`、`copy` の動作を壊さない。
- `go test ./...`、関連 conformance、最終的に `scripts/check.sh` が通る。
