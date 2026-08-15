$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$gitStartInfo = [Diagnostics.ProcessStartInfo]::new()
$gitStartInfo.FileName = "git"
$gitStartInfo.UseShellExecute = $false
$gitStartInfo.RedirectStandardOutput = $true
$gitStartInfo.ArgumentList.Add("ls-files")
$gitStartInfo.ArgumentList.Add("-z")
$gitStartInfo.ArgumentList.Add("--")
$gitStartInfo.ArgumentList.Add("*.go")

$gitProcess = [Diagnostics.Process]::new()
$gitProcess.StartInfo = $gitStartInfo
if (-not $gitProcess.Start()) {
    throw "failed to start git"
}

$pathBuffer = [IO.MemoryStream]::new()
try {
    $gitProcess.StandardOutput.BaseStream.CopyTo($pathBuffer)
    $gitProcess.WaitForExit()
    $gitExit = $gitProcess.ExitCode
    $rawPathBytes = $pathBuffer.ToArray()
}
finally {
    $pathBuffer.Dispose()
    $gitProcess.Dispose()
}

if ($gitExit -ne 0) {
    exit $gitExit
}

$pathText = [Text.UTF8Encoding]::new($false, $true).GetString($rawPathBytes)
$goFiles = @($pathText.Split([char]0, [StringSplitOptions]::RemoveEmptyEntries))

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
