import json, os, sys
from PIL import Image, ImageDraw, ImageFont
sys.path.insert(0, os.path.dirname(__file__))
import render

ROOT = os.path.dirname(__file__)
CAP = json.load(open(os.path.join(ROOT, 'captures.json')))
MONO = ImageFont.truetype('/System/Library/Fonts/Menlo.ttc', 17)
TITLE = ImageFont.truetype('/System/Library/Fonts/Menlo.ttc', 13)

def make(mode, out):
    lines = CAP[mode][-1]['lines']
    while lines and not lines[0].strip(): lines = lines[1:]
    while lines and not lines[-1].strip(): lines = lines[:-1]
    lines = lines[-14:]
    W = 980
    row_h = 21
    H = 52 + len(lines) * row_h + 18
    img = Image.new('RGB', (W, H), (4, 9, 7))
    d = ImageDraw.Draw(img)
    d.rectangle([0, 0, W, 42], fill=(10, 20, 14))
    for i, c in enumerate([(255,95,87),(254,188,46),(40,200,64)]):
        d.ellipse([16+i*24, 14, 30+i*24, 28], fill=c)
    d.text((105, 13), 'lmon --' + ('matrix --rain' if mode == 'matrix' else mode), font=TITLE, fill=(120,170,135))
    cw = 10.5
    for row, line in enumerate(lines):
        y = 55 + row * row_h
        for col, (ch, fg, bg, bold) in enumerate(render.parse_line(line)[:90]):
            x = 14 + col * cw
            if bg is not None:
                d.rectangle([x, y-1, x+cw, y+row_h-1], fill=bg)
            if ch != ' ':
                d.text((x, y), ch, font=MONO, fill=fg)
    # scale to 640 wide
    img = img.resize((640, int(H * 640 / W)), Image.LANCZOS)
    img.save(out)

make('bars', os.path.join(ROOT, '../../docs/img/real-terminal.png'))
print('saved')
