#!/usr/bin/env bash
# 交叉编译全部平台二进制到 build/（在 Linux 上执行）
# 提示：build/ 已在 .gitignore 忽略，不会提交；部署时把所需产物拷到目标机。
#   Linux amd64 服务器:   cp build/ProxyToClash_linux_amd64 <目标机>/ProxyToClash
#   Android 不跑转换器，无需二进制。
set -e
mkdir -p build
CGO_ENABLED=0
echo "== build linux/amd64 =="
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o build/ProxyToClash_linux_amd64 .
echo "== build linux/arm64 (树莓派/NAS) =="
GOOS=linux GOARCH=arm64 go build -trimpath -ldflags "-s -w" -o build/ProxyToClash_linux_arm64 .
echo "== build windows/amd64 =="
GOOS=windows GOARCH=amd64 go build -trimpath -ldflags "-s -w" -o build/ProxyToClash_windows_amd64.exe .
echo "== 完成，产物在 build/ =="
ls -lh build