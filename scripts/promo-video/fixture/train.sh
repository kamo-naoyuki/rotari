#!/bin/sh
# A project's training job; vision's dataset path is wrong.
name=$(basename "$PWD")
echo "[$name] start on $(hostname)"
for epoch in 1 2 3 4 5 6; do
    sleep 2
    if [ "$name" = vision ] && [ "$epoch" = 2 ]; then
        echo "Traceback (most recent call last):" >&2
        echo "  File \"train.py\", line 31, in <module>" >&2
        echo "FileNotFoundError: [Errno 2] No such file or directory: 'data/imagenet/train'" >&2
        exit 1
    fi
    echo "[$name] epoch $epoch/6  loss=0.$((80 - epoch * 9))"
done
