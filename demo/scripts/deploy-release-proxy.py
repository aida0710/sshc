"""Deploy the read-only GitHub proxy using a separately supplied Cloudflare credential."""
import json
import os
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen
import uuid


def cloudflare_request(credentials, path, *, method="GET", body=None, content_type="application/json"):
    account = credentials["accountID"]
    request = Request(f"https://api.cloudflare.com/client/v4/accounts/{account}/{path}",
        data=body, method=method, headers={"Authorization": "Bearer " + credentials["apiToken"],
                                         "Content-Type": content_type})
    try:
        with urlopen(request, timeout=60) as response:
            payload = json.load(response)
    except HTTPError as error:
        payload = json.load(error)
    if not payload.get("success"):
        raise RuntimeError("Cloudflare API rejected the request: " + json.dumps(payload.get("errors", [])))
    return payload["result"]


def main():
    credentials = json.loads(Path(os.environ["SSHC_CF_CREDENTIALS_FILE"]).read_text())
    directory = Path(__file__).resolve().parents[1] / "release-proxy"
    configuration = json.loads((directory / "wrangler.jsonc").read_text())
    cloudflare_request(credentials, "tokens/verify")
    subdomain = cloudflare_request(credentials, "workers/subdomain")["subdomain"]
    metadata = json.dumps({"main_module": "worker.js", "compatibility_date": configuration["compatibility_date"]}).encode()
    boundary = "sshc-demo-" + uuid.uuid4().hex
    parts = []
    for name, filename, content_type, body in [
        ("metadata", "metadata.json", "application/json", metadata),
        ("worker.js", "worker.js", "application/javascript+module", (directory / "worker.js").read_bytes()),
    ]:
        parts.append((f"--{boundary}\r\nContent-Disposition: form-data; name=\"{name}\"; filename=\"{filename}\"\r\n"
                      f"Content-Type: {content_type}\r\n\r\n").encode() + body + b"\r\n")
    parts.append(f"--{boundary}--\r\n".encode())
    script_path = "workers/scripts/" + configuration["name"]
    cloudflare_request(credentials, script_path, method="PUT", body=b"".join(parts),
                       content_type="multipart/form-data; boundary=" + boundary)
    cloudflare_request(credentials, script_path + "/subdomain", method="POST",
                       body=json.dumps({"enabled": True, "previews_enabled": False}).encode())
    print(f"https://{configuration['name']}.{subdomain}.workers.dev/")


if __name__ == "__main__":
    main()
