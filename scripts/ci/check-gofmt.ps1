$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$goFiles = @(& git ls-files -- "*.go")
$gitExit = $LASTEXITCODE
if ($gitExit -ne 0) {
    exit $gitExit
}

$unformatted = @()
if ($goFiles.Count -gt 0) {
    $unformatted = @(& gofmt -l -- @goFiles)
    $gofmtExit = $LASTEXITCODE
    if ($gofmtExit -ne 0) {
        exit $gofmtExit
    }
}

if ($unformatted.Count -gt 0) {
    [Console]::Error.WriteLine("These files are not gofmt-formatted. Run: gofmt -w <path>.")
    foreach ($path in $unformatted) {
        [Console]::Error.WriteLine([string]$path)
    }
    exit 1
}
