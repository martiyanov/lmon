import json, os, re, sys
from PIL import Image, ImageDraw, ImageFont

W, H, FPS = 1080, 1920, 30
ROOT = os.path.dirname(__file__)
CAP = json.load(open(os.path.join(ROOT, 'captures.json')))
BOLD = '/System/Library/Fonts/Supplemental/Arial Bold.ttf'
UNI = '/System/Library/Fonts/Supplemental/Arial Unicode.ttf'
MONO = '/System/Library/Fonts/Menlo.ttc'
logo = ImageFont.truetype(BOLD, 128)
h1 = ImageFont.truetype(BOLD, 46)
h2 = ImageFont.truetype(UNI, 28)
mono = ImageFont.truetype(MONO, 15)
small = ImageFont.truetype(MONO, 17)

GREEN = (0, 255, 102)
DIM = (0, 150, 75)
BLACK = (3, 7, 5)

def xterm(n):
    n = int(n)
    base = [
        (0,0,0),(205,49,49),(13,188,121),(229,229,16),(36,114,200),(188,63,188),(17,168,205),(229,229,229),
        (102,102,102),(241,76,76),(35,209,139),(245,245,67),(59,142,234),(214,112,214),(41,184,219),(255,255,255)]
    if n < 16: return base[n]
    if n < 232:
        n -= 16; r=n//36; g=(n%36)//6; b=n%6
        v=[0,95,135,175,215,255]
        return (v[r],v[g],v[b])
    v=8+(n-232)*10
    return (v,v,v)

def parse_line(line):
    # returns list of (char, fg, bg, bold)
    fg=(214,222,230); bg=None; bold=False
    out=[]; pos=0
    for part in re.split(r'(\x1b\[[0-9;]*m)', line):
        if not part: continue
        if part.startswith('\x1b['):
            nums=part[2:-1].split(';')
            i=0
            while i < len(nums):
                try: n=int(nums[i] or 0)
                except: n=0
                if n==0: fg=(214,222,230); bg=None; bold=False
                elif n==1: bold=True
                elif n==22: bold=False
                elif n==39: fg=(214,222,230)
                elif n==49: bg=None
                elif 30<=n<=37: fg=xterm(n-30)
                elif 90<=n<=97: fg=xterm(n-90+8)
                elif 40<=n<=47: bg=xterm(n-40)
                elif 100<=n<=107: bg=xterm(n-100+8)
                elif n in (38,48) and i+2 < len(nums) and nums[i+1]=='5':
                    c=xterm(nums[i+2]); 
                    if n==38: fg=c
                    else: bg=c
                    i+=2
                i+=1
            continue
        for ch in part:
            out.append((ch,fg,bg,bold))
    return out

def rain_bg(t):
    img=Image.new('RGB',(W,H),BLACK); d=ImageDraw.Draw(img)
    chars='.,:;!ilI|+-=_~#%&$@λπΣΨΩ±×÷'
    for x in range(-30,W+40,54):
        speed=80+(x*41)%140; phase=(x*73)%1300
        y0=int((t*speed+phase)%(H+260))-260
        for k in range(16):
            y=y0+k*32
            if -30<=y<=H:
                ch=chars[(x//7+k)%len(chars)]
                col=GREEN if k==0 else (0,max(30,150-k*8),max(15,70-k*4))
                d.text((x,y),ch,font=mono,fill=col)
    return img

def terminal_window(mode, t, lang):
    img=rain_bg(t); d=ImageDraw.Draw(img)
    x0,y0,w,h=55,650,970,500
    d.rectangle([x0,y0,x0+w,y0+h],fill=(3,8,6),outline=(0,85,42),width=2)
    d.rectangle([x0,y0,x0+w,y0+46],fill=(9,18,12))
    for i,c in enumerate([(255,95,87),(254,188,46),(40,200,64)]):
        d.ellipse([x0+20+i*26,y0+15,x0+34+i*26,y0+29],fill=c)
    cmd={'digits':'lmon','bars':'lmon --bars','compact':'lmon --compact',
         'matrix':'lmon --matrix --rain','horizontal':'lmon --horizontal'}[mode]
    d.text((x0+110,y0+11),cmd,font=small,fill=(120,170,135))
    cap=CAP[mode]
    idx=min(len(cap)-1, int(t/0.12)) if cap else 0
    lines=cap[idx]['lines'] if cap else []
    while lines and not lines[0].strip(): lines=lines[1:]
    while lines and not lines[-1].strip(): lines=lines[:-1]
    lines=lines[-16:]
    cw,ch=10.5,19
    start=y0+h-14-len(lines)*ch
    for row,line in enumerate(lines):
        y=start+row*ch
        for col,(chv,fg,bg,bold) in enumerate(parse_line(line)[:90]):
            x=x0+10+col*cw
            if bg is not None:
                d.rectangle([x,y-1,x+cw,y+ch-1],fill=bg)
            if chv!=' ':
                d.text((x,y),chv,font=mono,fill=fg)
    # caption ribbon
    d.rectangle([x0,y0+h+18,x0+w,y0+h+92],fill=(5,22,12))
    cap_ru={'digits':'Цифры и цвет: состояние по порогам',
            'bars':'Блоки без цифр: цвет вместо чисел',
            'compact':'Одна буква и один блок на параметр',
            'matrix':'Матрица: без цифр, символы вместо значений',
            'horizontal':'Горизонтальная лента: параметр — строка'}[mode]
    cap_en={'digits':'Digits and color: state against thresholds',
            'bars':'Color blocks instead of numbers',
            'compact':'One letter and one block per parameter',
            'matrix':'Matrix: no digits, glyphs instead of values',
            'horizontal':'Horizontal timeline: one parameter per row'}[mode]
    d.text((x0+22,y0+h+37),(cap_ru if lang=='ru' else cap_en),font=h2,fill=(180,255,200))
    # top label
    d.text((60,90),'lmon',font=logo,fill=GREEN)
    d.text((64,245),('пульс компьютера' if lang=='ru' else 'the pulse of your computer'),font=h1,fill=(170,255,195))
    d.text((64,305),('в одной строке' if lang=='ru' else 'in one line'),font=h1,fill=(170,255,195))
    return img

def intro(t, lang):
    img=rain_bg(t); d=ImageDraw.Draw(img)
    d.text((70,560),'lmon',font=logo,fill=GREEN)
    if lang=='ru':
        lines=['пульс компьютера','в одной строке','реальный TUI · без графики']
    else:
        lines=['the pulse of your computer','in one line','real TUI · no graphs']
    for i,s in enumerate(lines):
        d.text((76,735+i*72),s,font=h1 if i<2 else h2,fill=(180,255,205))
    d.text((80,1010),'lmon --matrix --rain',font=small,fill=(0,210,100))
    return img

def end(t, lang):
    img=rain_bg(t); d=ImageDraw.Draw(img)
    d.text((70,480),'lmon',font=logo,fill=GREEN)
    if lang=='ru':
        lines=['пульс компьютера','в одной строке','github.com/martiyanov/lmon']
    else:
        lines=['the pulse of your computer','in one line','github.com/martiyanov/lmon']
    for i,s in enumerate(lines):
        d.text((76,660+i*72),s,font=h2 if i<2 else small,fill=(180,255,205) if i<2 else (0,190,100))
    d.text((76,930),'lmon --matrix --rain',font=small,fill=(0,255,120))
    d.text((76,980),('реальный вывод терминала' if lang=='ru' else 'captured from a real terminal'),font=small,fill=(0,150,75))
    return img

def fade(img,k):
    if k<=0: return img
    black=Image.new('RGB',(W,H),(0,0,0))
    return Image.blend(img,black,min(1,k))

SEQ=[('intro',2.0),('digits',3.0),('bars',3.0),('compact',3.0),('matrix',3.4),('horizontal',3.0),('end',3.0)]

def run(lang):
    out=os.path.join(ROOT,'frames-'+lang); os.makedirs(out,exist_ok=True)
    total=sum(d for _,d in SEQ); nframes=int(total*FPS)
    cur=0.0
    for i in range(nframes):
        t=i/FPS
        # find scene
        acc=0.0; scene='end'; local=0.0
        for name,dur in SEQ:
            if t < acc+dur:
                scene=name; local=t-acc; break
            acc+=dur
        if scene=='intro': img=intro(t,lang)
        elif scene=='end': img=end(t,lang)
        else: img=terminal_window(scene,local,lang)
        k=1-(t/0.4)
        if k>0: img=fade(img,k)
        k=(t-(total-0.5))/0.5
        if k>0: img=fade(img,k)
        img.save(os.path.join(out,f'f{i:04d}.png'))
    print(lang,nframes,total)

if __name__=='__main__':
    run(sys.argv[1] if len(sys.argv)>1 else 'ru')
