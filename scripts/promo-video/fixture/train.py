# One training run for the opening shot.
import time

for epoch in range(1, 6):
    time.sleep(0.5)
    print(f"epoch {epoch}/5  loss={0.92 - epoch * 0.13:.3f}", flush=True)
print("saved checkpoint: ckpt/best.pt")
