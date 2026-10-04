"""Write sample files of every previewable type for the static web demo.

Uses only the standard library, so the demo needs no numpy, imaging, or
audio packages. The files are small but valid: a PNG and an SVG plot, CSV and
TSV tables, a log, text, JSON, an .npy array and an .npz archive, a WAV tone,
an MP4 clip (copied from scripts/templates/demo), an opaque checkpoint, and a
directory of checkpoints.
"""

from __future__ import annotations

import argparse
import io
import json
import math
import shutil
import struct
import wave
import zipfile
import zlib
from pathlib import Path


def png(width: int, height: int) -> bytes:
    """Return an RGB PNG with a diagonal gradient."""
    rows = b"".join(
        b"\x00"
        + bytes(
            channel
            for x in range(width)
            for channel in (x * 255 // width, y * 255 // height, 160)
        )
        for y in range(height)
    )

    def chunk(kind: bytes, data: bytes) -> bytes:
        body = kind + data
        return struct.pack(">I", len(data)) + body + struct.pack(">I", zlib.crc32(body))

    header = struct.pack(">IIBBBBB", width, height, 8, 2, 0, 0, 0)
    return (
        b"\x89PNG\r\n\x1a\n"
        + chunk(b"IHDR", header)
        + chunk(b"IDAT", zlib.compress(rows))
        + chunk(b"IEND", b"")
    )


def npy(descr: str, shape: tuple[int, ...], data: bytes) -> bytes:
    """Return an .npy version 1.0 file."""
    dimensions = ", ".join(str(size) for size in shape) + (
        "," if len(shape) == 1 else ""
    )
    header = (
        f"{{'descr': '{descr}', 'fortran_order': False, 'shape': ({dimensions}), }}"
    )
    padding = 64 - (10 + len(header) + 1) % 64
    header += " " * padding + "\n"
    return (
        b"\x93NUMPY\x01\x00" + struct.pack("<H", len(header)) + header.encode() + data
    )


def tone(path: Path) -> None:
    """Write a one-second 440 Hz tone."""
    rate = 8000
    with wave.open(str(path), "wb") as output:
        output.setnchannels(1)
        output.setsampwidth(2)
        output.setframerate(rate)
        output.writeframes(
            b"".join(
                struct.pack(
                    "<h", int(8000 * math.sin(2 * math.pi * 440 * index / rate))
                )
                for index in range(rate)
            )
        )


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("workspace", type=Path)
    workspace = parser.parse_args().workspace
    results = workspace / "results"
    (results / "checkpoints").mkdir(parents=True, exist_ok=True)
    (workspace / "data").mkdir(exist_ok=True)

    (workspace / "data" / "train.csv").write_text("x,y\n0.1,0.3\n0.4,0.9\n0.7,1.6\n")
    (results / "metrics.csv").write_text(
        "epoch,loss,accuracy\n1,0.92,0.61\n2,0.55,0.78\n3,0.41,0.84\n"
    )
    (results / "predictions.tsv").write_text(
        "id\tlabel\tscore\n1\tcat\t0.93\n2\tdog\t0.71\n"
    )
    (results / "train.log").write_text(
        "".join(
            f"epoch {epoch}: loss {loss}\n"
            for epoch, loss in [(1, 0.92), (2, 0.55), (3, 0.41)]
        )
    )
    (results / "notes.txt").write_text("Learning rate 0.1 converged by epoch 3.\n")
    (results / "summary.json").write_text(
        json.dumps({"best_epoch": 3, "loss": 0.41, "accuracy": 0.84}, indent=2) + "\n"
    )
    (results / "loss.svg").write_text(
        '<svg xmlns="http://www.w3.org/2000/svg" width="240" height="120">'
        '<polyline fill="none" stroke="#4f46e5" stroke-width="3" points="20,20 120,70 220,90"/>'
        "</svg>\n"
    )
    (results / "plot.png").write_bytes(png(96, 64))
    weights = npy("<f4", (2, 3), struct.pack("<6f", 0.1, -0.2, 0.3, 0.4, -0.5, 0.6))
    (results / "weights.npy").write_bytes(weights)
    archive = io.BytesIO()
    with zipfile.ZipFile(archive, "w", zipfile.ZIP_DEFLATED) as members:
        members.writestr("weights.npy", weights)
        members.writestr("labels.npy", npy("<i8", (4,), struct.pack("<4q", 0, 1, 1, 0)))
    (results / "arrays.npz").write_bytes(archive.getvalue())
    tone(results / "tone.wav")
    shutil.copy(
        Path(__file__).parent / "templates" / "demo" / "clip.mp4", results / "clip.mp4"
    )
    (results / "model.pt").write_bytes(b"PK\x03\x04 demo checkpoint")
    for epoch in range(1, 4):
        (results / "checkpoints" / f"epoch-{epoch}.pt").write_bytes(b"demo checkpoint")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
