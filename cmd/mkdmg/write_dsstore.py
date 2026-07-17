#!/usr/bin/env python3
"""Write the styled .DS_Store into a mounted DMG volume, then exit.

Kept deliberately tiny and short-lived: this process must be dead before
the volume is detached, so nothing lingers holding the filesystem busy.
Values mirror the go-apps family DMG layout (660x400 window, app at
165,190, Applications at 495,190, background in .background/).

Requires: pip install ds-store mac-alias

Usage: write_dsstore.py <mount_point> <app_name> <background_relpath> [readme_name]
"""

import os
import sys

from ds_store import DSStore
from mac_alias import Alias

mount, appname, bgrel = sys.argv[1], sys.argv[2], sys.argv[3]
readme = sys.argv[4] if len(sys.argv) > 4 else None

bwsp = {
    "ShowStatusBar": False,
    "WindowBounds": "{{200, 120}, {660, 400}}",
    "ContainerShowSidebar": False,
    "PreviewPaneVisibility": False,
    "SidebarWidth": 180,
    "ShowTabView": False,
    "ShowToolbar": False,
    "ShowPathbar": False,
    "ShowSidebar": False,
}

icvp = {
    "viewOptionsVersion": 1,
    "backgroundType": 2,  # image
    "backgroundImageAlias": Alias.for_file(os.path.join(mount, bgrel)).to_bytes(),
    "gridOffsetX": 0.0,
    "gridOffsetY": 0.0,
    "gridSpacing": 100.0,
    "arrangeBy": "none",
    "showIconPreview": False,
    "showItemInfo": False,
    "labelOnBottom": True,
    "textSize": 13.0,
    "iconSize": 110.0,
    "scrollPositionX": 0.0,
    "scrollPositionY": 0.0,
}

with DSStore.open(os.path.join(mount, ".DS_Store"), "w+") as d:
    d["."]["vSrn"] = ("long", 1)
    d["."]["bwsp"] = bwsp
    d["."]["icvp"] = icvp
    d["."]["icvl"] = (b"type", b"icnv")
    d[appname]["Iloc"] = (165, 190)
    d["Applications"]["Iloc"] = (495, 190)
    if readme:
        d[readme]["Iloc"] = (330, 320)  # bottom center, below the arrow

print("dsstore written")
