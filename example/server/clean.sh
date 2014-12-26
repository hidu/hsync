#!/bin/bash
cd $(dirname $0)

rm data/* -rf
rm webroot/* -rf

echo "all clean" 