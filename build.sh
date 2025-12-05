#!/bin/bash

# 编译 Linux x86_64 版本的程序

set -e

echo "开始编译 Linux x86_64 版本..."

# 设置编译环境变量
export GOOS=linux
export GOARCH=amd64

# 编译
go build -o cpumembusy .

echo "编译完成！"
echo "输出文件: ./cpumembusy"

