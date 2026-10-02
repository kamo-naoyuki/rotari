# アイデア出し: masterdir を起点に agent へ rotari の情報を渡す

## この文書の位置づけ

これは実装計画や確定 API 仕様ではなく、rotari と agent の連携で「agent がどんな情報を受け取れると仕事を進めやすいか」を考えるための叩き台である。以下の tool 名、引数、情報項目、MCP 機能は候補にすぎず、必要性やまとめ方を議論するための例として挙げる。

まず、MCP サーバーの探索範囲を一つの masterdir に対応させ、その registry から見つかる basedir を単位として情報を返す案を土台にする。masterdir や basedir の物理パスを agent に毎回渡させず、basedir ごとの状態を起点に project、run、job、log へたどれるようにする。その上で、agent にどんな情報を渡すと仕事を進めやすいかを考える。Tools / Resources / Prompts のどれで公開するかは、必要な情報と操作を考えた後で選ぶ。

用語上の注意: ここでいう「masterdir に 1:1 対応」は、MCP サーバーの設定・探索範囲を一つの masterdir に対応づける意味で使う。rotari の registry は一つの masterdir の下に複数 basedir を登録できるため、masterdir と basedir 自体が 1:1 という意味ではない。

## 用語

MCP の通信で `initialize`、`tools/list`、`tools/call`、`resources/list`、`resources/read` のような名前は、**MCP の protocol method（JSON-RPC method／プロトコルメソッド）**と呼ぶ。クライアントが要求メッセージの `method` に指定する操作名である。

`rotari_search_logs` のような名前は MCP の protocol method ではなく、`tools/list` でサーバーが公開し、`tools/call` の `params.name` で呼ばれる **tool name（ツール名）**である。`initialize` 応答の `capabilities` は、Tools や Resources など機能カテゴリへの対応をクライアントに知らせる。具体的なツールやリソースは、それぞれ `tools/list` や `resources/list` で列挙する。

この計画では、MCP protocol method と rotari が公開する tool/resource を区別して記述する。

## 考えたいこと

- MCP サーバーごとに一つの masterdir を設定し、その registry に登録された basedir を列挙する形がよいか。
- basedir の物理パスを表に出さず、basedir 単位の識別子と情報を返す場合、agent と利用者にどんな名前・説明を見せると選びやすいか。
- basedir 一覧の各項目に、project 数、最近の run、実行中件数、最近の失敗など、どの概要を添えると「どこを調べるか」を選びやすいか。
- agent に任せたいのは、状態確認、ログ調査、原因診断、run 間比較、再実行の提案、実際の実行のどこまでか。
- agent が初めに知らない project / run / job をどこまで自力で発見できるとよいか。
- 一度返した情報を次の問い合わせで使うには、どの識別子や参照を返すと迷いにくいか。
- どの情報を一度に返し、どの情報を追加問い合わせにするのがよいか。ログ量や context 消費をどう抑えるか。
- パスを隠すことと、複数の basedir / 同名 project を安全に区別することをどう両立するか。
- agent に提案だけさせるのか、queue 変更や実行までさせるのか。人の確認が必要な境界はどこか。

## MCP 上での載せ方（参考）

MCP クライアント（VS Code など）と MCP サーバー間の流れを次のようにする。

1. クライアントが `initialize` を送り、protocol version、client capabilities、client info を伝える。
2. サーバーは `capabilities` で Tools / Resources / Prompts など、実装している機能カテゴリを知らせる。
3. `notifications/initialized` の後、クライアントは必要に応じて `tools/list`、`resources/list` などで個別の提供内容を問い合わせる。
4. Tools を使う設計なら、agent の選択を受けたクライアントが `tools/call` を送り、サーバーからの結果を agent に渡す。

これは情報を載せる方法の説明であり、Tools を初版で選ぶという決定ではない。

`capabilities` は機能カテゴリの概要、`tools/list` は提供ツールの一覧、`tools/call` は個別の実行であり、「Tools モード」への切り替えではない。Resources を使う場合は `resources/list`、`resources/read` 等を別途使う。

## 情報の探索階層

現時点の土台案では、情報を次の階層でたどる。basedir ごとの概要を最初に返し、必要になった項目だけを深掘りする。

```text
MCP server instance
└── configured masterdir
    └── registry に登録された basedir
        └── project
            └── run
                └── job / attempt / log / diagnosis
```

masterdir は registry を見つける設定上のスコープであり、それ自体をジョブやログの識別子にしない。registry は複数 basedir を持てるので、返却情報や後続の問い合わせでは、どの basedir に属するかを保つ必要がある。異なる basedir の同名 project や同じ job ID を混同しないことが要点。

## agent に渡す情報のアイデア

以下は情報の候補であり、すべてを返す提案ではない。agent にどの問いを解かせたいかを決め、そこから必要な情報を絞る。

### 1. 「どの basedir を見ればよい？」

- 候補情報: registry 由来の basedir 参照、利用者向けの表示名、登録状態（存在する / 消失している）、含まれる project 数、最近の活動時刻、実行中 run 数、最近の失敗数。
- agent に嬉しい点: basedir や state directory の物理パスを知らなくても、調査対象の候補と概要を見つけられる。
- 例となる問い合わせ: 「登録されている state を一覧して」「どの basedir で最近失敗があった？」
- 例となる tool 候補: `rotari_list_basedirs()`, `rotari_get_basedir_overview(basedir_ref)`。この叩き台では MCP server 起動時の設定で一つの masterdir を選び、tool の引数には `masterdir_scope` を出さない案を置く。
- 議論したい点: basedir の表示名をどう付けるか。物理パスを完全に隠すか、利用者が見分けるための限定的な説明を返すか。registry に残るが存在しない basedir を一覧に含めるか。

Web UI との対応を考える際は、既存 Web が basedir を選択してから project / run を見ること、履歴検索では利用者が選んだ複数 basedir scope を検索できることを参考にする。ただし Web の basedir ID は path の base64 表現であり、秘密の opaque ID ではない。MCP でも同じ ID を使うか、別の opaque reference にするかは未決。

### 2. 「その basedir のどの project を見る？」

- 候補情報: project 名、最近の run、現在実行中か、queue に待機 job があるか、成功/失敗の概要。
- agent に嬉しい点: basedir を一段選んだ後、その中でユーザーの指す project を発見できる。
- 例となる問い合わせ: 「この basedir の project を見せて」「どの project で今 run が動いている？」
- 例となる tool 候補: `rotari_list_projects(basedir_ref)`, `rotari_get_project_overview(basedir_ref, project_name)`。
- 議論したい点: Basedir の一覧応答に project 一覧まで同梱するか、件数や概要だけにして別要求で読むか。

### 3. 「最近どうなっている？」「どの run を見る？」

- 候補情報: basedir_ref、project 名、run id/name、状態、開始・終了時刻、所要時間、成功/失敗/未完了数、実行中 job、queue の変更有無。
- agent に嬉しい点: 詳細情報を読む前に対象 run を絞り、前回と今回、成功 run と失敗 run を選べる。
- 例となる問い合わせ: 「最新の失敗 run は？」「今動いている run を教えて」「直近3回の結果を比べたい」
- 例となる tool 候補: `rotari_list_runs(basedir_ref, project_name, status?, limit?)`, `rotari_get_run_summary(basedir_ref, project_name, run_id)`。
- 議論したい点: 最新 run の要約を常に返すか、明示検索させるか。履歴の既定期間やページ幅はいくつがよいか。

### 4. 「何が失敗した？」「この job はどういう job？」

- 候補情報: basedir_ref、project 名、run id、job id/name、stage、array/matrix の親子関係、依存関係、command、状態、exit code、failure kind、attempt 履歴、executor/host、開始・終了時刻。
- agent に嬉しい点: 同じエラーの複数 task や、retry 前後の結果を混同せず、原因がどの単位にあるか把握できる。
- 例となる問い合わせ: 「失敗した job と原因を一覧」「この job は何に依存している？」「retry で何が変わった？」
- 例となる tool 候補: `rotari_list_jobs(basedir_ref, project_name, run_id, filters?)`, `rotari_get_job(basedir_ref, project_name, run_id, job_id)`。
- 議論したい点: command や working directory を返す必要があるか。環境変数・host・実行パスに含まれる秘密情報をどう扱うか。attempt の全履歴と最終結果をどう区別するか。

### 5. 「ログのどこに手がかりがある？」

- 候補情報: basedir_ref、project 名、keyword の一致箇所、stdout/stderr、run/job/attempt、行番号、前後の短い excerpt、検索範囲と省略件数。
- agent に嬉しい点: `grep` 用の file path を知らなくても、失敗メッセージを探し、該当箇所だけ追加取得できる。
- 例となる問い合わせ: 「project 内で CUDA error を探して」「この job の最初の例外と直前のログを見せて」
- 例となる tool 候補: `rotari_search_logs(basedir_ref, project_name?, keyword, run_id?, job_id?, stream?)`, `rotari_get_log_excerpt(basedir_ref, project_name, run_id, job_id, stream, match_ref?)`。
- 議論したい点: 検索対象を最新 run、指定 run、全履歴のどれにするか。正規表現や大小文字無視が必要か。ログ量と出力上限をどう決めるか。

### 6. 「原因をどう解釈すればよい？」

- 候補情報: basedir_ref、project 名、run/job 参照、rotari の rule-based diagnosis、evidence、suggestion、diagnosis が古くなっていないか。
- agent に嬉しい点: 生ログだけを渡すより、既知の失敗パターンと根拠を結び付け、追加調査の出発点にできる。
- 例となる問い合わせ: 「rotari の診断結果と根拠を見せて」「診断と実際のログは一致している？」
- 例となる tool 候補: job/run 詳細に diagnosis を含める、または `rotari_get_diagnosis` を分ける。
- 議論したい点: diagnosis を job 詳細に含めるか独立して問い合わせるか。evidence をどの程度返すか。agent に提案させる際、確定診断と仮説をどう区別するか。

### 7. 「前回と何が違う？」「retry で改善した？」

- 候補情報: basedir_ref、project 名、run 間の job 増減、command/config 差分、成功/失敗の変化、所要時間、exit code、diagnosis、出力差分への参照。
- agent に嬉しい点: 単にログを読むだけでなく、変化点から回帰や修正の効果を説明できる。
- 例となる問い合わせ: 「成功した run と失敗した run の違いは？」「retry でどの job が直った？」
- 例となる tool 候補: `rotari_compare_runs(basedir_ref, project_name, run_a, run_b)`, `rotari_get_lineage(basedir_ref, project_name, run_id)`。
- 議論したい点: 比較に必要な最小情報と、出力差分・大きな artifact を扱う方法。

### 8. 「次に何ができる？」

- 候補情報: readiness check、未完了の前提、失敗 job を再実行する場合の対象、実行中/停止中の制御可能性。
- agent に嬉しい点: 調査結果から次の操作へつなげられる。ただし提案と実際の変更・実行は区別する。
- 例となる問い合わせ: 「再実行できる？」「失敗したものだけ retry する計画を見せて」「この run を止める必要がある？」
- 例となる tool 候補: 後段で `check` / `preview_retry` / `start_run` / `cancel` 相当を検討する。
- 議論したい点: 読み取り・提案・変更・実行のどの段階で人の承認を挟むか。二重呼び出しや timeout にどう対応するか。

## 情報を返す形のアイデア

- **一覧 → 要約 → 詳細 → excerpt** のように、agent が必要な分だけ深掘りできる段階構成。
- 各結果に、次の問い合わせで使える project/run/job/attempt の識別子を付ける。
- ID だけでなく、人が確認できる job 名・run 名も添える。
- 「結果なし」「検索範囲」「省略あり」「診断なし」「古い情報」など、情報の限界も明示する。
- JSON の機械可読フィールドと、短い自然言語の説明を併せる案。どちらが agent に有効か実験する。
- 巨大なログや artifact は全文を押し込まず、該当箇所と追加取得のための参照を返す。
- 複数候補がある場合、候補の差が分かる情報を返し、勝手に選ばない。

## どの MCP 機能に載せるかのアイデア

- **Tools**: 「検索する」「一覧する」「比較する」など、引数を受けて rotari 側で処理する操作の候補。
- **Resources**: run summary や diagnosis report のように、識別子で指定して読み取る情報を VS Code がどう提示するかを見て検討する候補。resource picker / 自動コンテキスト注入が役立つかは未検証。
- **Prompts**: 「失敗 job を調べる手順」のような定型の調査手順を提供する候補。状態データや設定値を保存する用途とは分ける。
- これらは排他的とは限らないが、まず必要な情報・操作を整理し、クライアントでの使われ方を見てから選ぶ。

## 後から検討できる変更・実行のアイデア

- queue 内容の確認や追加・変更案の preview。
- 実行可能性の check と、実行対象の説明。
- 非同期 run の開始、進捗の確認、結果待ち。
- failed / unfinished を rotari の既存規則で選んだ retry。
- job / run の cancel、suspend、resume。

これらを agent に許すべきか、preview / 承認 / 実行をどう分けるかは、読み取り情報の価値を試してから決める。

## 実装に進む場合に守りたい制約（案）

- basedir / file path の暗黙探索を agent に任せない。project を特定できる文脈を MCP 側で扱う。
- basedir ごとの情報を混ぜない。一覧は basedir 単位で出し、詳細問い合わせもどの basedir の project / run / job かを保つ。必要なら複数 basedir の横断検索を明示的な別案として検討する。
- 同じ名前や ID の候補がある場合に、別 project の job/log を誤って返さない。
- 状態解決や選択規則を MCP 独自に再実装せず、rotari の既存ルールを利用する。
- 環境変数や認証情報などを不用意に返さない。ログや command に含まれる機密情報の扱いも考える。
- ログ検索結果には、どの範囲を調べたか、何件省略したか、次にどう深掘りするかを含める。

## アイデアを絞るための試し方

まず一つの masterdir に複数 basedir が登録された状態を用意し、実際の問い合わせ例で agent が解決するのに必要な情報を洗い出す。たとえば:

1. 「registry に登録されている basedir と、それぞれの最近の状態を見せて」
2. 「この basedir の最新 run で失敗した job と、失敗理由を要約して」
3. 「この basedir の run と前回成功 run の違いを見て」
4. 「この basedir のログで CUDA error を探して、前後の文脈と該当 job を示して」
5. 「同じ project 名が複数 basedir にあるとき、候補を混同せず表示して」
6. 「失敗した job だけ再実行すると何が対象になるか説明して」

それぞれについて、agent に最初から与える情報、最初の問い合わせで返す情報、追加問い合わせに回す情報、最終回答に必要な情報を分ける。既存 CLI を agent に使わせた場合と仮の MCP tool を使わせた場合を比べ、「コマンドを間違えにくいか」「path や basedir をユーザーに聞かずに済むか」「結果を正しく次の問い合わせへつなげられるか」を評価する。

## まだ決めないこと

- Tools / Resources / Prompts のどれを使うか。
- MCP server の実装言語、SDK、transport、配布形態。
- registry に登録された basedir をどこまで含めるか（存在しない登録の扱いを含む）。
- basedir ごとの情報を返す粒度と、Web UI に似た複数 basedir 横断検索を提供するか。
- basedir を識別する参照形式（Web UI と同様の可逆 ID か、opaque reference か）。
- 「basedir + project + run + job」など、次の操作に使う参照の形式。
- ログ検索の既定範囲、全文を返すか excerpt にするか、件数や byte 上限。
- 読み取りに加え、queue 変更や run 操作まで agent に許可するか。
- VS Code 専用 extension を作る必要があるか。

## 参考にする既存実装

- [basedirregistry](../internal/basedirregistry/): masterdir 配下の registry から basedir を登録・列挙する。
- [Web UI の basedir 選択と scope](../internal/webui/webui.go) および [history search](../internal/webui/history_search.go): 登録済み basedir を列挙・選択し、basedir ごとに project / run を扱う。履歴検索では選択した複数 scope を指定できる。
- [Web projection の履歴検索モデル](../internal/web/search.go): basedir、project、run、job を検索結果に保持する。

これらは MCP の要件を確定するものではなく、既存 UI の情報構造や解決規則を再利用できるか調べる際の手掛かりとする。
