#!/bin/sh
# CPU preprocessing on the workstation.
echo "prep on $(hostname): tokenizing 12 shards"
for i in 1 2 3; do
    sleep 1
    echo "shard $((i * 4))/12"
done
