import json, os, subprocess, time
SESSION = "lmon-reel"
SOCKET = "lmonreel"
SIZE = ("90", "28")
MODES = [
    ("digits", ["lmon", "--interval", "0.45"]),
    ("bars", ["lmon", "--bars", "--interval", "0.45"]),
    ("compact", ["lmon", "--compact", "--interval", "0.45"]),
    ("matrix", ["lmon", "--matrix", "--rain", "--interval", "0.45"]),
    ("horizontal", ["lmon", "--horizontal", "--interval", "0.45"]),
]
def sh(args):
    return subprocess.run(args, check=False, capture_output=True, text=True)
def kill():
    sh(["tmux", "-L", SOCKET, "kill-session", "-t", SESSION])
def capture(mode, cmd, seconds=3.2):
    kill()
    shell = "env -u NO_COLOR TERM=xterm-256color COLORTERM=truecolor " + " ".join(cmd)
    r = sh(["tmux", "-L", SOCKET, "new-session", "-d", "-s", SESSION, "-x", SIZE[0], "-y", SIZE[1], shell])
    if r.returncode != 0:
        print("tmux start failed", mode, r.stderr)
        return []
    time.sleep(1.4)
    frames = []
    end = time.time() + seconds
    while time.time() < end:
        r = sh(["tmux", "-L", SOCKET, "capture-pane", "-p", "-e", "-t", SESSION, "-S", "-100"])
        lines = r.stdout.splitlines()[-28:]
        frames.append({"t": len(frames) * 0.12, "lines": lines})
        time.sleep(0.12)
    kill()
    return frames
all_frames = {}
for name, cmd in MODES:
    print("capture", name)
    all_frames[name] = capture(name, cmd)
with open(os.path.join(os.path.dirname(__file__), "captures.json"), "w") as f:
    json.dump(all_frames, f, ensure_ascii=False)
print({k: len(v) for k, v in all_frames.items()})
