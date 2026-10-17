# Assets

| File | Size | Used by |
| --- | --- | --- |
| `icon.png` | 512×512, transparent | The top of the README |
| `social-preview.png` | 1280×640 | The repository's social preview, set in its settings |

The icon was drawn with ChatGPT from the prompt below. `icon.py` cleaned its edge and scaled it. `social-preview.py` drew the preview from the cleaned icon. Both scripts need Pillow, and `icon.py` needs NumPy too. `social-preview.py` reads the SF fonts that macOS ships.

```bash
python3 icon.py drawn.png clean.png icon.png
python3 social-preview.py clean.png social-preview.png
```

The images are reduced to 256 colors, as the scripts save them.

## Logo

```text
Create an original app icon for a developer tool called "txbypass", a Go
linter that catches database writes that sneak around a transaction. The
mascot is the Go gopher.

Match this series of icons exactly in style: a light cream circle as the
background, a large purple hexagon behind the character, a cyan-blue Go gopher
with big round white eyes, small tan hands and feet, and a few small
four-pointed sparkles in cyan and white around it.

Subject: the gopher stands in a protective pose, hugging a small database
cylinder (three stacked discs, light mint) with one arm. A dashed rounded
rectangle in white surrounds the gopher and the cylinder, like a fence. With
the other hand held out flat, palm forward, the gopher stops a small curved
arrow in coral red that tries to sneak around the outside of the dashed
rectangle. The arrow's head is just touching the gopher's palm and stops
there. The gopher looks determined but friendly.

Style: flat vector illustration, bold clean shapes, thick smooth dark
outlines, soft highlights only. It must stay readable at 32×32 pixels, so keep
the cylinder, the dashed rectangle and the arrow large and simple.

Composition: centered, square 1024×1024. The cream circle fills the canvas,
with a transparent background outside it. No text, no letters, no border,
no drop shadow.

Make it an original drawing of the gopher. Do not copy any existing gopher
artwork.
```

The Go gopher was designed by Renée French, under [CC BY 4.0](https://creativecommons.org/licenses/by/4.0/).
