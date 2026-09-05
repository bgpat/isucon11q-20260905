---
title: "競技結果を GitHub Release に記録する"
labels:
  - phase:after-competition
  - type:documentation
  - type:task
depends_on:
  - final-benchmark.md
---

# 目的

競技終了後に、最終状態、スコア、変更内容、判断の根拠を GitHub Release として残す。

# 前提条件

- default branch にマージされた commit を正とする。
- 実測していないスコアや結果を補完しない。

# 対象サーバー

なし。GitHub 上の記録作業。

# 作業

- default branch にマージされた commit を確認する。
- タイムスタンプとスコアを含む名前で GitHub Release を作成する。
- 変更内容の要約を記載する。
- 関連 Issue と Pull Request へのリンクを記載する。
- ベンチマーク結果、既知の問題、未実施の計測を記録する。
- 実測していないスコアや結果を補完しない。

## 実行例

コマンドは実行例である。問題や環境によって構成、パス、サービス名、接続方法は異なる。Issue に記録したユーザー提供情報と実機の状態に合わせて読み替える。

```bash
REPO=dbgofy/isucon11q-20260905
BRANCH=master
SCORE=REPLACE_WITH_MEASURED_SCORE
STAMP=$(date +%Y%m%d-%H%M%S)
TAG="${STAMP}-${SCORE}"
```

## 1. 対象 commit を確認する

```bash
git fetch origin
git log -1 --oneline "origin/${BRANCH}"
git status
```

## 2. GitHub Release を作成する

```bash
gh release create "$TAG" \
  --repo "$REPO" \
  --target "$BRANCH" \
  --title "${STAMP} score ${SCORE}" \
  --notes "$(cat <<EOF
## 要約

- 最終スコア: ${SCORE}（実測）
- 対象 commit: $(git rev-parse "origin/${BRANCH}")

## 関連

- Issue:
- Pull Request:

## ベンチマーク

- 実行日時:
- 対象サーバー:
- 結果の保存先:

## 既知の問題 / 未実施の計測

- 
EOF
)"
```

計測ログを付ける場合は、秘密情報を含まないファイルだけを asset にする。

```bash
# gh release upload "$TAG" /tmp/isucon-results/YYYYMMDD-HHMMSS-final/score.txt --repo "$REPO"
```

# 確認方法

```bash
gh release view "$TAG" --repo "$REPO"
```

# 失敗時の復旧

- tag が衝突する: `${STAMP}` を含めた名前で作り直す。
- スコア未確定: GitHub Release を作らず、確定してから実行する。仮の数値で tag しない。

# 完了条件

- GitHub Release から対象 commit、変更内容、関連 Issue・PR、スコアを確認できる。
- 推測や未確認事項が明示されている。

# 作業後の記録

- 実行日時:
- GitHub Release URL:
- tag:
