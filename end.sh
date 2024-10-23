#!/bin/bash

#通用停止当前目录启动的进程
#kill -9 发送 SIGKILL信号
#ps -aux | grep "$(pwd)" | grep -v "grep" | awk '{print $2}' | xargs kill -9
#15 发送 SIGTERM信号，允许程序优雅退出
ps -aux | grep "$(pwd)" | grep -v "grep" | awk '{print $2}' | xargs kill -15
