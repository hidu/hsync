hsync
===
基于fsnotify的实时文件同步工具  

可用来帮助我们码农将本地办公电脑上的代码实时同步并部署到远程测试环境，以达到实时预览效果的目的。  


【办公电脑】  -----（变化的文件）--（实时）---->   【测试机】  


## install

```
go get -u github.com/hidu/hsync
```


## useage
### server:
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
5. "{pwd}"：是配置文件当前目录
6. home：本地保存接收文件的目录，可以是相对于配置文件的相对路径，也可以是绝对路径

deployCmd运行时的实际参数：
>bash deploy.sh dst_path src_path update  

即运行时会添加上参数 `dst_path src_path update`,`deploy.sh`脚本可以自己依据参数做一些业务逻辑  

>bash deploy.sh /home/work/app/phpsrc/index.php phpsrc/index.php update

#### deploy.sh demo
```
#!/bin/bash

DST=$1
SRC=$2
if [ "$SRC" == "a/d1" ];then
 grep -rl hello $DST |xargs -n 1 sed -i s/hello/nihao/g
fi
```

####force deploy all
>hsync -deploy hsyncd.json



### client:
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
说明：  
1. token:验证用，客户端和服务端必须保持一致  
2. home:是待同步文件的目录（也就是代码的workspace），可以是相对于配置文件的相对路径，也可以是绝对路径   
3. ignore：不同步到远端的忽略文件列表  
4. server: 服务端地址  

默认忽略的文件：
>.*  
>*~  