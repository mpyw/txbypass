"""Draws social-preview.png for txbypass from the icon.

Usage: python3 social-preview.py <icon> <out>. It needs Pillow, and the SF
fonts that macOS ships in /System/Library/Fonts.
"""
import random
import sys

from PIL import Image, ImageDraw, ImageFilter, ImageFont

icon_path, out_path = sys.argv[1], sys.argv[2]
W, H = 1280, 640
BG = (30, 27, 58)

img = Image.new("RGBA", (W, H), BG + (255,))

# A faint pattern of braces and brackets, like source code, at about 5%.
pattern = Image.new("RGBA", (W, H), (0, 0, 0, 0))
pd = ImageDraw.Draw(pattern)
mono = ImageFont.truetype("/System/Library/Fonts/SFNSMono.ttf", 30)
rnd = random.Random(7)
glyphs = ["{ }", "< >", "( )", "[ ]", "/>", "=>", "{", "}", "<", ">"]
for y in range(-10, H, 64):
    for x in range(-20 + (y // 64 % 2) * 48, W, 96):
        pd.text((x + rnd.randint(-6, 6), y + rnd.randint(-6, 6)), rnd.choice(glyphs),
                font=mono, fill=(255, 255, 255, 13))
img.alpha_composite(pattern)

# The icon on the right, about 480 pixels tall, with a soft glow behind it.
size = 480
icon = Image.open(icon_path).convert("RGBA").resize((size, size), Image.LANCZOS)
ix, iy = W - 90 - size, (H - size) // 2
glow = Image.new("RGBA", (W, H), (0, 0, 0, 0))
ImageDraw.Draw(glow).ellipse((ix - 30, iy - 30, ix + size + 30, iy + size + 30), fill=(150, 120, 255, 90))
img.alpha_composite(glow.filter(ImageFilter.GaussianBlur(45)))
img.alpha_composite(icon, (ix, iy))

# The name and the tagline on the left, left-aligned.
def rounded(px, weight):
    f = ImageFont.truetype("/System/Library/Fonts/SFNSRounded.ttf", px)
    f.set_variation_by_name(weight)
    return f

d = ImageDraw.Draw(img)
title, tag = rounded(124, "Bold"), rounded(40, "Semibold")
tx = 110
tb = d.textbbox((0, 0), "txbypass", font=title)
gb = d.textbbox((0, 0), "Inside the transaction, use tx", font=tag)
gap = 26
block = (tb[3] - tb[1]) + gap + (gb[3] - gb[1])
top = (H - block) // 2
d.text((tx - tb[0], top - tb[1]), "txbypass", font=title, fill=(255, 255, 255))
d.text((tx - gb[0], top + (tb[3] - tb[1]) + gap - gb[1]), "Inside the transaction, use tx",
       font=tag, fill=(201, 190, 255))

img.convert("RGB").quantize(256, method=Image.Quantize.MEDIANCUT, dither=Image.Dither.NONE).save(out_path, optimize=True)
