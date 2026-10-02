# 計画: agent 向け rotari MCP インターフェース

## 用語

MCP の通信で `initialize`、`tools/list`、`tools/call`、`resources/list`、`resources/read` のような名前は、**MCP の protocol method（JSON-RPC method／プロトコルメソッド）**と呼ぶ。クライアントが要求メッセージの `method` に指定する操作名である。

`rotari_search_logs` のような名前は MCP の protocol method ではなく、`tools/list` でサーバーが公開し、`tools/call` の `params.name` で呼ばれる **tool name（ツール名）**である。`initialize` 応答の `capabilities` は、Tools や Resources など機能カテゴリへの対応をクライアントに知らせる。具体的なツールやリソースは、それぞれ `tools/list` や `resources/list` で列挙する。

この計画では、MCP protocol method と rotari が公開する tool/resource を区別して記述する。

## 目的

- agent が rotari のプロジェクト、実行、ジョブ、ログ、診断情報を、任意のシェルコマンドやファイルパスを組み立てずに調べられるようにする。
- basedir やログファイルの物理パスを MCP の通常の引数・返却値から隠し、サーバーがプロジェクトの登録情報から解決する。
- job-id 単体のように場所を特定できない識別子で誤った対象を扱わない。tool 間で引き継げる、曖昧さのない project/run/job 参照を返す。
- 人間向け CLI のコマンド体系をそのまま複製するのではなく、agent が小さな操作を組み合わせられるインターフェースにする。
- 既存の rotari の解決規則、状態・ログの読み取り、選択規則を MCP 用コードに重複実装しない。

## 基本方針

1. **プロトコル準拠とドメイン API を分ける。** `initialize`、`tools/list` などは MCP SDK に処理させ、rotari の固有機能は MCP tool のハンドラーから既存の rotari 機能へ接続する。
2. **既定では登録済みの許可範囲だけを参照する。** MCP 起動設定でアクセス可能な basedir を登録し、その外を暗黙に走査しない。
3. **パスではなくプロジェクト参照を受け渡す。** `project_ref` は MCP サーバーが登録対象に割り当てる論理参照とする。表示名が重複しても別プロジェクトを識別できる。basedir の実パスは原則返さない。
4. **検索結果から次の操作に必要な識別子を返す。** 少なくとも `project_ref`、`run_id`、`job_id`、必要なら `attempt_id` と検索位置を含める。
5. **応答は機械可読・小さく・制限付きにする。** JSON の構造化結果、上限件数、ページングまたは切り詰め情報を使い、巨大ログを一括で agent に渡さない。
6. **曖昧さはエラーとして解消する。** project 名が重複する、run/job が project 内で見つからない、権限範囲外などの場合に、候補や不足情報を返して選択を促す。候補があるからといって暗黙に一件を選ばない。
7. **読み取りから始める。** 実行・変更系は読み取り系と分け、preview と実行を分離する。MCP クライアントの確認 UI だけを安全性の根拠にしない。

## 目標とするプロトコルの流れ

MCP クライアント（VS Code など）と MCP サーバー間の流れを次のようにする。

1. クライアントが `initialize` を送り、protocol version、client capabilities、client info を伝える。
2. サーバーは version を合意し、`capabilities` に提供する機能（初版は `tools`）と `serverInfo` を返す。
3. クライアントが `notifications/initialized` を通知する。
4. クライアントは `tools/list` で tool 名、説明、入力 JSON Schema、可能なら出力 schema を取得する。
5. agent が tool を使う判断をしたとき、クライアントが `tools/call` に tool name と arguments を載せて送る。
6. サーバーは入力を検証し、project 参照を登録情報に解決して rotari の処理を呼ぶ。成功時は構造化結果、失敗時は修正可能なエラーを返す。
7. クライアントは結果を agent に渡す。agent は返された project/run/job 参照を使い、別の tool call で詳細を追加取得できる。

`capabilities` は概要、`tools/list` は提供ツールの一覧、`tools/call` は個別の実行であり、「Tools モード」への切り替えではない。Resources を採用する場合も、`initialize` で `resources` capability を宣言し、`resources/list`、`resources/read` 等を別途使う。

## 初版に含める tool 候補

ツール名は設計案であり、確定 API 名ではない。個々の出力フィールドと JSON Schema は実装前に確定する。

### プロジェクト解決

- `rotari_list_projects`: サーバーで許可された project の `project_ref`、表示名、必要な識別情報を列挙する。物理パスは返さない。
- `rotari_describe_project`: `project_ref` の存在確認、現在の queue/run の概要、利用可能な検索範囲などを返す。

これにより、agent が最初から basedir を知らなくても、登録済み範囲から選択して後続操作を呼べるようにする。アクセス可能な登録対象が一つだけの場合でも、応答ではその `project_ref` を明示する。

### 実行・ジョブの発見と詳細

- `rotari_list_runs(project_ref, limit, cursor, status?)`: run の ID、名前、状態、開始・終了時刻、集計を返す。
- `rotari_list_jobs(project_ref, run_id, filters?, limit, cursor)`: job の ID、名前、状態、exit code、attempt 情報の要約を返す。
- `rotari_get_job(project_ref, run_id, job_id)`: 解決済みの状態、結果、実行情報、関連 attempt とログ取得の参照方法を返す。
- `rotari_get_report(project_ref, run_id?, job_id?)`: 既存の診断レポートを agent が扱いやすい構造または Markdown で返す。

run と job の詳細取得は project を必須にする。run/job ID のみから全 basedir を無制限検索する機能は初版に含めない。

### ログ検索と取得

- `rotari_search_logs(project_ref, keyword, run_id?, job_id?, stream?, limit?, cursor?)`: 指定 project の許可された履歴範囲からキーワードを検索し、run/job/attempt、stdout/stderr、行番号、短い前後 excerpt を返す。
- `rotari_get_log_excerpt(project_ref, run_id, job_id, stream, start_line?, end_line?, max_bytes?)`: 一致箇所の前後など、必要な範囲だけ取得する。

検索結果に十分な識別情報を含め、続く excerpt 取得で同じ basedir や file path を要求しない。検索は対象 scope を明示できるようにし、既定の最大範囲・最大件数・最大バイト数を定める。検索語は任意のシェル式として解釈しない。

## Resources と Prompts の扱い

### Resources

初版は **Tools を主な API とする**。プロジェクト横断の検索や条件付き抽出など、入力を受けて処理を実行する用途は tool が自然であり、Resources の URI に検索条件や状態設定を詰め込まない。

将来、安定してアドレス可能な読み取り専用データ（例: 選択済み run の概要、job の診断レポート）を VS Code の resource picker や自動コンテキスト注入に活用したい要件が確認された場合に、`resources/list`、`resources/read`、必要なら resource templates を検討する。resource URI は物理 file path にしない。URI の形式、更新・購読、情報の鮮度、クライアント側での提示方法を先に定義する。

### Prompts

agent に作業手順を明示的に選ばせたい場合（例: 失敗 job を診断する手順）に `prompts` を検討する。初版の機能 API やプロジェクト設定の保管場所として prompts を使わない。

## エラーと返却形式

- `tools/call` の引数不正、対象なし、対象の曖昧さ、検索上限到達、rotari 内部エラーを区別する。
- JSON-RPC レベルの malformed request / unknown method / unknown tool と、tool 実行結果の業務エラーを分ける。業務エラーは `isError` を使い、agent が入力を直せる情報を含める。
- 成功応答は、`structuredContent` と対応する出力 schema を優先して検討し、クライアント互換のため必要なテキスト要約も併せる。
- 出力に秘密情報を含む環境変数、認証情報、不要な絶対パスを含めない。command や working directory などは、機密性・サイズを評価して個別に選ぶ。

## 書き込み・実行系の後続段階

読み取り専用 API を試した後、必要性と承認モデルを評価して次を追加する。

- `rotari_check_project`: 実行前の readiness check。
- `rotari_preview_import`: manifest の検証と変更計画を返す dry-run。
- `rotari_add_jobs` / `rotari_change_job`: 明示した project に限定した queue 編集。
- `rotari_start_run`: 非同期実行を開始し、`run_id` を返す。
- `rotari_get_run_status` / `rotari_wait_run`: run 状態または完了を確認する。
- `rotari_retry_failed`: failed/unfinished の選択を rotari の共通選択規則に委譲する。
- `rotari_cancel_job` / `rotari_cancel_run`: 対象を特定した制御。

変更系は「preview → ユーザー確認または明示的な許可 → commit」の形を検討し、対象 project と変更内容を再表示できるようにする。dry-run と実行で条件が変わった場合の扱い、呼び出しの再送・重複実行、長時間処理、キャンセル、監査ログを別途設計する。初版では `run --async` 相当の切り離しを基本とし、クライアントの tool timeout がジョブを意図せずキャンセルしないようにする。

## 既存実装との接続方針

- 既存 CLI を文字列生成して呼び出す adapter を初期試作に使う場合も、引数を固定的に構築し、任意シェル文字列を実行しない。JSON 出力を検証して構造体へ変換する。
- 長期的には MCP adapter が `cmd/rotari` の CLI 層に依存せず、共通の内部パッケージを呼び出す境界を検討する。CLI 表示用文字列を MCP API の契約にしない。
- status/result の解決は `internal/jobstatus`、job 選択は `internal/jobfilter` 等、既存の一元化された規則を通す。Web の履歴検索やログ解決の規則が使える場合も、MCP 側に別の実装を複製しない。
- basedir と project の解決、登録された basedir の許可範囲、複数 basedir に同名 project が存在する場合の参照 ID は、MCP adapter 内に独自の暗黙ルールを作らず、共有境界として設計する。

## 実装・検証の段階

1. **要件と API 契約を決める**: MCP SDK/言語、stdio または HTTP transport、project registry の供給方法、`project_ref`、ログ検索範囲、出力上限、エラー形式、対応 MCP version を決定する。
2. **読み取り専用の縦切り試作**: `initialize`、`tools/list`、`tools/call` と list_projects / list_runs / search_logs / get_log_excerpt を実装し、VS Code から呼ぶ。
3. **コンテキスト継続性を検証する**: project 名が重複する環境、basedir が未指定、同じ job ID が別 project にある場合でも、検索結果から正しいログへ一意にたどれるかを試験する。
4. **結果の有用性を検証する**: 検索結果の excerpt、識別情報、診断情報、件数制限で agent が追加の shell/path 操作なしに原因特定できるかを実タスクで評価する。
5. **書き込み操作を別途設計する**: 読み取り版の使い勝手・誤選択の実績を踏まえ、preview と実行操作、確認、アクセス制御を合意してから実装する。
6. **配布形態を選ぶ**: rotari 本体への同梱か独立 MCP server かを、バージョン同期、SDK/依存、運用、認証、リモート利用の必要性で比較する。

## 初版の非目標

- MCP サーバーを通じた任意コマンド実行、任意ファイル読み書き、任意 basedir 探索。
- 全プロジェクト・全ログを制限なく走査して agent に返す機能。
- MCP が agent の内部状態や永続メモリを直接設定すること。
- MCP Resources を必ず導入すること。Tools と Resources のどちらが使いやすいかは、クライアントでの resource 提示要件を確認してから決める。
- VS Code 専用の UI 拡張を初版で作ること。まず MCP の読み取り API と既存 VS Code の MCP クライアントで価値を検証する。

## 未決事項

- MCP server として実装する言語・SDK、および rotari 本体との配布単位。
- MCP 起動時に許可する basedir 群をどこから得るか（明示設定、master registry、workspace との結び付け）。
- `project_ref` の安定性・表示名・衝突時の選択 UI、およびサーバー再起動をまたいだ安定性。
- ログ検索を最新 run のみにするか、期間・run を指定できる履歴検索にするか。既存 history search とどう重ねるか。
- 複数の MCP client/session 間で選択状態を共有するか。初版では tool 引数の `project_ref` による明示を優先し、サーバーのグローバルな active project 状態は持たない案を推奨する。
- `structuredContent` / output schema を含む応答の、対象 MCP version と VS Code 実装での互換性。
- project ごとの読み取り・書き込み許可、機密ログの扱い、必要な監査記録。
