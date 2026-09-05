# Issue #8・#9・#17・#19・#22・#23 確認結果

確認日: 2026-09-06（Asia/Tokyo）

## 共通

- `README.md`、`files/vimrc`、対象 task 手順書を確認した。
- 既存の変更（`.github/workflows/on-created.yml`、`AGENTS.md`、`tasks/README.md`、`results/`）は変更していない。
- GitHub API は `github.com` の名前解決に失敗し、`gh auth status` でも保存済み認証情報が無効だった。Issue へのコメント・クローズは未実行。

## 判定

### #8 運営提供のインフラを適用する

- 依存: `prepare-repo.md`、`check-portal.md`
- ローカルに CloudFormation、Terraform、Ansible、インベントリ、配布インフラ定義は見つからなかった。
- ポータル URL、マニュアル、配布物はユーザーから提供されていない。
- 依存先の #11（ポータル確認）が未完了のため、運営提供なしの完了記録も確定できない。
- 判定: 未完了。推測でインフラを適用しない。

### #9 DB の初期データなど Git 管理対象外のデータをバックアップする

- 依存: `initialize-servers.md`
- 対象サーバー、DB 配置、バックアップ対象、概算サイズ、ベンチマーク停止時間が不明。
- サイズまたは影響が不明な場合はユーザー確認が必要な手順であり、SSH・dump・Release asset 作成は実行していない。
- 判定: 未完了。

### #17 競技中の連絡手段と通知を準備する

- 依存: `prepare-repo.md`
- チームメンバー、通知先、共有先 Issue / Pull Request、通知確認者の情報がない。
- 通知テストコメントなどの外部状態変更は実行していない。
- 判定: 未完了。手順書確認まで。

### #19 ローカルの接続設定を用意する

- 依存: `prepare-repo.md`、`apply-provided-infra.md`
- サーバー名、IP / DNS、SSH 鍵のリポジトリ外パス、名前解決要否が不明。
- 推測で `~/.ssh/config`、`/etc/hosts`、SSH 接続先を変更していない。
- 判定: 未完了。ユーザー提供情報待ち。

### #22 GitHub Project を準備する

- 依存: `prepare-repo.md`
- 既存 Project、Project number / URL、自動追加 workflow、label 絞り込みを確認できていない。
- Project 作成、リンク、workflow、item 追加などの外部状態変更は実行していない。
- 判定: 未完了。既存設定または明確な作成要求がないため実行しない。

### #23 競技用リポジトリを準備する

- 依存: なし
- ローカルではリポジトリ、`master` の追跡先、`README.md`、`AGENTS.md`、`tasks/README.md`、既存ブランチを確認した。
- GitHub API が利用できず、公開範囲、default branch、メンバー権限、PR 設定、Update SSH Keys workflow の実行結果を確認できていない。
- メンバー招待、設定変更、workflow 実行は行っていない。
- 判定: 未完了。GitHub 接続と権限確認が必要。

## 次に必要な情報

- 有効な GitHub CLI 認証とネットワーク接続
- ポータル URL、マニュアル / 配布物の場所
- 競技サーバーの接続先、役割、初回 SSH ユーザー、鍵の場所
- チームメンバー、通知先、Project の既存設定または作成要求
