#!/bin/bash

exeName="gate"

echo "use $exeName"
#增加执行权限
chmod +xxx "./$exeName"

#exe后面带上 & 防止关闭终端，就把go进程结束掉;而且必须以shell脚本的形式启动 & 才能起作用。
#直接在终端中敲下面的命令关掉终端，进程还是结束了。。

ulimit -c unlimited
export GOTRACEBACK=crash

#多个服务只是目录的不同
$(pwd)/$exeName &
#$(pwd)/$exeName -env debug &

