hsync
===
real time sync dir by fsnotify

## install

```
go get -u github.com/hidu/hsync
```
or download from 
[![Gobuild Download](http://gobuild.io/badge/github.com/hidu/hsync/downloads.svg)](http://gobuild.io/github.com/hidu/hsync)

## useage
###server slide:
>hsync -d hsyncd.json

```
{
    "server":":8700",
    "home":"./",
    "token":"abc",
    "deploy":[
        {"from":"a/","to":"d/"}
    ]
}
```
###client slide:
>hsync hsync.json  

```
{
    "server":"127.0.0.1:8700",
    "home":"./",
    "token":"abc",
    "ignore":[
        "*.exe"
    ]
}
```

default ignore files: .*,*~