# QCC brand assets

<img alt="QCC wordmark" src="./readme-banner.svg" width="420">

Marks for the project README, social cards, and favicons. Every mark is
pure SVG with no font files to ship.

| File | Use |
|---|---|
| `readme-banner.svg` | the banner at the top of the project README |
| `qcc-mark-light.svg` | wordmark on a light background |
| `qcc-mark-dark.svg` | wordmark on a dark background |
| `qcc-mark-blue.svg` | hero images, social cards, OG images |
| `qcc-icon.svg` | square app icon, 64 by 64, rounded |
| `favicon.svg` | browser tab favicon, 32 by 32 |

The remaining files in this directory are alternate banner crops and
light or dark variants of the same marks.

## Colors

```
ink           #161616
paper-dim     #f4f4f4
phase / 60    #0f62fe   primary
phase / 30    #a6c8ff   dot on dark
night         #0a0a16
```

## Using a mark

GitHub picks the variant from the reader's color scheme:

```html
<picture>
  <source media="(prefers-color-scheme: dark)" srcset="qcc-mark-dark.svg">
  <img alt="QCC" src="qcc-mark-light.svg" height="48">
</picture>
```

Alt text is required. It should say what the mark is, not that it is an
image.

## Fonts

The SVGs use a font stack that starts at `IBM Plex Sans` and falls back
to the system sans-serif, so they render everywhere but kern exactly only
where IBM Plex Sans is installed. Converting the glyphs to outlines in a
vector editor before re-exporting makes the kerning fixed.
