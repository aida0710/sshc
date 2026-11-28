#!/usr/bin/env bash
#
# staticcheck を、Linux・Windows・macOS・Android の組み立てでまとめて流す。
#
# 1 つの OS の組み立てで流すと、ほかの OS だけのファイル（_windows.go、_darwin.go など）は
# 解析されない。-matrix に組み立てを並べると全部を解析し、U1000（使われていないもの）は
# どの組み立てでも使われないものだけを報告する。規則の選び方は staticcheck.conf にある。
set -euo pipefail

cd "$(dirname "$0")/../.."

# staticcheck を組み立てる Go と、解析する package の型の情報（export data）を書き出す Go を、
# go.mod の版にそろえる。新しい版の Go（手元の Go 1.27 など）が書いた型の情報を、この版の
# staticcheck は読めず、どのファイルも解析できないまま内部エラーを報告する。
GOTOOLCHAIN="go$(sed -n 's/^go //p' go.mod)"
export GOTOOLCHAIN

# 1 行が 1 つの組み立てで、「名前: 環境変数」の形。名前は報告の [windows] のような印になる。
# macOS と Android は、その OS の C コンパイラが無くても型を検査できるよう cgo を切る。
# このため、macOS で cgo を要るファイル（internal/serialtransport/backend_detailed.go）は、
# macOS の組み立てとしては解析しない。
go tool staticcheck -matrix ./... <<'MATRIX'
linux: GOOS=linux GOARCH=amd64
windows: GOOS=windows GOARCH=amd64
darwin: GOOS=darwin GOARCH=arm64 CGO_ENABLED=0
android: GOOS=android GOARCH=arm64 CGO_ENABLED=0
MATRIX
echo "staticcheck: Linux・Windows・macOS・Android の組み立てで指摘は無い"
