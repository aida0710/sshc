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

# Windowsのコマンドラインは32,767文字までなので、全パスを1回のgofmtへ渡すとプロセスを
# 起動できない。引用符と区切りの空白の分を見込んで、上限の半分の文字数ごとに分けて渡す。
$maxBatchCharacters = 16000

$batches = [Collections.Generic.List[string[]]]::new()
$batch = [Collections.Generic.List[string]]::new()
$batchCharacters = 0
foreach ($path in $goFiles) {
    # 引用符2つと区切りの空白1つの分を足す。
    $pathCharacters = $path.Length + 3
    if ($batch.Count -gt 0 -and $batchCharacters + $pathCharacters -gt $maxBatchCharacters) {
        $batches.Add($batch.ToArray())
        $batch.Clear()
        $batchCharacters = 0
    }
    $batch.Add($path)
    $batchCharacters += $pathCharacters
}
if ($batch.Count -gt 0) {
    $batches.Add($batch.ToArray())
}

$unformatted = [Collections.Generic.List[string]]::new()
foreach ($batchPaths in $batches) {
    $batchUnformatted = @(& gofmt -l -- @batchPaths)
    $gofmtExit = $LASTEXITCODE
    if ($gofmtExit -ne 0) {
        exit $gofmtExit
    }
    foreach ($path in $batchUnformatted) {
        $unformatted.Add([string]$path)
    }
}

if ($unformatted.Count -gt 0) {
    [Console]::Error.WriteLine("These files are not gofmt-formatted. Run: gofmt -w <path>.")
    foreach ($path in $unformatted) {
        [Console]::Error.WriteLine([string]$path)
    }
    exit 1
}
