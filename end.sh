#!/bin/bash

exeName="gate"

#通用停止当前目录启动的进程
#kill -9 发送 SIGKILL信号
#ps -aux | grep "$(pwd)" | grep -v "grep" | awk '{print $2}' | xargs kill -9
#15 发送 SIGTERM信号，允许程序优雅退出
#ps -aux | grep "$(pwd)" | grep -v "grep" | awk '{print $2}' | xargs kill -15

function stop(){
    pid=$(ps -aux | grep "$(pwd)/$1" | grep -v "grep" | awk '{print $2}')
    if [ -n "$pid" ]; then
        echo "stop $1 pid: $pid"
        echo "$pid" | sudo xargs kill "$2"
    else
        echo "No process:$1 to kill"
    fi
}

function stop15(){
    stop "$1" -15
}

stop15 $exeName
