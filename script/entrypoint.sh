#!/bin/bash
#이 파일로 Dockerfile ENTRYPOINT 지정하여 p2pcp 시작시 argument에 대해 bash command substiution을 가능하게 함.
eval p2pcp $@
