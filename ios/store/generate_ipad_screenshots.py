#!/usr/bin/env python3
"""Generate App Store screenshots of the MASQUE iOS UI (no alpha)."""

from __future__ import annotations

from pathlib import Path

from PIL import Image, ImageDraw, ImageFont

BG = (242, 242, 247)
WHITE = (255, 255, 255)
LABEL = (0, 0, 0)
SECONDARY = (60, 60, 67)
BLUE = (0, 122, 255)
GRAY_FILL = (229, 229, 234)
FIELD_BG = (255, 255, 255)
FIELD_BORDER = (209, 209, 214)
HOME = (0, 0, 0)
ISLAND = (0, 0, 0)

OUT = Path(__file__).resolve().parent


def font(pt: int, scale: int, bold: bool = False) -> ImageFont.FreeTypeFont:
    size = int(pt * scale)
    names = (
        ("seguisb.ttf", "segoeuib.ttf", "arialbd.ttf")
        if bold
        else ("segoeui.ttf", "arial.ttf")
    )
    windir = Path(r"C:\Windows\Fonts")
    for name in names:
        path = windir / name
        if path.exists():
            return ImageFont.truetype(str(path), size)
    return ImageFont.load_default()


def rounded(draw, xy, radius, fill, outline=None, width=1):
    draw.rounded_rectangle(xy, radius=radius, fill=fill, outline=outline, width=width)


def button(draw, xy, text, fnt, prominent: bool, enabled: bool = True):
    x0, y0, x1, y1 = xy
    if prominent:
        rounded(draw, xy, 14 * (y1 - y0) // 50, BLUE if enabled else (174, 174, 178))
        color = WHITE
    else:
        rounded(draw, xy, 14 * (y1 - y0) // 50, GRAY_FILL)
        color = BLUE if enabled else (174, 174, 178)
    bbox = draw.textbbox((0, 0), text, font=fnt)
    tw, th = bbox[2] - bbox[0], bbox[3] - bbox[1]
    draw.text(
        ((x0 + x1 - tw) / 2, (y0 + y1 - th) / 2 - 2),
        text,
        font=fnt,
        fill=color,
    )


def render(
    *,
    size: tuple[int, int],
    scale: int,
    iphone: bool,
    status: str,
    ping: str,
    version: str,
    connected: bool,
    filename: str,
) -> None:
    w, h = size
    img = Image.new("RGB", (w, h), BG)
    draw = ImageDraw.Draw(img)
    s = scale

    f_status = font(17, s, bold=True)
    f_body = font(17, s)
    f_caption = font(13, s)
    f_title = font(34, s, bold=True)
    f_btn = font(17, s)

    if iphone:
        draw.text((24 * s, 18 * s), "9:41", font=f_status, fill=LABEL)
        island_w, island_h = 126 * s, 37 * s
        ix0 = (w - island_w) // 2
        iy0 = 11 * s
        rounded(draw, (ix0, iy0, ix0 + island_w, iy0 + island_h), island_h // 2, ISLAND)
        right = "5G  Wi-Fi  100%"
        rb = draw.textbbox((0, 0), right, font=f_status)
        draw.text((w - 22 * s - (rb[2] - rb[0]), 18 * s), right, font=f_status, fill=LABEL)
        title_y = 62 * s
        x = 20 * s
        y = 118 * s
        btn_w = min(220 * s, w - 40 * s)
    else:
        draw.text((48 * s, 14 * s), "9:41", font=f_status, fill=LABEL)
        right = "Wi-Fi   100%"
        rb = draw.textbbox((0, 0), right, font=f_status)
        draw.text((w - 48 * s - (rb[2] - rb[0]), 14 * s), right, font=f_status, fill=LABEL)
        title_y = 56 * s
        x = 20 * s
        y = 112 * s
        btn_w = 220 * s

    draw.text((x, title_y), "MASQUE", font=f_title, fill=LABEL)

    gap = 16 * s
    draw.text((x, y), status, font=f_body, fill=LABEL)
    y += 28 * s
    draw.text((x, y), ping, font=f_body, fill=SECONDARY)
    y += 26 * s
    draw.text((x, y), version, font=f_caption, fill=SECONDARY)
    y += 36 * s

    draw.text((x, y + 8 * s), "MTU", font=f_body, fill=LABEL)
    field = (x + 56 * s, y, x + 156 * s, y + 36 * s)
    rounded(draw, field, 8 * s, FIELD_BG, FIELD_BORDER, 2)
    draw.text((field[0] + 10 * s, field[1] + 7 * s), "1400", font=f_body, fill=LABEL)
    y += 56 * s

    btn_h = 50 * s
    button(draw, (x, y, x + btn_w, y + btn_h), "Import Profile", f_btn, False, enabled=not connected)
    y += btn_h + gap
    connect = "Disconnect" if connected else "Connect"
    button(draw, (x, y, x + btn_w, y + btn_h), connect, f_btn, True)

    bar_w, bar_h = (134 * s if not iphone else 139 * s), 5 * s
    bx0 = (w - bar_w) // 2
    by0 = h - 10 * s - bar_h
    rounded(draw, (bx0, by0, bx0 + bar_w, by0 + bar_h), bar_h // 2, HOME)

    out = OUT / filename
    img.save(out, "PNG")
    print(f"wrote {out} ({img.size[0]}x{img.size[1]} RGB)")


def shots(size, scale, iphone, prefix):
    common = dict(size=size, scale=scale, iphone=iphone, version="v1.7.0 (20)")
    render(
        **common,
        status="Status: profile not configured",
        ping="Ping: —",
        connected=False,
        filename=f"{prefix}-01-import.png",
    )
    render(
        **common,
        status="Status: VPN active",
        ping="Ping: 24 ms",
        connected=True,
        filename=f"{prefix}-02-connected.png",
    )


def main() -> None:
    shots((2064, 2752), 2, False, "ipad-13")
    # App Store Connect 6.5" iPhone class (required if 6.9" is not accepted).
    shots((1284, 2778), 3, True, "iphone-65")


if __name__ == "__main__":
    main()
