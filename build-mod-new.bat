@echo off	

echo ��ǰ�̷���%~d0
echo ��ǰ·����%cd%
echo ��ǰִ�������У�%0
echo ��ǰbat�ļ�  ·����%~dp0
echo ��ǰbat�ļ���·����%~sdp0

:: ��ǰ�̷�
%~d0
:: ��ǰĿ¼
cd %~dp0
:: ����Ŀ¼
cd ./main
set GOARCH=amd64
set GOOS=linux
set GOTRACEBACK=all
echo build...
go build -o ../bin/gate
pause

@echo on
