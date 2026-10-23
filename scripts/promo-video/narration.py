"""Narration for the promo short: Kokoro TTS clips, timing, audio track, subtitles.

Each scene may carry `say`, one English line, and `sayAt`, when in the scene
the line starts (default LEAD seconds), and `sfx`, [at, WAV] to play a sound
from the material (such as a synthesized sample) without a subtitle.
`narrate` synthesizes the lines
with kokoro-onnx (cached by text, voice, and speed), lengthens any scene too
short for its line, and returns the clips placed on the timeline. `write_audio`
mixes them into one WAV and `write_srt` writes matching subtitles.
"""

import hashlib
from pathlib import Path

import numpy as np
import soundfile as sf

LEAD = 0.25  # seconds from a scene's start to its line
TAIL = 0.35  # silence kept after a line before the next scene
PEAK = 0.89  # -1 dBFS


def synthesize(lines, model_dir, cache_dir, voice, speed):
    """Return {text: (path, seconds, sample_rate)}, reusing cached clips."""
    cache = Path(cache_dir)
    cache.mkdir(parents=True, exist_ok=True)
    kokoro = None
    out = {}
    for text in lines:
        key = hashlib.sha1(f"{voice}|{speed}|{text}".encode()).hexdigest()[:16]
        path = cache / f"{key}.wav"
        if not path.exists():
            if kokoro is None:
                from kokoro_onnx import Kokoro

                kokoro = Kokoro(
                    f"{model_dir}/kokoro-v1.0.onnx", f"{model_dir}/voices-v1.0.bin"
                )
            audio, rate = kokoro.create(text, voice=voice, speed=speed, lang="en-us")
            sf.write(path, audio, rate)
        info = sf.info(path)
        out[text] = (path, info.frames / info.samplerate, info.samplerate)
    return out


def narrate(scenes, model_dir, cache_dir, voice, speed):
    """Fit scenes to their lines and sounds; return [(start, seconds, path, text)].

    A sound's text is None, so it gets no subtitle.
    """
    clips = synthesize(
        [s["say"] for s in scenes if s.get("say")], model_dir, cache_dir, voice, speed
    )
    placed, start = [], 0.0
    for s in scenes:
        s["start"] = round(start, 3)
        if s.get("say"):
            path, seconds, _ = clips[s["say"]]
            at = s.get("sayAt", LEAD)
            s["dur"] = max(s["dur"], at + seconds + TAIL)
            s["sub"] = [at, at + seconds, s["say"]]
            placed.append((start + at, seconds, path, s["say"]))
        if s.get("sfx"):
            at, path = s["sfx"]
            info = sf.info(path)
            seconds = info.frames / info.samplerate
            s["dur"] = max(s["dur"], at + seconds + TAIL)
            placed.append((start + at, seconds, path, None))
        start += s["dur"]
    return placed, start


def write_audio(placed, total, path):
    rates = {sf.info(p).samplerate for _, _, p, _ in placed}
    if len(rates) != 1:
        raise ValueError(f"clips have different sample rates: {sorted(rates)}")
    rate = rates.pop()
    track = np.zeros(int(total * rate) + 1, dtype=np.float32)
    for start, _, clip, _ in placed:
        audio, _ = sf.read(clip, dtype="float32", always_2d=True)
        audio = audio.mean(axis=1)
        i = int(start * rate)
        track[i : i + len(audio)] += audio[: len(track) - i]
    peak = float(np.abs(track).max())
    if peak > 0:
        track *= PEAK / peak
    sf.write(path, track, rate)


def write_srt(placed, path):
    def stamp(t):
        ms = round(t * 1000)
        return f"{ms // 3600000:02}:{ms // 60000 % 60:02}:{ms // 1000 % 60:02},{ms % 1000:03}"

    spoken = [p for p in placed if p[3] is not None]
    cues = [
        f"{i}\n{stamp(start)} --> {stamp(start + seconds)}\n{text}\n"
        for i, (start, seconds, _, text) in enumerate(spoken, 1)
    ]
    Path(path).write_text("\n".join(cues))
