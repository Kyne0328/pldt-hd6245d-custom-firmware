import hashlib
import pathlib
import socket
import subprocess
import sys
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]
SENDER = ROOT / "tools" / "hgfw_send.py"


class SenderTests(unittest.TestCase):
    def test_one_shot_local_transfer_matches_hash(self):
        payload = b"hgfw-local-fixture" * 40
        with tempfile.TemporaryDirectory() as td:
            binary = pathlib.Path(td) / "binary"
            binary.write_bytes(payload)
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as hold:
                hold.bind(("127.0.0.1", 0))
                port = hold.getsockname()[1]
            proc = subprocess.Popen(
                [sys.executable, str(SENDER), "--file", str(binary),
                 "--bind", "127.0.0.1", "--peer", "127.0.0.1",
                 "--port", str(port)],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True,
            )
            try:
                lines = [proc.stdout.readline().strip() for _ in range(4)]
                self.assertEqual(lines[0], "FILE=" + str(binary))
                self.assertEqual(lines[1], "BYTES=" + str(len(payload)))
                self.assertEqual(lines[2], "SHA256=" + hashlib.sha256(payload).hexdigest())
                self.assertTrue(lines[3].startswith("READY=127.0.0.1:"))
                with socket.create_connection(("127.0.0.1", port), timeout=4) as conn:
                    chunks = []
                    while True:
                        piece = conn.recv(4096)
                        if not piece:
                            break
                        chunks.append(piece)
                self.assertEqual(b"".join(chunks), payload)
                self.assertEqual(proc.wait(timeout=5), 0)
                self.assertEqual(proc.stdout.readline().strip(), "SENT_COMPLETE=true")
            finally:
                if proc.poll() is None:
                    proc.kill()
                proc.communicate(timeout=5)
