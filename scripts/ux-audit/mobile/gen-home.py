"""スマホの点検（2026-11-03）で、アプリの HOME（files/）に入れる ~/.ssh を作る。

使い方: python3 gen-home.py <clean|rough> <出力先>
出力先の .ssh を tar にして、adb の run-as でアプリの files/ へ展開する（push-home.sh）。
接続できる接続先は、エミュレータから見たこのマシン（10.0.2.2）の使い捨ての sshd（29299）を指す。
"""
import shutil
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
FIXTURE = HERE.parent / "fixture"
SSHD = ["HostName 10.0.2.2", "Port 29299"]


def host(alias: str, lines: list[str]) -> str:
    return "\n".join([f"Host {alias}", *(f"\t{line}" for line in lines), ""])


def write(path: Path, text: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(text)


def clean(ssh: Path) -> None:
    write(ssh / "config", "\n".join([
        "# >>> sshc groups (generated). Child groups first: OpenSSH keeps the first value it reads.",
        "# Edit through the UI; lines between these markers are replaced on the next save.",
        "Include connections/production/*.conf",
        "Include connections/home-lab/*.conf",
        "# <<< sshc groups",
        "",
        host("web-01", [*SSHD, "User deploy", "IdentityFile ~/.ssh/id_ed25519_deploy", "IdentitiesOnly yes"]),
        host("password-only", [*SSHD, "User deploy", "PubkeyAuthentication no"]),
        host("key-with-passphrase", [*SSHD, "User deploy", "IdentityFile ~/.ssh/id_ed25519_passphrase", "IdentitiesOnly yes"]),
        host("bastion", ["HostName 203.0.113.10", "User ops", "Port 2222"]),
        "Host *\n\tServerAliveInterval 30\n",
    ]))
    write(ssh / "connections/production/web.conf", "\n".join([
        host("web-02", [*SSHD, "User deploy", "IdentityFile ~/.ssh/id_ed25519_deploy", "IdentitiesOnly yes"]),
        host("db-primary", ["HostName 198.51.100.20", "User postgres", "ProxyJump bastion"]),
        host("analytics-batch-worker-very-long-hostname-01", ["HostName 198.51.100.40", "User batch", "ProxyJump bastion"]),
    ]))
    write(ssh / "connections/home-lab/lab.conf", "\n".join([
        host("nas", ["HostName 192.168.10.20", "User admin"]),
        host("raspberrypi", ["HostName 192.168.10.31", "User pi"]),
    ]))


def rough(ssh: Path) -> None:
    clean(ssh)
    regions = ["tokyo", "orchid", "fukuoka", "sapporo", "singapore", "frankfurt", "virginia", "oregon", "sydney", "saopaulo"]
    roles = ["web", "api", "worker", "cache", "db", "search", "queue", "batch"]
    groups = []
    for index, region in enumerate(regions):
        group = f"fleet-{region}"
        groups.append(group)
        blocks = [host(f"{region}-{role}-{n:02d}", [f"HostName 10.{index}.{roles.index(role)}.{n}", "User deploy", "ProxyJump bastion"]) for role in roles for n in (1, 2)]
        write(ssh / "connections" / group / "hosts.conf", "\n".join(blocks))
    write(ssh / "connections/broken/broken.conf", "\n".join([
        host("refused-01", ["HostName 10.0.2.2", "Port 1", "User deploy"]),
        host("timeout-01", ["HostName 192.0.2.1", "User deploy", "ConnectTimeout 5"]),
        host("wrong-user", [*SSHD, "User nobody-here", "IdentityFile ~/.ssh/id_ed25519_deploy"]),
        host("customer-acme-corporation-production-kubernetes-node-pool-a-0001", ["HostName ip-10-200-31-187.ap-northeast-1.compute.internal", "User ec2-user", "ProxyJump bastion"]),
    ]))
    config = (ssh / "config").read_text()
    includes = "\n".join(f"Include connections/{group}/*.conf" for group in [*groups, "broken"])
    (ssh / "config").write_text(config.replace("Include connections/home-lab/*.conf", f"Include connections/home-lab/*.conf\n{includes}"))


def main() -> None:
    kind, out = sys.argv[1], Path(sys.argv[2])
    ssh = out / ".ssh"
    if out.exists():
        shutil.rmtree(out)
    ssh.mkdir(parents=True)
    {"clean": clean, "rough": rough}[kind](ssh)
    for name in ["id_ed25519_deploy", "id_ed25519_deploy.pub", "id_ed25519_passphrase", "id_ed25519_passphrase.pub"]:
        shutil.copyfile(FIXTURE / name, ssh / name)
    for path in ssh.rglob("*"):
        path.chmod(0o700 if path.is_dir() else 0o600)
    print("wrote", ssh)


if __name__ == "__main__":
    main()
