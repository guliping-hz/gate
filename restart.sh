#!/bin/bash

#通用重启当前目录的进程。
chmod +xxx ./start.sh
chmod +xxx ./end.sh

bash $(pwd)/end.sh
echo "sleep 2s..."
sleep 2s
bash $(pwd)/start.sh
