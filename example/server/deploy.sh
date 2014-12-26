#!/bin/bash

DST=$1
SRC=$2

if [ "$SRC" == "js/config.js" ];then
    grep -rl hello $DST |xargs -n 1 sed -i s/hello/nihao/g
fi