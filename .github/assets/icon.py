"""Cleans the drawn icon and scales it to icon.png.

Usage: python3 icon.py <drawn> <clean> <icon>. It needs Pillow and NumPy.

The drawing came with ragged white specks around its cream circle. A clean
circle replaces that edge. The arrow's tail reaches past the circle, so its
red and dark strokes are kept outside it. <clean> is the full-size result,
which social-preview.py reads.
"""
import sys

import numpy as np
from PIL import Image, ImageFilter

drawn, clean_path, icon_path = sys.argv[1:4]
a = np.asarray(Image.open(drawn).convert("RGBA")).astype(float)
r, g, b, al = a[..., 0], a[..., 1], a[..., 2], a[..., 3]
h, w = al.shape
yy, xx = np.mgrid[0:h, 0:w]

# Measured on the 1254×1254 drawing: the circle's center and its radius
# inside the specks.
cx, cy, radius = 627, 627, 580
d = np.hypot(xx - cx, yy - cy)
circle = np.clip(radius + 0.75 - d, 0, 1.5) / 1.5

red = (r > 180) & (g < 140) & (b < 130)
dark = (r < 90) & (g < 90) & (b < 110)
stroke = (al > 120) & (red | dark) & (d < 600)
stroke = Image.fromarray((stroke * 255).astype(np.uint8))
stroke = np.asarray(stroke.filter(ImageFilter.MaxFilter(3)).filter(ImageFilter.GaussianBlur(0.6))) / 255.0

out = a.copy()
out[..., 3] = np.where(circle >= 1, 255, al * np.maximum(circle, stroke))
img = Image.fromarray(out.clip(0, 255).astype(np.uint8))

# Center what is left on a square canvas.
box = img.getbbox()
side = max(box[2] - box[0], box[3] - box[1])
clean = Image.new("RGBA", (side, side), (0, 0, 0, 0))
clean.paste(img.crop(box), ((side - (box[2] - box[0])) // 2, (side - (box[3] - box[1])) // 2))
clean.save(clean_path)

icon = clean.resize((512, 512), Image.LANCZOS)
icon.quantize(256, method=Image.Quantize.FASTOCTREE, dither=Image.Dither.NONE).save(icon_path, optimize=True)
