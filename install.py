#!/usr/bin/env python3
"""Build ergotime-server and install it on a remote host.

Compiles the linux/amd64 binary locally, copies it with scp to
/opt/ergotime2/bin/ergotime-server on --server, installs the systemd unit
when the remote copy differs, and restarts the service.

Usage:
  ./install.py --server user@host
"""

import argparse
import os
import secrets
import shlex
import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent
UNIT = ROOT / "deploy" / "ergotime-server.service"
BINARY = ROOT / "bin" / "ergotime-server"
REMOTE_PREFIX = "/opt/ergotime2"
REMOTE_BIN = f"{REMOTE_PREFIX}/bin/ergotime-server"
SERVICE = "ergotime-server"

# Copied to the host and executed there. Re-execs with sudo when needed.
# Arguments: uploaded binary, uploaded unit.
REMOTE_SCRIPT = r"""
set -euo pipefail
if [ "$(id -u)" -ne 0 ]; then
  exec sudo bash "$0" "$@"
fi
src_bin=$1
src_unit=$2
prefix=/opt/ergotime2
dest_bin=/opt/ergotime2/bin/ergotime-server
unit=/etc/systemd/system/ergotime-server.service

if ! getent group ergotime >/dev/null; then
  groupadd --system ergotime
fi
if ! id ergotime >/dev/null 2>&1; then
  useradd --system --gid ergotime --home-dir "$prefix" --shell /usr/sbin/nologin ergotime
fi
install -d -o ergotime -g ergotime -m 755 "$prefix" "$prefix/bin"
install -m 755 -o ergotime -g ergotime "$src_bin" "$dest_bin"

changed=0
if [ ! -f "$unit" ] || ! cmp -s "$src_unit" "$unit"; then
  install -m 644 "$src_unit" "$unit"
  changed=1
  echo "==> Updated $unit"
else
  echo "==> systemd unit already matches"
fi
systemctl daemon-reload
systemctl enable ergotime-server
systemctl restart ergotime-server
rm -f "$src_bin" "$src_unit"
sleep 1
if ! systemctl is-active --quiet ergotime-server; then
  systemctl --no-pager --full status ergotime-server || true
  journalctl -u ergotime-server -n 30 --no-pager || true
  echo "error: ergotime-server is not running" >&2
  exit 1
fi
systemctl --no-pager --full status ergotime-server
"""


def die(msg: str) -> None:
    print(f"error: {msg}", file=sys.stderr)
    sys.exit(1)


def run(cmd: list[str], **kwargs) -> None:
    print("==>", " ".join(shlex.quote(part) for part in cmd))
    subprocess.run(cmd, check=True, **kwargs)


def build() -> None:
    dist = ROOT / "web" / "dist" / "index.html"
    if not dist.is_file():
        print("==> web/dist missing, building the embedded UI")
        run(["make", "frontend"], cwd=ROOT)
    env = os.environ.copy()
    env["GOOS"] = "linux"
    env["GOARCH"] = "amd64"
    env["CGO_ENABLED"] = "0"
    BINARY.parent.mkdir(parents=True, exist_ok=True)
    run(
        ["go", "build", "-ldflags=-s -w", "-o", str(BINARY), "./cmd/server"],
        cwd=ROOT,
        env=env,
    )
    if not BINARY.is_file() or BINARY.stat().st_size == 0:
        die(f"build did not produce {BINARY}")


def deploy(server: str) -> None:
    token = secrets.token_hex(4)
    remote_bin = f"/tmp/ergotime-server.{token}"
    remote_unit = f"/tmp/ergotime-server.{token}.service"
    remote_script = f"/tmp/ergotime-install.{token}.sh"
    script = ROOT / "bin" / f".install-{token}.sh"
    script.write_text(REMOTE_SCRIPT, encoding="utf-8")
    try:
        run(["scp", str(BINARY), f"{server}:{remote_bin}"])
        run(["scp", str(UNIT), f"{server}:{remote_unit}"])
        run(["scp", str(script), f"{server}:{remote_script}"])
        # -tt keeps a tty so sudo can ask for a password.
        remote = (
            f"bash {shlex.quote(remote_script)} "
            f"{shlex.quote(remote_bin)} {shlex.quote(remote_unit)}; "
            f"rc=$?; rm -f {shlex.quote(remote_script)}; exit $rc"
        )
        print(f"==> ssh {server} install {REMOTE_BIN} and restart {SERVICE}")
        run(["ssh", "-tt", server, remote])
    finally:
        script.unlink(missing_ok=True)


def main() -> None:
    parser = argparse.ArgumentParser(
        description=__doc__,
        formatter_class=argparse.RawDescriptionHelpFormatter,
    )
    parser.add_argument(
        "--server",
        required=True,
        help="ssh destination (host or user@host) that receives the binary",
    )
    args = parser.parse_args()
    server = args.server.strip()
    if not server or any(c.isspace() for c in server):
        die("--server must be a single ssh destination, for example user@host")
    if not UNIT.is_file():
        die(f"missing unit file {UNIT}")
    build()
    deploy(server)
    print(f"==> {SERVICE} restarted on {server} ({REMOTE_BIN})")


if __name__ == "__main__":
    try:
        main()
    except subprocess.CalledProcessError as exc:
        die(f"command failed with exit {exc.returncode}")
