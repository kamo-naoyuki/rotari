"""Fake TTS training run for the "results" episode.

    tts_train.py --out DIR [--epochs N]

Logs a few epochs, then writes what a real run would leave behind: a loss
plot, synthesized samples, an evaluation table, and TensorBoard scalars. The
samples are copied from $TTS_SAMPLES (clips the capture made with Kokoro).
"""

import argparse
import csv
import math
import os
import shutil
import time
from pathlib import Path

import matplotlib

matplotlib.use("Agg")
import matplotlib.pyplot as plt
from tensorboard.compat.proto import event_pb2, summary_pb2
from tensorboard.summary.writer.event_file_writer import EventFileWriter

parser = argparse.ArgumentParser()
parser.add_argument("--out", required=True)
parser.add_argument("--epochs", type=int, default=6)
args = parser.parse_args()
out = args.out

steps = list(range(0, 3001, 50))
train = [1.9 * math.exp(-s / 700) + 0.32 + 0.03 * math.sin(s / 37) for s in steps]
val = [2.0 * math.exp(-s / 650) + 0.38 + 0.02 * math.sin(s / 53) for s in steps]

for epoch in range(1, args.epochs + 1):
    time.sleep(1.5)
    i = len(steps) * epoch // args.epochs - 1
    print(
        f"epoch {epoch}/{args.epochs}  step {steps[i]}  loss={train[i]:.3f}  val={val[i]:.3f}",
        flush=True,
    )

os.makedirs(out, exist_ok=True)

plt.style.use("dark_background")
fig, ax = plt.subplots(figsize=(6.4, 3.6), dpi=150)
ax.plot(steps, train, label="train", color="#4cc38a")
ax.plot(steps, val, label="validation", color="#f08b70")
ax.set_xlabel("step")
ax.set_ylabel("mel loss")
ax.set_title("FastSpeech2 · LJSpeech")
ax.legend()
fig.tight_layout()
fig.savefig(f"{out}/loss.png")

writer = EventFileWriter(f"{out}/tb")
for s, t, v in zip(steps, train, val, strict=True):
    for split, value in (("train", t), ("val", v)):
        tag = f"{split}/mel_loss"
        summary = summary_pb2.Summary(
            value=[summary_pb2.Summary.Value(tag=tag, simple_value=value)]
        )
        writer.add_event(
            event_pb2.Event(wall_time=time.time(), step=s, summary=summary)
        )
writer.close()

samples = Path(f"{out}/samples")
samples.mkdir(exist_ok=True)
source = Path(os.environ["TTS_SAMPLES"])
rows = []
for i, wav in enumerate(sorted(source.glob("*.wav")), 1):
    shutil.copy(wav, samples / wav.name)
    rows.append(
        {
            "utt_id": wav.stem,
            "text": (source / f"{wav.stem}.txt").read_text().strip(),
            "wav": f"samples/{wav.name}",
            "utmos": f"{4.31 - 0.07 * i:.2f}",
            "cer": f"{1.2 + 0.4 * i:.1f}",
        }
    )
with open(f"{out}/eval.csv", "w", newline="") as f:
    w = csv.DictWriter(f, fieldnames=list(rows[0]))
    w.writeheader()
    w.writerows(rows)
print(f"saved {len(rows)} samples, eval.csv, loss.png, tb/ to {out}")
