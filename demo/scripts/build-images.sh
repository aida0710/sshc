#!/usr/bin/env bash
set -euo pipefail

demo_directory=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd)
repository_directory=$(cd -- "$demo_directory/.." && pwd)
images_directory="$demo_directory/images"
mkdir -p "$images_directory"
rootfs_directory=$(mktemp -d)
container_id=
cleanup() {
    if [ -n "$container_id" ]; then docker rm "$container_id" >/dev/null; fi
    rm -rf -- "$rootfs_directory"
}
trap cleanup EXIT

fetch_verified() {
    local url=$1 filename=$2 checksum=$3
    curl -fsSL --retry 2 "$url" -o "$images_directory/$filename.partial"
    printf '%s  %s\n' "$checksum" "$images_directory/$filename.partial" | sha256sum --check --status
    mv -- "$images_directory/$filename.partial" "$images_directory/$filename"
}

# Freeze the public emulator boot assets; stop if the upstream kernel changes.
fetch_verified https://i.copy.sh/buildroot-bzimage68.bin kernel.bin 507a759c70ab7a490a233be454d0b5b88bc667956a410b531cb4edc091e2eb1c
bios_base=https://raw.githubusercontent.com/copy/v86/6db8b157974dbaf1b54d2c2ec12dd71ddc1891e9/bios
fetch_verified "$bios_base/seabios.bin" seabios.bin 73e3f359102e3a9982c35fce98eb7cd08f18303ac7f1ba6ebfbe6cdc1c244d98
fetch_verified "$bios_base/vgabios.bin" vgabios.bin a4bc0d80cc3ca028c73dafa8fee396b8d054ce87ebd8abfbd31b06b437607880

cd -- "$repository_directory"
version=${VERSION:-$(git describe --tags --exact-match 2>/dev/null || printf 'dev')}
if ! [[ "$version" =~ ^(dev|v[0-9]+\.[0-9]+\.[0-9]+([-+][A-Za-z0-9.-]+)?)$ ]]; then
    printf 'Invalid demo version\n' >&2
    exit 1
fi
GOOS=linux GOARCH=386 GO386=sse2 CGO_ENABLED=0 go build -trimpath -ldflags="-s -w -X main.version=$version" -o "$images_directory/sshc" ./cmd/sshc
GOOS=linux GOARCH=386 GO386=sse2 CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o "$images_directory/sshc-demo-bridge" ./demo/guestbridge

image_tag="sshc-browser-demo:$(git rev-parse --short HEAD)"
docker build --platform linux/386 --tag "$image_tag" --file demo/guest/Dockerfile demo/guest
container_id=$(docker create --platform linux/386 "$image_tag")
docker export "$container_id" | tar -xf - -C "$rootfs_directory"
cp -- "$images_directory/sshc" "$images_directory/sshc-demo-bridge" "$rootfs_directory/usr/local/bin/"
python3 - "$rootfs_directory" "$images_directory" "$version" <<'PY'
from pathlib import Path
import json
import sys
rootfs, images = map(Path, sys.argv[1:3])
version = sys.argv[3]
packages = []
for package in (rootfs / 'lib/apk/db/installed').read_text().split('\n\n'):
    fields = dict(line.split(':', 1) for line in package.splitlines() if ':' in line)
    if 'P' in fields:
        packages.append({'name': fields['P'], 'version': fields['V'], 'license': fields.get('L', ''),
                         'origin': fields.get('o', ''), 'sourceCommit': fields.get('c', '')})
(images / 'alpine-packages.json').write_text(json.dumps(packages, indent=2) + '\n')
(images / 'build-version.json').write_text(json.dumps({'version': version}) + '\n')
PY

# Exporting as an unprivileged user changes host ownership. Linux and OpenSSH need root-owned image files.
cd -- "$rootfs_directory"
find . -print0 | cpio --null --owner=0:0 -o --format=newc | gzip -1 > "$images_directory/client.cpio.gz.partial"
mv -- "$images_directory/client.cpio.gz.partial" "$images_directory/client.cpio.gz"
find . ! -path './usr/local/bin/sshc' ! -path './usr/local/bin/sshc-demo-bridge' -print0 \
    | cpio --null --owner=0:0 -o --format=newc | gzip -1 > "$images_directory/server.cpio.gz.partial"
mv -- "$images_directory/server.cpio.gz.partial" "$images_directory/server.cpio.gz"
