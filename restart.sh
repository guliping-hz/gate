#!/bin/bash

#上传后，如果运行报错。更改回车符：goland->File->File Properties->Line Separators->LF-Unix & macOs

#通用重启当前目录的进程。
sudo chmod +xxx $(pwd)/start.sh
sudo chmod +xxx $(pwd)/end.sh

sudo $(pwd)/end.sh
echo "sleep 2s..."
sleep 2s
sudo $(pwd)/start.sh
