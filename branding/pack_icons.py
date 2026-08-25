"""Resize branding masters into Android mipmaps, TV banner, notification glyph, and Windows ICO."""
from pathlib import Path

from PIL import Image

ROOT = Path(__file__).resolve().parents[1]
SRC = Path(__file__).resolve().parent
ASSETS = Path(r"C:\Users\nextx\.cursor\projects\d-Masque\assets")

def load(name: str) -> Image.Image:
    for base in (ASSETS, SRC):
        p = base / name
        if p.exists():
            return Image.open(p).convert("RGBA")
    raise FileNotFoundError(name)

def save_resized(im: Image.Image, dest: Path, size: int) -> None:
    dest.parent.mkdir(parents=True, exist_ok=True)
    out = im.resize((size, size), Image.Resampling.LANCZOS)
    out.save(dest, "PNG")

def main() -> None:
    icon = load("masque-icon.png")
    glyph = load("masque-glyph.png")
    notify = load("masque-notify.png")
    banner = load("masque-tv-banner.png")

    (SRC / "masque-icon.png").parent.mkdir(parents=True, exist_ok=True)
    icon.save(SRC / "masque-icon.png", "PNG")
    glyph.save(SRC / "masque-glyph.png", "PNG")
    notify.save(SRC / "masque-notify.png", "PNG")
    banner.save(SRC / "masque-tv-banner.png", "PNG")

    mip = {
        "mdpi": 48,
        "hdpi": 72,
        "xhdpi": 96,
        "xxhdpi": 144,
        "xxxhdpi": 192,
    }
    res = ROOT / "android" / "app" / "src" / "main" / "res"
    for dens, px in mip.items():
        save_resized(icon, res / f"mipmap-{dens}" / "ic_launcher.png", px)
        save_resized(icon, res / f"mipmap-{dens}" / "ic_launcher_round.png", px)
        save_resized(glyph, res / f"mipmap-{dens}" / "ic_launcher_foreground.png", px)

    notify_dir = res / "drawable-hdpi"
    notify_dir.mkdir(parents=True, exist_ok=True)
    # Status-bar icons must be white + alpha.
    n = notify.resize((72, 72), Image.Resampling.LANCZOS)
    px = n.load()
    for y in range(n.height):
        for x in range(n.width):
            r, g, b, a = px[x, y]
            lum = (r + g + b) // 3
            px[x, y] = (255, 255, 255, min(a, lum if lum > 8 else a))
    n.save(notify_dir / "ic_stat_masque.png", "PNG")

    tv = ROOT / "android" / "app" / "src" / "tv" / "res" / "drawable"
    tv.mkdir(parents=True, exist_ok=True)
    banner.resize((320, 180), Image.Resampling.LANCZOS).save(tv / "tv_banner.png", "PNG")

    ico_sizes = [(16, 16), (24, 24), (32, 32), (48, 48), (64, 64), (128, 128), (256, 256)]
    ico_path = ROOT / "windows" / "installer" / "masque.ico"
    ico_path.parent.mkdir(parents=True, exist_ok=True)
    icon.save(ico_path, format="ICO", sizes=ico_sizes)

    gui_icon = ROOT / "windows" / "cmd" / "vpn-gui" / "icon.png"
    glyph.resize((256, 256), Image.Resampling.LANCZOS).save(gui_icon, "PNG")

    print("packed icons")

if __name__ == "__main__":
    main()
