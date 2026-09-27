# contract/assets

Static files for the showcases. The backend serves them at `GET /assets/<path>`. The demo operations return absolute
URLs for the images. The screens reference Lottie and Rive files by server-relative paths such as
`/assets/lottie/pulse.json`, and the renderers resolve those against the server base URL.

| File | What | Origin / license |
|---|---|---|
| `fonts/*` | DS fonts bundled by the SDKs | see the font vendors |
| `avatar.png` | 256×256 abstract avatar (blue gradient, person glyph), used by `demo.profile` | generated for Wishbone with Python PIL; project-owned |
| `offers/o1.jpg … o6.jpg` | 800×600 abstract gradient art, one per `demo.offers` item: two tickets, rings, sunrise, bokeh night, waves, confetti | generated for Wishbone with Python PIL; project-owned |
| `lottie/pulse.json` | three rings pulse around a breathing dot (accent blue), 150 frames at 60 fps, loops | hand-authored Lottie 5.7; project-owned |
| `lottie/sparkle.json` | a four-point star turns and breathes while two small stars twinkle (orange), 120 frames at 60 fps, loops | hand-authored Lottie 5.7; project-owned |
| `lottie/check.json` | a green circle draws on, then the checkmark, then a soft disc fades in; 120 frames at 60 fps; play once | hand-authored Lottie 5.7; project-owned |
| `rive/play_pause.riv` | play/pause button (Rive format 7.0, 3.3 KB) | see below |

## Rive: `play_pause.riv`

- **Source:** <https://github.com/rive-app/rive-ios/blob/main/Demo-App/RiveExampleSPM/Rive%20Files/play_pause.riv>
  (raw: `https://raw.githubusercontent.com/rive-app/rive-ios/main/Demo-App/RiveExampleSPM/Rive%20Files/play_pause.riv`).
- **License:** MIT, the license of the rive-ios repository (<https://github.com/rive-app/rive-ios/blob/main/LICENSE>),
  © Rive.
- **Contents:**
  - artboard `Artboard` (the default one, so `artboard` can stay empty);
  - state machine `State Machine 1`;
  - **boolean** input `Play`.
- **Where the names come from:** the repo's example `Demo-App/RiveExampleSPM/SplashScreen.swift`, which does
  `RiveViewModel(fileName: "play_pause", stateMachineName: "State Machine 1")` and `playButton.setInput("Play", value: isPlaying)`.
  I confirmed them in the binary: object type 53 (StateMachine) is named `State Machine 1`, and it is followed by type
  59 (StateMachineBool) named `Play`.
- **Used by:** `showcase.promo` node `liveButton`:
  `{"url": "/assets/rive/play_pause.riv", "stateMachine": "State Machine 1", "inputs": {"Play": live}}`.
