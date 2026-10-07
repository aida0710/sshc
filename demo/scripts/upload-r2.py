"""Publish only the generated static demo; credentials are supplied separately."""
import argparse
from concurrent.futures import ThreadPoolExecutor
import hashlib
import json
import mimetypes
from pathlib import Path
import re
import os

import boto3
from botocore.config import Config
from botocore.exceptions import ClientError


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--release", required=True)
    parser.add_argument("--manifest", required=True)
    options = parser.parse_args()
    if not re.fullmatch(r"[A-Za-z0-9._-]+", options.release):
        parser.error("Invalid release name")

    credentials = json.loads(Path(os.environ["SSHC_R2_CREDENTIALS_FILE"]).read_text())
    bucket = credentials["bucket"]
    client = boto3.client("s3", endpoint_url=credentials["endpoint"], region_name="auto",
        aws_access_key_id=credentials["accessKeyID"], aws_secret_access_key=credentials["secretAccessKey"],
        config=Config(signature_version="s3v4", request_checksum_calculation="when_required",
                      response_checksum_validation="when_required", retries={"max_attempts": 3}))
    # Check access before publishing, and never remove existing objects.
    client.list_objects_v2(Bucket=bucket, MaxKeys=1)
    directory = Path(__file__).resolve().parents[1] / "dist"
    files = sorted(path for path in directory.rglob("*") if path.is_file())
    if not (directory / "index.html").is_file():
        raise RuntimeError("Build demo/dist before publishing")
    prefix = f"releases/{options.release}/"
    # Immutable URLs must never be reused, even if a previous upload was interrupted.
    if client.list_objects_v2(Bucket=bucket, Prefix=prefix, MaxKeys=1).get("KeyCount", 0):
        raise RuntimeError("Release already exists; choose a new release name")

    config_path = directory / "config.json"
    configuration = json.loads(config_path.read_text())
    configuration["entryURL"] = "../../index.html"
    shared_images = configuration["imageBaseURL"] == "./images/"
    image_files = [path for path in files if path.is_relative_to(directory / "images")]
    image_prefix = ""
    if shared_images:
        # Content-based URLs keep unchanged VM images cached across UI releases.
        image_digests = {path.relative_to(directory / "images").as_posix():
                        hashlib.sha256(path.read_bytes()).hexdigest() for path in image_files}
        image_version = hashlib.sha256(json.dumps(image_digests, sort_keys=True).encode()).hexdigest()
        image_prefix = f"vm-images/{image_version}/"
        configuration["imageBaseURL"] = "../../" + image_prefix

    def upload(path):
        is_shared_image = shared_images and path in image_files
        contents = (json.dumps(configuration) + "\n").encode() if path == config_path else path.read_bytes()
        digest = hashlib.sha256(contents).hexdigest()
        relative_image = path.relative_to(directory / "images").as_posix() if is_shared_image else ""
        key = image_prefix + relative_image if is_shared_image else prefix + path.relative_to(directory).as_posix()
        content_type = {
            ".wasm": "application/wasm", ".js": "text/javascript; charset=utf-8",
            ".css": "text/css; charset=utf-8", ".html": "text/html; charset=utf-8",
            ".json": "application/json", ".gz": "application/octet-stream",
        }.get(path.suffix, mimetypes.guess_type(path.name)[0] or "application/octet-stream")
        saved = None
        if is_shared_image:
            try:
                saved = client.head_object(Bucket=bucket, Key=key)
            except ClientError as error:
                if error.response["ResponseMetadata"]["HTTPStatusCode"] != 404:
                    raise
        if saved is None:
            client.put_object(Bucket=bucket, Key=key, Body=contents, ContentType=content_type,
                CacheControl="public, max-age=31536000, immutable", Metadata={"sha256": digest})
            saved = client.head_object(Bucket=bucket, Key=key)
        if saved["ContentLength"] != len(contents) or saved["Metadata"].get("sha256") != digest:
            raise RuntimeError(f"Object verification failed: {key}")
        return {"key": key, "bytes": len(contents), "sha256": digest}

    with ThreadPoolExecutor(max_workers=4) as uploads:
        objects = list(uploads.map(upload, files))
    manifest = {"release": options.release, "objects": objects,
                "totalBytes": sum(entry["bytes"] for entry in objects),
                "imageBaseURL": configuration["imageBaseURL"]}
    manifest_bytes = (json.dumps(manifest, indent=2) + "\n").encode()
    Path(options.manifest).write_bytes(manifest_bytes)
    client.put_object(Bucket=bucket, Key=prefix + "manifest.json", Body=manifest_bytes,
        ContentType="application/json", CacheControl="public, max-age=31536000, immutable")
    # Worker registration bypasses Service Worker caches, so these files also live at the public root.
    for filename in ["demo-release-worker.js", "ui-cache-addresses.js"]:
        client.put_object(Bucket=bucket, Key=filename, Body=(directory / filename).read_bytes(),
            ContentType="text/javascript; charset=utf-8", CacheControl="no-store")
    # Update the entry only after every asset has arrived. R2 has no directory index routing.
    destination = f"./{prefix}index.html"
    entry = f'''<!doctype html><html lang="ja"><head><meta charset="utf-8">
<meta http-equiv="refresh" content="0;url={destination}"><title>sshc · ブラウザデモ</title>
</head><body><a href="{destination}">sshcのブラウザデモを開く</a></body></html>'''
    client.put_object(Bucket=bucket, Key="index.html", Body=entry.encode(),
        ContentType="text/html; charset=utf-8", CacheControl="no-store")
    print(f"Published {len(objects)} files, {manifest['totalBytes']} bytes, release {options.release}")


if __name__ == "__main__":
    main()
