#!/bin/sh
# One point of a 48-run sweep. Two kinds of failure, both gone after a plain
# `rotari retry` once sweep.conf is fixed:
#   GPU fault on tasks 7 19 33, first attempt only (a machine problem),
#   CUDA OOM on tasks 4 12 25 30 41 while max_batch is above 64.
. ./sweep.conf
t=${ROTARI_ARRAY_TASK_ID}
lr="1e-$((t % 4 + 3))"
echo "task $t: lr=$lr max_batch=$max_batch"
sleep "$((t % 3 + 1)).$((t * 7 % 10))"
case " 7 19 33 " in *" $t "*)
    if [ ! -e "$HOME/.gpu-faulted-$t" ]; then
        touch "$HOME/.gpu-faulted-$t"
        echo "NVRM: Xid (PCI:0000:3b:00): 79, pid=4121, GPU has fallen off the bus." >&2
        exit 1
    fi
    ;;
esac
case " 4 12 25 30 41 " in *" $t "*)
    if [ "$max_batch" -gt 64 ]; then
        echo "torch.OutOfMemoryError: CUDA out of memory. Tried to allocate 4.00 GiB" >&2
        exit 1
    fi
    ;;
esac
sleep 1.$((t % 10))
echo "task $t: eval_loss=0.$((40 + t % 17))"
