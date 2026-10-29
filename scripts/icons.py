#!/usr/bin/env python3
"""Draws the phoneset icon and writes every size the project needs.

One design drives the phone app, the tray and the Windows executable, so they
always match. Standard library only (no Pillow): run `python3 scripts/icons.py`.

  mobile/assets/icon.png              1024 px, full bleed (iOS masks it itself)
  mobile/assets/adaptive-icon.png     Android adaptive icon foreground: the glyph alone, inside the safe zone
  mobile/assets/toolbar/*.png         Android header buttons (SF Symbols do not exist there)
  pc/internal/icons/tray-idle.*       rounded icon for the tray / menu bar
  pc/internal/icons/tray-connected.*  the same with a green "connected" badge
  pc/internal/icons/app.ico           multi-size icon for phoneset.exe
  pc/internal/icons/logo.png          for the local pairing page
"""

import math
import os
import struct
import zlib

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))

BLUE_TOP = (0x3B, 0x6B, 0xFF)
BLUE_BOTTOM = (0x25, 0x4B, 0xE0)
WHITE = (255, 255, 255)
GREEN = (0x30, 0xD1, 0x58)

# Android crops an adaptive icon to a circle 66/108 of its width, so the glyph is shrunk to sit in it.
FOREGROUND_SCALE = 0.62
GLYPH_CENTER = (580, 512)  # the middle of the phone and its waves together, in 1024-unit space


def sd_round_rect(px, py, cx, cy, w, h, r):
    dx = abs(px - cx) - (w / 2 - r)
    dy = abs(py - cy) - (h / 2 - r)
    return math.hypot(max(dx, 0), max(dy, 0)) + min(max(dx, dy), 0) - r


def sd_arc(px, py, cx, cy, rad, thick, half_angle):
    x, y = px - cx, py - cy
    d, a = math.hypot(x, y), math.atan2(y, x)
    if abs(a) <= half_angle:
        return abs(d - rad) - thick / 2
    ea = half_angle if a > 0 else -half_angle
    return math.hypot(px - (cx + rad * math.cos(ea)), py - (cy + rad * math.sin(ea))) - thick / 2


def mix(base, top, c):
    return tuple(base[i] * (1 - c) + top[i] * c for i in range(3))


def render(size, rounded=False, badge=False, foreground=False):
    """RGBA bytes. Shapes live in a 1024-unit space and scale to `size`.

    `foreground` draws only the white glyph on transparency, centred and shrunk for Android's
    adaptive icon (and its monochrome variant)."""
    ss = 4 if size <= 256 else 1  # supersample small sizes for clean edges
    n = size * ss
    k = 1024 / n  # 1024-space units per rendered pixel
    px_per_unit = n / 1024 * (FOREGROUND_SCALE if foreground else 1)
    phone_cx, phone_cy = 400, 512
    out = []
    for y in range(n):
        row = []
        row_y = (y + 0.5) * k
        base = mix(BLUE_TOP, BLUE_BOTTOM, row_y / 1024)
        for x in range(n):
            fx, fy = (x + 0.5) * k, row_y
            if foreground:
                fx = (fx - 512) / FOREGROUND_SCALE + GLYPH_CENTER[0]
                fy = (fy - 512) / FOREGROUND_SCALE + GLYPH_CENTER[1]
            c = lambda d: max(0.0, min(1.0, 0.5 - d * px_per_unit))
            body = c(sd_round_rect(fx, fy, phone_cx, phone_cy, 360, 620, 76))
            screen = c(sd_round_rect(fx, fy, phone_cx, phone_cy + 10, 296, 520, 44))
            waves = 0.0
            for rad in (130, 230, 330):
                waves = max(waves, c(sd_arc(fx, fy, phone_cx + 190, phone_cy, rad, 38, math.radians(38))))
            dot = c(math.hypot(fx - phone_cx, fy - (phone_cy + 10)) - 52)
            if foreground:
                row.append((255, 255, 255, max(body * (1 - screen), waves, dot)))
                continue
            col = mix(base, WHITE, body)
            col = mix(col, base, screen)
            col = mix(col, WHITE, waves)
            col = mix(col, WHITE, dot)
            if badge:
                col = mix(col, WHITE, c(math.hypot(fx - 830, fy - 830) - 150))
                col = mix(col, GREEN, c(math.hypot(fx - 830, fy - 830) - 112))
            alpha = c(sd_round_rect(fx, fy, 512, 512, 1024, 1024, 230)) if rounded else 1.0
            row.append((col[0], col[1], col[2], alpha))
        out.append(row)
    # box-filter down to the target size
    pix = bytearray()
    for y in range(size):
        for x in range(size):
            r = g = b = a = 0.0
            for dy in range(ss):
                for dx in range(ss):
                    pr, pg, pb, pa = out[y * ss + dy][x * ss + dx]
                    r += pr * pa
                    g += pg * pa
                    b += pb * pa
                    a += pa
            count = ss * ss
            if a > 0:
                pix += bytes((int(r / a), int(g / a), int(b / a), int(255 * a / count)))
            else:
                pix += bytes((0, 0, 0, 0))
    return bytes(pix)


def glyph(size, shape, ss=4):
    """RGBA bytes of a black shape on transparency (the system tints it).

    `shape(x, y, c)` gets a point in a 0..1 square and `c`, which turns a signed distance in that
    space into pixel coverage."""
    n = size * ss
    unit = 1 / n
    cover = lambda d: max(0.0, min(1.0, 0.5 - d / unit))
    samples = [[shape((x + 0.5) * unit, (y + 0.5) * unit, cover) for x in range(n)] for y in range(n)]
    pix = bytearray()
    for y in range(size):
        for x in range(size):
            total = sum(samples[y * ss + dy][x * ss + dx] for dy in range(ss) for dx in range(ss))
            pix += bytes((0, 0, 0, int(255 * total / (ss * ss))))
    return bytes(pix)


def plus(x, y, c):
    return max(
        c(sd_round_rect(x, y, 0.5, 0.5, 0.64, 0.1, 0.05)),
        c(sd_round_rect(x, y, 0.5, 0.5, 0.1, 0.64, 0.05)),
    )


def gear(x, y, c):
    """Eight-toothed cog with a round hole."""
    dx, dy = x - 0.5, y - 0.5
    d, a = math.hypot(dx, dy), math.atan2(dy, dx)
    t = abs((a / (2 * math.pi / 8)) % 1 - 0.5)  # 0 in the middle of a gap, 0.5 in the middle of a tooth
    tooth = max(0.0, min(1.0, (t - 0.22) / 0.1))
    return c(d - (0.3 + 0.12 * tooth)) * (1 - c(d - 0.15))


def png(rgba, size, opaque=False):
    raw = b""
    stride = size * 4
    for y in range(size):
        raw += b"\x00" + rgba[y * stride : (y + 1) * stride]
    if opaque:  # smaller file: drop alpha
        raw = b"".join(
            b"\x00" + bytes(v for i, v in enumerate(rgba[y * stride : (y + 1) * stride]) if i % 4 != 3)
            for y in range(size)
        )

    def chunk(tag, data):
        return struct.pack(">I", len(data)) + tag + data + struct.pack(">I", zlib.crc32(tag + data) & 0xFFFFFFFF)

    header = struct.pack(">IIBBBBB", size, size, 8, 2 if opaque else 6, 0, 0, 0)
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", header) + chunk(b"IDAT", zlib.compress(raw, 9)) + chunk(b"IEND", b"")


def ico(images):
    """images: list of (size, png_bytes). PNG-compressed entries, Windows Vista and later."""
    out = struct.pack("<HHH", 0, 1, len(images))
    offset = 6 + 16 * len(images)
    blobs = b""
    for size, data in images:
        out += struct.pack("<BBBBHHII", 0 if size >= 256 else size, 0 if size >= 256 else size, 0, 0, 1, 32, len(data), offset)
        offset += len(data)
        blobs += data
    return out + blobs


def write(path, data):
    os.makedirs(os.path.dirname(path), exist_ok=True)
    with open(path, "wb") as f:
        f.write(data)
    print(f"{os.path.relpath(path, ROOT):55s} {len(data):>7} bytes")


def main():
    icons = os.path.join(ROOT, "pc", "internal", "icons")

    write(os.path.join(ROOT, "mobile", "assets", "icon.png"), png(render(1024), 1024, opaque=True))
    write(os.path.join(ROOT, "mobile", "assets", "adaptive-icon.png"), png(render(1024, foreground=True), 1024))
    for name, shape in (("plus", plus), ("settings", gear)):
        write(os.path.join(ROOT, "mobile", "assets", "toolbar", f"{name}.png"), png(glyph(72, shape), 72))

    sizes = (16, 24, 32, 48, 64)
    for name, badge in (("idle", False), ("connected", True)):
        frames = {s: png(render(s, rounded=True, badge=badge), s) for s in sizes}
        write(os.path.join(icons, f"tray-{name}.png"), png(render(64, rounded=True, badge=badge), 64))
        write(os.path.join(icons, f"tray-{name}.ico"), ico([(s, frames[s]) for s in sizes]))

    app_sizes = (16, 24, 32, 48, 64, 128, 256)
    write(os.path.join(icons, "app.ico"), ico([(s, png(render(s, rounded=True), s)) for s in app_sizes]))
    write(os.path.join(icons, "logo.png"), png(render(128, rounded=True), 128))


if __name__ == "__main__":
    main()
