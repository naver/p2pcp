#!/bin/bash

tmp=$(mktemp /tmp/p2pcp.XXXXXX)

cd $1
{ find . -ls | awk '{printf("%s ",$3); for(i=11; i<=NF; i++){printf("%s ",$i)}; printf("\n");}' ; } > $tmp
find . -type f -exec sha1sum {} \; >> $tmp
CHECKSUM=$(sha1sum $tmp | awk '{print $1}')

rm $tmp
echo $CHECKSUM
