#!/usr/bin/env python3
"""Embeds the phoneset icon in the Windows executables.

Writes pc/cmd/phoneset/rsrc_windows_{amd64,arm64}.syso, COFF objects that the Go linker
picks up by their file name and turns into the .exe's icon resources (what Explorer shows).
Standard library only: run `python3 scripts/winres.py` after `python3 scripts/icons.py`.
"""

import os
import struct

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
ICO = os.path.join(ROOT, "pc", "internal", "icons", "app.ico")
OUT = os.path.join(ROOT, "pc", "cmd", "phoneset")

RT_ICON, RT_GROUP_ICON, LANG_EN_US = 3, 14, 0x0409
# machine, relocation type that stores an image-base-relative 32-bit address
ARCHES = {"amd64": (0x8664, 0x0003), "arm64": (0xAA64, 0x0002)}


def read_ico(path):
    data = open(path, "rb").read()
    _, kind, count = struct.unpack_from("<HHH", data, 0)
    assert kind == 1, "not an .ico file"
    images = []
    for i in range(count):
        w, h, colors, _, planes, bits, size, offset = struct.unpack_from("<BBBBHHII", data, 6 + 16 * i)
        images.append({"w": w, "h": h, "colors": colors, "planes": planes, "bits": bits, "data": data[offset : offset + size]})
    return images


def build_tree(images):
    """Returns (section bytes, offsets of every data entry's OffsetToData field)."""
    n = len(images)

    def directory(entries):
        # entries: list of (id, target offset, is_subdirectory); IDs must be ascending
        out = struct.pack("<IIHHHH", 0, 0, 0, 0, 0, len(entries))
        for ident, target, is_dir in entries:
            out += struct.pack("<II", ident, target | (0x80000000 if is_dir else 0))
        return out

    # Sizes of each piece, to lay them out back to back.
    root_size = 16 + 2 * 8
    icon_dir_size = 16 + n * 8
    group_dir_size = 16 + 8
    lang_size = 16 + 8
    entry_size = 16

    root_off = 0
    icon_dir_off = root_off + root_size
    group_dir_off = icon_dir_off + icon_dir_size
    lang_off = [group_dir_off + group_dir_size + i * lang_size for i in range(n + 1)]  # n icons, then the group
    entries_off = lang_off[-1] + lang_size
    entry_off = [entries_off + i * entry_size for i in range(n + 1)]
    blobs_off = entries_off + (n + 1) * entry_size

    group = struct.pack("<HHH", 0, 1, n) + b"".join(
        struct.pack("<BBBBHHIH", im["w"], im["h"], im["colors"], 0, im["planes"], im["bits"], len(im["data"]), i + 1)
        for i, im in enumerate(images)
    )
    blobs = [im["data"] for im in images] + [group]
    blob_off, cursor = [], blobs_off
    for blob in blobs:
        blob_off.append(cursor)
        cursor += (len(blob) + 3) & ~3

    section = bytearray()
    section += directory([(RT_ICON, icon_dir_off, True), (RT_GROUP_ICON, group_dir_off, True)])
    section += directory([(i + 1, lang_off[i], True) for i in range(n)])
    section += directory([(1, lang_off[n], True)])
    for i in range(n + 1):
        section += directory([(LANG_EN_US, entry_off[i], False)])
    reloc_at = []
    for i in range(n + 1):
        reloc_at.append(len(section))  # the OffsetToData field of this entry
        section += struct.pack("<IIII", blob_off[i], len(blobs[i]), 0, 0)
    assert len(section) == blobs_off
    for blob in blobs:
        section += blob + b"\0" * (-len(blob) % 4)
    return bytes(section), reloc_at


def coff_object(machine, reloc_type, section, reloc_at):
    header_size, section_header_size = 20, 40
    raw_off = header_size + section_header_size
    reloc_off = raw_off + len(section)
    symtab_off = reloc_off + 10 * len(reloc_at)

    out = struct.pack("<HHIIIHH", machine, 1, 0, symtab_off, 1, 0, 0)
    out += struct.pack(
        "<8sIIIIIIHHI", b".rsrc", 0, 0, len(section), raw_off, reloc_off, 0, len(reloc_at), 0, 0x40000040
    )
    out += section
    for at in reloc_at:
        out += struct.pack("<IIH", at, 0, reloc_type)  # symbol 0 is the .rsrc section itself
    out += struct.pack("<8sIhHBB", b".rsrc", 0, 1, 0, 3, 0)  # static symbol for section 1
    out += struct.pack("<I", 4)  # empty string table
    return out


def main():
    images = read_ico(ICO)
    section, reloc_at = build_tree(images)
    for arch, (machine, reloc_type) in ARCHES.items():
        path = os.path.join(OUT, f"rsrc_windows_{arch}.syso")
        with open(path, "wb") as f:
            f.write(coff_object(machine, reloc_type, section, reloc_at))
        print(f"{os.path.relpath(path, ROOT):55s} {os.path.getsize(path):>7} bytes, {len(images)} icon sizes")


if __name__ == "__main__":
    main()
