# Channel Benchmark Fixtures

Small deterministic fixtures used by protocol and parameter compatibility tests.
They are generated locally, not downloaded, so licensing and reproducibility are
straightforward.

## Files

### Images

- `images/edit-source-256.png` — 256x256 PNG source image for image-edit
  multipart/base64/URL compatibility tests.
- `images/edit-mask-256.png` — 256x256 PNG mask for image-edit tests.
- `images/reference-square-128.jpg` — 128x128 JPEG reference image for
  image-to-video and multimodal request-shape tests.

### Audio

- `audio/tone-440hz-400ms.wav` — 0.4s mono 16 kHz WAV tone for audio
  protocol smoke tests.

### Video

- `videos/reference-160x90-1s.mp4` — 1s 160x90 MP4 test pattern for video
  input/proxy/ffprobe validation tests.

## Regeneration

Use the generator from the repository root:

```bash
python3 scripts/channel-benchmark/fixtures/generate_fixtures.py
```

The image and audio fixtures use Python standard libraries plus Pillow. The
video fixture requires `ffmpeg`; if `ffmpeg` is not installed, the script keeps
the existing video fixture and prints a warning.

Fixtures should stay tiny. Do not commit customer data, real people, secrets,
or provider-generated outputs here.

