#!/usr/bin/env python3
"""One-shot, LAN-only RAM binary sender for explicitly authorized HGFW tests.

Does not connect to the router, run router commands, or install any service.
Start from the PC; router initiates one TCP receive with BusyBox netcat.
"""
import argparse
import hashlib
import pathlib
import socket

MAX_SIZE = 12 * 1024 * 1024


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--file", type=pathlib.Path,
                        default=pathlib.Path(__file__).resolve().parents[1] / "build" / "hgfw-armv7-next")
    parser.add_argument("--bind", default="192.168.1.8")
    parser.add_argument("--peer", default="192.168.1.2")
    parser.add_argument("--port", type=int, default=8765)
    args = parser.parse_args()
    if not 1 <= args.port <= 65535:
        parser.error("port must be between 1 and 65535")
    payload = args.file.read_bytes()
    if not payload or len(payload) > MAX_SIZE:
        parser.error("invalid HGFW binary size")
    checksum = hashlib.sha256(payload).hexdigest()
    print(f"FILE={args.file}\nBYTES={len(payload)}\nSHA256={checksum}", flush=True)
    with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as server:
        server.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        server.bind((args.bind, args.port))
        server.listen(1)
        server.settimeout(180)
        print(f"READY={args.bind}:{args.port} PEER={args.peer}", flush=True)
        conn, address = server.accept()
        with conn:
            conn.settimeout(30)
            if address[0] != args.peer:
                raise RuntimeError(f"unexpected peer {address[0]}, no binary transmitted")
            conn.sendall(payload)
    print("SENT_COMPLETE=true", flush=True)


if __name__ == "__main__":
    main()
