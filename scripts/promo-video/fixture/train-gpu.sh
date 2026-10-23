#!/bin/sh
# GPU training submitted to Slurm.
echo "train on $(hostname)"
for epoch in 1 2 3; do
    sleep 1.5
    echo "epoch $epoch/3  loss=0.$((70 - epoch * 12))"
done
