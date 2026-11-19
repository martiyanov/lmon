import json, os, sys
from PIL import Image, ImageDraw, ImageFont
ROOT = os.path.dirname(__file__)
sys.path.insert(0, ROOT)
import render

W, H, FPS = 1080, 1920, 30
CAP = json.load(open(os.path.join(ROOT, 'captures.json')))
BOLD = '/System/Library/Fonts/Supplemental/Arial Bold.ttf'
UNI = '/System/Library/Fonts/Supplemental/Arial Unicode.ttf'
MONO = '/System/Library/Fonts/Menlo.ttc'
logo = ImageFont.truetype(BOLD, 50)
h1 = ImageFont.truetype(BOLD, 62)
h2 = ImageFont.truetype(UNI, 34)
mono = ImageFont.truetype(MONO, 26)
chip = ImageFont.truetype(MONO, 25)

GREEN = (0, 255, 102)
MUTED = (0, 130, 65)
INK = (200, 255, 215)
BLACK = (3, 7, 5)

SCENES = [
    ('digits', 6.5, 'lmon', 'Сервер тормозит? Сначала — общая картина.', 'Server feels slow? Start with the whole picture.'),
    ('bars', 6.5, 'lmon --bars', 'Цвет вместо цифр: проблема видна боковым зрением.', 'Color instead of numbers: spot trouble at a glance.'),
    ('compact', 6.0, 'lmon --compact', 'Узкое окно или tmux — минимум места.', 'Narrow window or tmux: minimum space.'),
    ('matrix', 8.0, 'lmon --matrix --rain', 'Тот же мониторинг, только красивее.', 'Same monitoring, nicer to look at.'),
    ('horizontal', 7.5, 'lmon --horizontal --labels full', 'История вправо: видно, когда началось.', 'History to the right: see when it started.'),
    ('once', 4.0, 'lmon --once --cols NGCMT', 'В статусбар или скрипт — одна строка.', 'Status bar or script: one line.'),
]
KEYS = ['lmon', '--bars', '--compact', '--matrix', '--rain', '--horizontal', '--labels full', '--cols', '--once', '--all', '--disk-io', '--gpu', '--fans']

def bg():
    img = Image.new('RGB', (W, H), BLACK)
    d = ImageDraw.Draw(img)
    for x in range(0, W, 54):
        for y in range(0, H, 54):
            d.ellipse([x, y, x+2, y+2], fill=(0, 22, 11))
    return img, d

def draw_keys(d, mode, lang):
    active = {'digits':'lmon', 'bars':'--bars', 'compact':'--compact',
              'matrix':'--matrix', 'horizontal':'--horizontal', 'once':'--once'}[mode]
    x, y = 42, 1400
    for k in KEYS:
        f = chip
        tw = d.textlength(k, font=f)
        w = tw + 22
        if x + w > W - 30:
            x = 42; y += 60
        on = (k == active)
        d.rounded_rectangle([x, y, x+w, y+44], radius=21,
                            fill=(0, 70, 35) if on else None,
                            outline=(0, 190, 95) if on else (0, 70, 38), width=1)
        d.text((x+14, y+9), k, font=f, fill=(150, 255, 180) if on else (0, 130, 65))
        x += w + 10

def terminal(mode, local, lang):
    img, d = bg()
    d.text((42, 40), 'lmon', font=logo, fill=GREEN)
    d.text((42, 100), 'терминальный мониторинг' if lang == 'ru' else 'terminal monitoring', font=mono, fill=(0, 140, 75))
    scene = next(x for x in SCENES if x[0] == mode)
    caption = scene[3] if lang == 'ru' else scene[4]
    # wrap caption to two lines at most
    words = caption.split(); lines=[]; cur=''
    for word in words:
        test=(cur+' '+word).strip()
        if d.textlength(test, font=h1) > 990 and cur:
            lines.append(cur); cur=word
        else: cur=test
    if cur: lines.append(cur)
    for i,line in enumerate(lines[:2]):
        d.text((42, 165+i*74), line, font=h1, fill=INK)
    d.text((42, 390), '$ ' + scene[2], font=mono, fill=(0, 230, 110))
    # terminal panel
    x0,y0,w,h=30,470,1020,660
    d.rectangle([x0,y0,x0+w,y0+h], fill=(3,8,6), outline=(0,95,48), width=2)
    d.rectangle([x0,y0,x0+w,y0+52], fill=(9,18,12))
    for i,c in enumerate([(255,95,87),(254,188,46),(40,200,64)]):
        d.ellipse([x0+20+i*26,y0+17,x0+34+i*26,y0+31], fill=c)
    d.text((x0+108,y0+13), scene[2], font=mono, fill=(120,170,135))
    cap = CAP.get(mode, [])
    idx = min(len(cap)-1, int(local*2.0/0.12)) if cap else 0
    lines = cap[idx]['lines'] if cap else []
    cw, rh = 18.0, 36
    # real terminal content, 90x28
    for row,line in enumerate(lines[-14:]):
        y = y0+64+row*rh
        for col,(ch,fg,bgcol,bold) in enumerate(render.parse_line(line)[:55]):
            x = x0+18+col*cw
            if bgcol is not None:
                d.rectangle([x,y-1,x+cw,y+rh-1], fill=bgcol)
            if ch != ' ':
                d.text((x,y), ch, font=mono, fill=fg)
    draw_keys(d, mode, lang)
    return img

def run(lang):
    out=os.path.join(ROOT,'benefit-'+lang); os.makedirs(out, exist_ok=True)
    total=sum(x[1] for x in SCENES); n=int(total*FPS); acc=0
    for i in range(n):
        t=i/FPS
        a=0; mode=SCENES[-1][0]; local=0
        for name,dur,*_ in SCENES:
            if t < a+dur:
                mode=name; local=t-a; break
            a+=dur
        img=terminal(mode, local, lang)
        # short fade in and out only
        if t < .3: img = Image.blend(img, Image.new('RGB',(W,H),(0,0,0)), 1-t/.3)
        if t > total-.4: img = Image.blend(img, Image.new('RGB',(W,H),(0,0,0)), (t-(total-.4))/.4)
        img.save(os.path.join(out,f'f{i:04d}.png'))
    print(lang,n,total)
if __name__=='__main__': run(sys.argv[1] if len(sys.argv)>1 else 'ru')
