import math, os, random, subprocess, sys
from PIL import Image, ImageDraw, ImageFont, ImageFilter

W, H, FPS, DUR = 1080, 1920, 30, 18
FRAMES = W, H
OUT = os.path.join(os.path.dirname(__file__), "frames")
os.makedirs(OUT, exist_ok=True)

BOLD = "/System/Library/Fonts/Supplemental/Arial Bold.ttf"
UNI = "/System/Library/Fonts/Supplemental/Arial Unicode.ttf"
MONO = "/System/Library/Fonts/Menlo.ttc"
logo = ImageFont.truetype(BOLD, 150)
h1 = ImageFont.truetype(BOLD, 54)
h2 = ImageFont.truetype(UNI, 34)
mono = ImageFont.truetype(MONO, 27)
small = ImageFont.truetype(MONO, 20)

GREEN = (0, 255, 102)
DIM = (0, 125, 55)
FAINT = (0, 55, 28)
BLACK = (3, 8, 5)

random.seed(14)
SYMS = list(".,:;!ilI|+-=_~#%&$@λπΣΨΩ±×÷")
HEADS = ["NET", "GTW", "RUN", "MEM", "C\u00b0", "CPU"]

def cell(zero):
    if zero:
        return " " + random.choice("·.,'`˙∙") + " "
    return "".join(random.sample(SYMS, 3))

def make_row():
    return "   ".join(cell(random.random() < 0.30) for _ in HEADS)

ROWS = [make_row() for _ in range(24)]

def rain_frame(t):
    img = Image.new("RGB", (W, H), BLACK)
    d = ImageDraw.Draw(img)
    # vertical "digital rain" columns
    for x in range(-20, W + 40, 56):
        speed = 90 + (x * 37 % 160)
        phase = (x * 91 + 17) % 900
        y0 = int((t * speed + phase) % (H + 300)) - 300
        for k in range(18):
            y = y0 + k * 34
            if y < -40 or y > H:
                continue
            ch = random.choice(SYMS)
            col = GREEN if k == 0 else (0, max(35, 160 - k * 8), max(20, 80 - k * 3))
            d.text((x, y), ch, font=mono, fill=col)
    return img

def terminal(t):
    img = rain_frame(t)
    d = ImageDraw.Draw(img)
    # panel
    x0, y0, x1, y1 = 70, 470, 1010, 1390
    d.rectangle([x0, y0, x1, y1], fill=(3, 9, 6), outline=(0, 90, 45), width=2)
    d.rectangle([x0, y0, x1, y0 + 52], fill=(9, 20, 13))
    for i, c in enumerate([(255, 95, 87), (254, 188, 46), (40, 200, 64)]):
        d.ellipse([x0 + 22 + i * 28, y0 + 18, x0 + 38 + i * 28, y0 + 34], fill=c)
    d.text((x0 + 130, y0 + 13), "lmon --matrix --rain", font=small, fill=(120, 170, 135))
    # header
    hx, hy = x0 + 44, y0 + 91
    cw = 145
    for i, h in enumerate(HEADS):
        xx = hx + i * cw
        d.rectangle([xx, hy, xx + 128, hy + 34], fill=(0, 68, 34))
        d.text((xx + 10, hy + 3), h, font=h2, fill=(140, 255, 180))
    # rows falling from under header
    step = int(max(0, (t - 2.1)) / 0.42)
    for i, row in enumerate(ROWS[:step]):
        y = hy + 50 + i * 48
        if y > y1 - 50:
            break
        d.text((hx + 3, y), row, font=mono, fill=GREEN)
    # command below
    d.text((80, 1430), "lmon --matrix --rain", font=mono, fill=(150, 255, 180))
    d.text((80, 1470), "без цифр · случайные безопасные символы · ноль — точка", font=small, fill=(0, 170, 85))
    return img

def title(t):
    img = rain_frame(t)
    d = ImageDraw.Draw(img)
    # dim rain already
    d.text((80, 620), "lmon", font=logo, fill=GREEN)
    d.text((88, 810), "весь компьютер", font=h1, fill=(190, 255, 210))
    d.text((88, 880), "в терминале", font=h1, fill=(190, 255, 210))
    d.text((92, 1000), "матричный режим", font=h2, fill=(0, 190, 95))
    d.text((92, 1070), "lmon --matrix --rain", font=mono, fill=(0, 255, 120))
    return img

def end(t):
    img = rain_frame(t)
    d = ImageDraw.Draw(img)
    d.text((80, 540), "lmon", font=logo, fill=GREEN)
    d.text((90, 730), "весь компьютер", font=h2, fill=(180, 255, 200))
    d.text((90, 790), "в одной команде", font=h2, fill=(180, 255, 200))
    d.text((90, 930), "lmon --matrix --all --rain", font=mono, fill=(0, 255, 120))
    d.text((90, 1000), "github.com/martiyanov/lmon", font=mono, fill=(0, 180, 90))
    d.text((90, 1070), "MIT · macOS · Linux · Windows", font=small, fill=(0, 140, 70))
    return img

def fade(img, k):
    if k <= 0:
        return img
    if k >= 1:
        return Image.new("RGB", (W, H), (0, 0, 0))
    black = Image.new("RGB", (W, H), (0, 0, 0))
    return Image.blend(img, black, k)

for n in range(FPS * DUR):
    t = n / FPS
    if t < 2.2:
        img = title(t)
    elif t < 15.2:
        img = terminal(t)
    else:
        img = end(t)
    # fade in/out at edges
    k = 1.0 - t / 0.5
    if k > 0:
        img = fade(img, min(1, k))
    k = (t - (DUR - 0.7)) / 0.7
    if k > 0:
        img = fade(img, min(1, k))
    img.save(os.path.join(OUT, f"f{n:04d}.png"))
print("frames", FPS * DUR)
