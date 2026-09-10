# Application license catalogue

The Menu → Others → License page loads a bundled catalogue, including copyright notices and full license text. It needs no external service. Each row expands independently and the list can be searched.

Regenerate after changing dependencies, bundled fonts or icons:

```sh
npm ci --prefix web
go mod download
# Use the project's supported JDK for Gradle.
python3 scripts/licenses/generate.py
npm run build --prefix web
```

The generator requires Go, Python 3, Java and the Android Gradle wrapper. Gradle resolves the actual release runtime AAR/JAR artifacts; their POM license declarations (including parent inheritance) and embedded notices are read locally. Unknown Android license types stop generation for review. No application install or Android build is performed.

Go dependencies are collected from the Linux, macOS and Windows CLI and Android mobile package with CGO disabled; the Go runtime and gomobile runtime are also included. The local serial fork's license is used. Web entries follow non-development packages in the npm lockfile and include Monaco's ThirdPartyNotices. Fonts and Devicon use their bundled notices. System-provided libraries are outside this catalogue.

`catalogue.generated.json` and `sources.generated.json` are committed. A Web test checks the hashes of dependency manifests and bundled notices, so dependency changes require regeneration. Generation is explicit rather than part of every UI build, because it requires Go and Java/Gradle in addition to Node. Regenerate with the repository's pinned toolchains; do not edit generated texts by hand.
