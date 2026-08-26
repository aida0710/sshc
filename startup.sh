#!/bin/sh
set -eu

# source checkoutから開発用engineを起動する。更新は取得するrevisionを確認できるよう
# このscriptでは行わず、別途git pull --ff-onlyを実行する。
make build
exec ./bin/sshc engine
