# Copyright 2026 Abhinav Gautam
# SPDX-License-Identifier: Apache-2.0
"""Decode BTHome packets with bthome-ble, for the Go interop test.

Reads a JSON list of {"mac", "sd" (service data after the UUID, hex),
"uuid" (16-bit, number), "key" (hex or "")} from the file named on the
command line and prints one JSON object per packet: the sensor, binary
sensor and event values bthome-ble found, or the error it raised.
"""

import json
import sys
import time
from datetime import datetime

from bthome_ble import BTHomeBluetoothDeviceData


class Info:
    """The fields of a Bluetooth service info that bthome-ble reads."""

    def __init__(self, mac, uuid, data):
        self.name = mac
        self.address = mac
        self.rssi = -60
        self.manufacturer_data = {}
        self.service_data = {"0000%04x-0000-1000-8000-00805f9b34fb" % uuid: data}
        self.service_uuids = list(self.service_data)
        self.source = "sigma"
        self.time = time.monotonic()
        self.connectable = False
        self.tx_power = None
        self.raw = None
        self.device = None
        self.advertisement = None


def plain(v):
    if isinstance(v, datetime):
        return v.timestamp()
    return v


def main():
    with open(sys.argv[1]) as f:
        vectors = json.load(f)
    for v in vectors:
        key = bytes.fromhex(v["key"]) if v.get("key") else None
        p = BTHomeBluetoothDeviceData(bindkey=key)
        out = {"sensors": {}, "binary": {}, "events": []}
        try:
            upd = p.update(Info(v["mac"], v["uuid"], bytes.fromhex(v["sd"])))
            for dk, sv in upd.entity_values.items():
                if dk.key == "signal_strength":
                    continue
                out["sensors"][dk.key] = plain(sv.native_value)
            for dk, bv in upd.binary_entity_values.items():
                out["binary"][dk.key] = bv.native_value
            for dk, ev in upd.events.items():
                out["events"].append({"key": dk.key, "type": ev.event_type, "props": ev.event_properties})
            out["bindkey_verified"] = p.bindkey_verified
        except Exception as e:  # reported to the Go side as a check result
            out["error"] = "%s: %s" % (type(e).__name__, e)
        print(json.dumps(out, default=str), flush=True)


if __name__ == "__main__":
    main()
