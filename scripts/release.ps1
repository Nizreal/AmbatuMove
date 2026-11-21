[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Version,
    [switch]$Publish
)

$ErrorActionPreference = 'Stop'
function Get-FileSha256([string]$Path) {
    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    try {
        $stream = [System.IO.File]::OpenRead($Path)
        try { return [System.BitConverter]::ToString($algorithm.ComputeHash($stream)).Replace('-', '').ToLowerInvariant() }
        finally { $stream.Dispose() }
    } finally { $algorithm.Dispose() }
}
function Get-SourceManifest([string]$Root, [string]$ReleaseVersion, [string]$GoPath) {
    $files = Get-ChildItem -LiteralPath $Root -Recurse -File | Where-Object {
        ($_.Extension -eq '.go' -and $_.Name -notlike '*_test.go') -or
        $_.Extension -in @('.sql', '.json') -or $_.Name -in @('go.mod', 'go.sum')
    } | Sort-Object FullName
    $lines = foreach ($file in $files) {
        $relative = $file.FullName.Substring($Root.Length).TrimStart('\', '/').Replace('\', '/')
        "$(Get-FileSha256 $file.FullName)  $relative"
    }
    $algorithm = [System.Security.Cryptography.SHA256]::Create()
    try { $inputsHash = [System.BitConverter]::ToString($algorithm.ComputeHash([System.Text.Encoding]::UTF8.GetBytes(($lines -join "`n") + "`n"))).Replace('-', '').ToLowerInvariant() }
    finally { $algorithm.Dispose() }
    $goVersion = (& $GoPath version).Trim()
    return "version=$ReleaseVersion`ngo=$goVersion`ninputs_sha256=$inputsHash`n"
}
if ($Version -notmatch '^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$') {
    throw "Version must be an explicit semantic version such as v1.2.3; got '$Version'."
}

$repo = Split-Path -Parent $PSScriptRoot
$backend = Join-Path $repo 'backend'
$dist = Join-Path $repo 'dist'
$archiveName = "ambatumove-core-$Version-linux-arm64.tar.gz"
$archive = Join-Path $dist $archiveName
$checksum = "$archive.sha256"
$stage = Join-Path $dist "$Version-linux-arm64"
$reuseExisting = $false
$hasOutput = (Test-Path $archive) -or (Test-Path $checksum) -or (Test-Path $stage)
if ($hasOutput) {
    if (-not $Publish -or -not (Test-Path $archive) -or -not (Test-Path $checksum) -or -not (Test-Path (Join-Path $stage 'server')) -or -not (Test-Path (Join-Path $stage 'release.manifest'))) {
        throw "Release output already exists or is incomplete for $Version; existing artifacts are preserved. Use -Publish only to verify and publish a complete existing build, or choose a new version."
    }
    $reuseExisting = $true
}
$go = Get-Command go -ErrorAction SilentlyContinue
if (-not $go) {
    $bundledGo = Join-Path $repo '.tools\go\go\bin\go.exe'
    if (Test-Path $bundledGo) { $goPath = $bundledGo } else { throw 'Go is required (go.exe was not found on PATH or in .tools/go/go/bin).' }
} else { $goPath = $go.Source }
$tar = Get-Command tar -ErrorAction SilentlyContinue
if (-not $tar) { throw 'tar is required to create the .tar.gz release archive.' }
if ($Publish) {
    $gh = Get-Command gh -ErrorAction SilentlyContinue
    if (-not $gh) { throw 'GitHub CLI (gh) is required when -Publish is specified.' }
    & $gh.Source auth status
    if ($LASTEXITCODE -ne 0) { throw 'GitHub CLI is not authenticated; run gh auth login first.' }
}

Push-Location $backend
try {
    & $goPath fmt ./...
    if ($LASTEXITCODE -ne 0) { throw 'go fmt failed.' }
    & $goPath vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'go vet failed.' }
    & $goPath test ./...
    if ($LASTEXITCODE -ne 0) { throw 'go test failed.' }

    $env:CGO_ENABLED = '0'; $env:GOOS = 'linux'; $env:GOARCH = 'arm64'
    New-Item -ItemType Directory -Force -Path $dist | Out-Null
    $verifyStage = $null
    if ($reuseExisting) {
        $verifyStage = Join-Path $dist ".${Version}.verify-$PID"
        if (Test-Path $verifyStage) { throw "Refusing to overwrite verification path $verifyStage." }
        New-Item -ItemType Directory -Path $verifyStage | Out-Null
        $verifyBinary = Join-Path $verifyStage 'server-compiled'
        & $goPath build -trimpath -ldflags "-s -w -X main.version=$Version" -o $verifyBinary ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'Linux ARM64 cross-build failed.' }
        $currentManifest = Get-SourceManifest $backend $Version $goPath
        $existingManifest = Get-Content -LiteralPath (Join-Path $stage 'release.manifest') -Raw
        if ($existingManifest -ne $currentManifest) { throw 'Existing release was built from different Go sources or a different Go toolchain; choose a new version.' }
        $stagedHash = Get-FileSha256 (Join-Path $stage 'server')
        $entries = @(& $tar.Source -tzf $archive)
        if ($LASTEXITCODE -ne 0 -or $entries.Count -ne 1 -or $entries[0] -ne 'server') { throw 'Existing release archive must contain exactly the server executable.' }
        $checksumLine = (Get-Content -LiteralPath $checksum -Raw).Trim()
        if ($checksumLine -notmatch ('^([a-fA-F0-9]{64})\s+' + [regex]::Escape($archiveName) + '$')) { throw 'Existing release checksum file is malformed.' }
        $expectedHash = $Matches[1].ToLowerInvariant()
        $sha256 = [System.Security.Cryptography.SHA256]::Create()
        try {
            $stream = [System.IO.File]::OpenRead($archive)
            try { $archiveHash = [System.BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '').ToLowerInvariant() }
            finally { $stream.Dispose() }
        } finally { $sha256.Dispose() }
        if ($archiveHash -ne $expectedHash) { throw 'Existing release archive does not match its SHA-256 checksum.' }
        $extractedBinary = Join-Path $verifyStage 'server'
        & $tar.Source -xzf $archive -C $verifyStage
        if ($LASTEXITCODE -ne 0) { throw 'Could not extract the existing release binary for verification.' }
        $archivedHash = Get-FileSha256 $extractedBinary
        if ($archivedHash -ne $stagedHash) { throw 'Existing release archive does not contain the verified release binary.' }
    } else {
        New-Item -ItemType Directory -Path $stage | Out-Null
        $binary = Join-Path $stage 'server'
        & $goPath build -trimpath -ldflags "-s -w -X main.version=$Version" -o $binary ./cmd/server
        if ($LASTEXITCODE -ne 0) { throw 'Linux ARM64 cross-build failed.' }
        $manifest = Get-SourceManifest $backend $Version $goPath
        [System.IO.File]::WriteAllText((Join-Path $stage 'release.manifest'), $manifest, [System.Text.UTF8Encoding]::new($false))

        & $tar.Source -czf $archive -C $stage server
        if ($LASTEXITCODE -ne 0 -or -not (Test-Path $archive)) { throw 'Release archive creation failed.' }
        $entries = @(& $tar.Source -tzf $archive)
        if ($LASTEXITCODE -ne 0 -or $entries.Count -ne 1 -or $entries[0] -ne 'server') { throw 'Release archive must contain exactly the server executable.' }
        $sha256 = [System.Security.Cryptography.SHA256]::Create()
        try {
            $stream = [System.IO.File]::OpenRead($archive)
            try { $digest = [System.BitConverter]::ToString($sha256.ComputeHash($stream)).Replace('-', '').ToLowerInvariant() }
            finally { $stream.Dispose() }
        } finally { $sha256.Dispose() }
        [System.IO.File]::WriteAllText($checksum, "$digest  $archiveName`n", [System.Text.UTF8Encoding]::new($false))
        if ($digest -notmatch '^[a-f0-9]{64}$') { throw 'Generated release SHA-256 is malformed.' }
    }

    if ($reuseExisting) { Write-Host "Verified existing release $archive" } else { Write-Host "Built $archive" }
    if (-not $reuseExisting) { Write-Host "SHA-256 $digest" }
    if ($Publish) {
        $remote = (& git -C $repo remote get-url origin 2>$null)
        if ($LASTEXITCODE -ne 0 -or -not $remote -or $remote -notmatch 'github\.com[:/]([^/]+/[^/]+?)(?:\.git)?$') { throw 'Could not determine OWNER/REPOSITORY from the GitHub origin.' }
        $repoName = $Matches[1]
        & $gh.Source release create $Version --repo $repoName --verify-tag --title "AmbatuMove $Version" --generate-notes $archive $checksum
        if ($LASTEXITCODE -ne 0) { throw 'GitHub Release publishing failed.' }
    }
} finally {
    if ($verifyStage -and (Test-Path $verifyStage)) { Remove-Item -LiteralPath $verifyStage -Recurse -Force }
    Pop-Location
}
