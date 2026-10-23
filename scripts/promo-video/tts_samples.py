"""Synthesize the clips the fake TTS job "produces", with Kokoro.

    tts_samples.py KOKORO_DIR OUT_DIR

Writes OUT_DIR/LJ001-000N.wav and .txt for three LJSpeech sentences.
"""

import sys
from pathlib import Path

import soundfile as sf
from kokoro_onnx import Kokoro

LINES = [
    (
        "LJ001-0001",
        "Printing, in the only sense with which we are at present concerned, "
        "differs from most if not from all the arts.",
    ),
    ("LJ001-0002", "In being comparatively modern."),
    (
        "LJ001-0003",
        "For although the Chinese took impressions from wood blocks engraved in relief.",
    ),
]

model, out = Path(sys.argv[1]), Path(sys.argv[2])
out.mkdir(parents=True, exist_ok=True)
kokoro = Kokoro(str(model / "kokoro-v1.0.onnx"), str(model / "voices-v1.0.bin"))
for uid, text in LINES:
    audio, rate = kokoro.create(text, voice="af_heart", speed=1.0, lang="en-us")
    sf.write(out / f"{uid}.wav", audio, rate)
    (out / f"{uid}.txt").write_text(text + "\n")
