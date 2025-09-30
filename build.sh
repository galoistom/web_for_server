#!/bin/bash
echo "building for windows+x86..."
GOOS=windows GOARCH=amd64 go build -o ./build/web_for_server_win_x86.exe
echo "building for windows+arm..."
GOOS=windows GOARCH=arm64 go build -o ./build/web_for_server_win_arm.exe
echo "building for macos+x86..."
GOOS=darwin GOARCH=amd64 go build -o ./build/web_for_server_mac_x86
echo "building for macos+arm..."
GOOS=darwin GOARCH=arm64 go build -o ./build/web_for_server_mac_arm
echo "building for linux+x86..."
GOOS=linux GOARCH=amd64 go build -o ./build/web_for_server_linux_x86
echo "building for linux+arm"
GOOS=linux GOARCH=arm64 go build -o ./build/web_for_server_linux_arm
