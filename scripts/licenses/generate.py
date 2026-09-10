#!/usr/bin/env python3
"""Collect installed distribution notices. Run after npm ci and Gradle dependency resolution."""
import concurrent.futures
import io
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import xml.etree.ElementTree as ET
import zipfile

ROOT = Path(__file__).resolve().parents[2]
os.chdir(ROOT)
NOTICE = re.compile(r"^(license|licence|copying|copyright|notice|thirdpartynotices|patents)([.\-_]|$)", re.I)
entries = []


def run(*args, **kwargs):
    return subprocess.check_output(args, text=True, **kwargs)


def documents(directory):
    return [{"name": path.name, "text": path.read_text(encoding="utf-8")} for path in sorted(directory.iterdir()) if path.is_file() and NOTICE.match(path.name)]


def label(notices):
    text = "\n".join(n["text"] for n in notices)
    if "Apache License" in text and "Version 2.0" in text: return "Apache-2.0"
    if "Permission is hereby granted, free of charge" in text: return "MIT"
    if "Redistribution and use in source and binary forms" in text: return "BSD"
    if "ISC" in text or "Permission to use, copy, modify, and/or distribute" in text: return "ISC"
    return "License"


def add(name, version, category, url, notices, license_name=None):
    if not notices or any(not n["text"].strip() for n in notices):
        raise RuntimeError(f"Missing license text: {name}")
    entries.append(dict(name=name, version=version, category=category, url=url, license=license_name or label(notices), notices=notices))


add("sshc", "", "Application", "https://github.com/aida0710/sshc", documents(ROOT))
goroot = Path(run("go", "env", "GOROOT").strip())
add("Go", run("go", "env", "GOVERSION").strip(), "Runtime", "https://go.dev", documents(goroot), "BSD-3-Clause")


def go_dependencies(platform):
    env = dict(os.environ, GOOS=platform, GOARCH="arm64" if platform == "android" else "amd64", CGO_ENABLED="0")
    targets = ["./mobile"] if platform == "android" else ["./cmd/sshc"]
    return run("go", "list", "-deps", "-f", "{{with .Module}}{{.Path}}{{end}}", *targets, env=env).splitlines()


with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
    modules = set(sum(pool.map(go_dependencies, ["linux", "darwin", "windows", "android"]), [])) - {"", "sshc"}
# gomobile also supplies the Android Java/Go binding runtime.
modules.add("golang.org/x/mobile")
for name in sorted(modules):
    module = json.loads(run("go", "list", "-m", "-json", name))
    directory = Path(module.get("Replace", module)["Dir"])
    add(name, module["Version"], "Go", "https://pkg.go.dev/" + name + "@" + module["Version"], documents(directory))

lock = json.loads((ROOT / "web/package-lock.json").read_text())
for relative, package in sorted(lock["packages"].items()):
    if not relative or package.get("dev"): continue
    directory = ROOT / "web" / relative
    metadata = json.loads((directory / "package.json").read_text())
    add(metadata["name"], package["version"], "Web", "https://www.npmjs.com/package/" + metadata["name"], documents(directory), metadata.get("license"))

add("JetBrains Mono", "", "Font", "https://github.com/JetBrains/JetBrainsMono", [{"name": "OFL.txt", "text": (ROOT / "web/public/fonts/OFL.txt").read_text()}], "OFL-1.1")
add("Devicon", "7330accdbc47", "Icons", "https://github.com/devicons/devicon", documents(ROOT / "web/src/ui/os-icons"), "MIT")

# Use Gradle's selected runtime versions, not all versions present in its cache.
report = run(str(ROOT / "android/gradlew"), "-p", "android", "-I", str(ROOT / "scripts/licenses/artifacts.gradle"), ":app:licenseArtifacts", "--console", "plain")
artifacts = {}
for line in report.splitlines():
    if line.startswith("SSHC_LICENSE_ARTIFACT\t"):
        _, group, artifact, version, filename = line.split("\t")
        artifacts.setdefault((group, artifact, version), []).append(Path(filename))
if not artifacts: raise RuntimeError("No Android runtime artifacts resolved")
coordinates = set(artifacts)
cache = Path(os.environ.get("GRADLE_USER_HOME", str(Path.home() / ".gradle"))) / "caches/modules-2/files-2.1"
ns = {"m": "http://maven.apache.org/POM/4.0.0"}
def pom_licenses(pom):
    names = [n.text for n in pom.findall("m:licenses/m:license/m:name", ns)]
    if names: return names
    parent = pom.find("m:parent", ns)
    if parent is None: return []
    parts = [parent.findtext("m:" + name, namespaces=ns) for name in ["groupId", "artifactId", "version"]]
    parent_file = next((cache / parts[0] / parts[1] / parts[2]).glob("*/*.pom"))
    return pom_licenses(ET.parse(parent_file).getroot())

for group, artifact, version in sorted(coordinates):
    directory = cache / group / artifact / version
    pom = ET.parse(next(directory.glob("*/*.pom"))).getroot()
    names = pom_licenses(pom)
    if not names or any("Apache" not in name for name in names):
        raise RuntimeError(f"Review Android license: {group}:{artifact}: {names}")
    notices = [{"name": "LICENSE", "text": (ROOT / "LICENSE").read_text()}]
    seen = set()
    def collect_archive(raw):
        with zipfile.ZipFile(raw) as archive:
            for filename in sorted(archive.namelist()):
                if filename.endswith("classes.jar"):
                    collect_archive(io.BytesIO(archive.read(filename)))
                elif NOTICE.match(Path(filename).name) and not filename.endswith("/"):
                    text = archive.read(filename).decode("utf-8")
                    if text not in seen:
                        seen.add(text)
                        notices.append({"name": filename, "text": text})
    for archive in sorted(artifacts[(group, artifact, version)]):
        if archive.suffix in (".jar", ".aar") and "sources" not in archive.name: collect_archive(archive)
    add(group + ":" + artifact, version, "Android", pom.findtext("m:url", namespaces=ns) or "https://developer.android.com/jetpack/androidx", notices, "Apache-2.0")

entries.sort(key=lambda e: (e["name"] != "sshc", e["name"].lower()))
output = ROOT / "web/src/licenses/catalogue.generated.json"
output.write_text(json.dumps(entries, ensure_ascii=False, indent=2) + "\n")
print(f"Collected {len(entries)} components into {output.relative_to(ROOT)}")

inputs = ["go.mod", "go.sum", "web/package-lock.json", "android/app/build.gradle.kts", "LICENSE", "web/public/fonts/OFL.txt", "web/src/ui/os-icons/LICENSE", "web/src/ui/os-icons/README.md", "third_party/go-serial/LICENSE", "scripts/licenses/generate.py", "scripts/licenses/artifacts.gradle"]
(ROOT / "web/src/licenses/sources.generated.json").write_text(json.dumps({name: hashlib.sha256((ROOT / name).read_bytes()).hexdigest() for name in inputs}, indent=2) + "\n")
