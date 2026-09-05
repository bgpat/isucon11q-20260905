# 共通タスク候補

このディレクトリには、本番と練習のどちらでも使える一回限りのタスク候補を置く。
各 Markdown ファイルは GitHub Issue のタイトル、label、本文の雛形として利用する。

## Issue 化

1. 対象ファイルの front matter から `title` と `labels` を確認する。
2. `body` 以下の内容を Issue 本文として使用する。
3. 実施対象の競技、サーバー情報、関連タスクを追記する。
4. `phase:*` と `type:*` の label を付けて Issue を作成する。

各タスクの `depends_on` には、先に完了させるタスクのファイル名を記載する。
依存関係がないタスクは `depends_on: []` とする。
Issue 化時には `depends_on` が依存先 Issue への `Depends on #番号` として本文に反映される。

リポジトリ作成時に `on-created` workflow が有効な場合は、`tasks/*.md`（`README.md` を除く）から
自動的に Issue が作成される。自動作成後に、対象競技やサーバー情報など問題ごとの内容を追記する。
作成された「ISUCON 共通タスク」Issue の sub-issue として登録される。
依存関係のあるタスクは、リポジトリ作成時に限らず、親となる Issue の sub-issue として登録する。

phase label は作業を行う時期を表す。

- `phase:day-before` — 前日準備
- `phase:just-before` — 競技直前
- `phase:start` — 競技開始直後
- `phase:early-competition` — 競技序盤
- `phase:mid-competition` — 競技中盤
- `phase:final-check` — 競技終盤
- `phase:after-competition` — 競技終了後

タスク候補は問題固有のエンドポイント名、テーブル名、解答手順を含めない。
問題固有の改善は、計測結果に基づいて競技中に個別の Issue として作成する。
