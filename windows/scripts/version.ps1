function Get-MasqueVersion {
    $path = Join-Path $PSScriptRoot "..\internal\version\version.go"
    $hit = Select-String -Path $path -Pattern 'const Version = "([^"]+)"'
    if (-not $hit -or $hit.Matches.Count -eq 0) {
        throw "could not read Version from $path"
    }
    return $hit.Matches[0].Groups[1].Value
}
