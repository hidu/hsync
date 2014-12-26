hsync
===
基于fsnotify的实时文件同步工具

## install

```
go get -u github.com/hidu/hsync
```

或直接下载编译好的二进制文件：
[![Gobuild Download](http://gobuild.io/badge/github.com/hidu/hsync/downloads.svg)](http://gobuild.io/github.com/hidu/hsync)

## useage
###server:
>hsync -d hsyncd.json

```
{
    "addr":":8700",
    "home":"./",
    "token":"abc",
    "deploy":[
        {"from":"a/","to":"d/"}，
        {"from":"phpsrc/","to":"/home/work/app/phpsrc/"}
    ],
     "deployCmd":""
}
```
说明：  
1. token:验证用，客户端和服务端必须保持一致  
2. deploy.from是以home为根目录的相对目录，deploy.to可以是相对目录或者决定目录  
3. deploy:同步完成后进行在对文件进行拷贝部署  
4. deployCmd：在每次deploy 之后运行，用来做一些自动化修改。如："bash {pwd}/deploy.sh"  
5."{pwd}"是配置文件当前目录  

deployCmd运行时的参数：
>bash deploy.sh dst_path src_path update  

>bash deploy.sh /home/work/app/phpsrc/index.php phpsrc/index.php update

####deploy.sh demo
```
#!/bin/bash

DST=$1
SRC=$2
if [ "$SRC" == "a/d1" ];then
 grep -rl hello $DST |xargs -n 1 sed -i s/hello/nihao/g
fi
```

###client:
>hsync hsync.json  

```
{
    "server":"127.0.0.1:8700",
    "home":"./",
    "token":"abc",
    "ignore":[
        "*.exe",
        "log/*"
    ]
}
```

默认忽略的文件：
>.*  
>*~  