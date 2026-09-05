# 共通タスク候補

このディレクトリには、本番と練習のどちらでも使える一回限りのタスク候補を置く。
各 Markdown ファイルは GitHub Issue のタイトル、label、本文の雛形として利用する。

各タスクには、目的に加えて前提条件、対象、作業、確認方法、失敗時の復旧、完了条件、作業後の記録欄を書く。

「作業」には実施項目を箇条書きする。コマンドは「実行例」として段落を分ける。
実行例の説明を書く場合も、その「実行例」の中に書く。
コマンドは問題や環境によって構成、パス、サービス名、言語、接続方法が異なるため、Issue に記録したユーザー提供情報と実機の状態に合わせて読み替える。推測で埋めない。

## Issue 化

1. 対象ファイルの front matter から `title` と `labels` を確認する。
2. `body` 以下の内容を Issue 本文として使用する。
3. 実施対象の競技、サーバー情報、関連タスクを追記する。
4. `phase:*` と `type:*` の label を付けて Issue を作成する。

各タスクの `depends_on` には、先に完了させるタスクのファイル名を記載する。
依存関係がないタスクは `depends_on: []` とする。
Issue 化時には `depends_on` を依存先IssueへのGitHub Issue dependency（blocked by）として登録する。

リポジトリ作成時に `on-created` workflow が有効な場合は、`tasks/*.md`（`README.md` を除く）から
自動的に Issue が作成される。自動作成後に、対象競技やサーバー情報など問題ごとの内容を追記する。
作成された「ISUCON 共通タスク」Issue の sub-issue として登録される。
`depends_on` はsub-issueの親子関係ではなく、複数の依存先を表現できるIssue dependency（blocked by）として登録する。共通タスクのsub-issueの親を依存先へ変更してはならない。

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
