# contract/fonts

DS fonts bundled by the SDKs. **Not served by the backend** — this directory is outside `contract/assets/`, which is
the only tree exposed at `GET /assets/<path>` (by construction, not by a route-level check).

| File | Origin / license |
|---|---|
| `Manrope-*.ttf`, `Montserrat-*.ttf` | SIL OFL 1.1, full text with copyright lines in `OFL.txt` (copied next to the bundled fonts in both SDKs) |
| `GiorgioSansLCG-HeavyItalic.otf` | commercial (Commercial Type), see below |

## GiorgioSansLCG

`GiorgioSansLCG-HeavyItalic.otf` is a commercial font: "(C)2012 Christian Schwartz with Vincent Chan, under
exclusive license to Schwartzco Inc./Commercial Type. All rights reserved." (license URL in the file:
<http://commercialtype.com/license>). It is not covered by `OFL.txt`. **The owner must confirm that the Korpu font
license allows redistributing it inside the SDKs and apps** before either SDK ships outside the team.
