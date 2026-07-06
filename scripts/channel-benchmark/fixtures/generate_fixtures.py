"""Generate deterministic benchmark fixture assets.

Run from the repository root:

    python3 scripts/channel-benchmark/fixtures/generate_fixtures.py
"""

from __future__ import annotations

import math
import shutil
import struct
import subprocess
import wave
from pathlib import Path

from PIL import Image, ImageDraw


BASE = Path(__file__).resolve().parent


def _save_rgb(name: str, size: tuple[int, int], draw_fn) -> None:
    path = BASE / "images" / name
    path.parent.mkdir(parents=True, exist_ok=True)
    image = Image.new("RGB", size, "white")
    draw = ImageDraw.Draw(image)
    draw_fn(draw)
    image.save(path, optimize=True)


def generate_images() -> None:
    _save_rgb(
        "edit-source-256.png",
        (256, 256),
        lambda d: (
            d.rectangle([0, 0, 255, 255], fill=(245, 248, 252)),
            d.ellipse([54, 54, 202, 202], fill=(36, 120, 220), outline=(20, 70, 140), width=4),
            d.rectangle([112, 32, 144, 224], fill=(255, 255, 255)),
            d.rectangle([32, 112, 224, 144], fill=(255, 255, 255)),
        ),
    )
    _save_rgb(
        "edit-mask-256.png",
        (256, 256),
        lambda d: (
            d.rectangle([0, 0, 255, 255], fill=(0, 0, 0)),
            d.ellipse([84, 84, 172, 172], fill=(255, 255, 255)),
        ),
    )
    _save_rgb(
        "reference-square-128.jpg",
        (128, 128),
        lambda d: (
            d.rectangle([0, 0, 127, 127], fill=(250, 250, 250)),
            d.rectangle([32, 32, 96, 96], fill=(220, 60, 60), outline=(120, 20, 20), width=3),
            d.line([0, 0, 127, 127], fill=(30, 30, 30), width=2),
        ),
    )


def generate_audio() -> None:
    path = BASE / "audio" / "tone-440hz-400ms.wav"
    path.parent.mkdir(parents=True, exist_ok=True)
    sample_rate = 16_000
    duration = 0.4
    with wave.open(str(path), "w") as wav:
        wav.setnchannels(1)
        wav.setsampwidth(2)
        wav.setframerate(sample_rate)
        for i in range(int(sample_rate * duration)):
            value = int(0.25 * 32767 * math.sin(2 * math.pi * 440 * i / sample_rate))
            wav.writeframes(struct.pack("<h", value))


def generate_video() -> None:
    ffmpeg = shutil.which("ffmpeg")
    if not ffmpeg:
        print("ffmpeg not found; keeping existing video fixture if present")
        return
    path = BASE / "videos" / "reference-160x90-1s.mp4"
    path.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(
        [
            ffmpeg,
            "-y",
            "-f",
            "lavfi",
            "-i",
            "testsrc=size=160x90:rate=8:duration=1",
            "-pix_fmt",
            "yuv420p",
            "-movflags",
            "+faststart",
            str(path),
        ],
        check=True,
        stdout=subprocess.DEVNULL,
        stderr=subprocess.DEVNULL,
    )


def main() -> None:
    generate_images()
    generate_audio()
    generate_video()
    print(f"fixtures written under {BASE}")


if __name__ == "__main__":
    main()

