<#
.SYNOPSIS
    Removes the "Find subtitles (MoanDrop)" right-click entry added by
    install-context-menu.ps1.
#>
$ErrorActionPreference = 'Stop'

$extensions = '.mp4', '.m4v', '.mkv', '.avi', '.wmv', '.flv', '.mov', '.mpg', '.mpeg'

foreach ($ext in $extensions) {
    $shellKey = "HKCU:\Software\Classes\SystemFileAssociations\$ext\shell\MoanDrop"
    if (Test-Path -LiteralPath $shellKey) {
        Remove-Item -Path $shellKey -Recurse -Force
        Write-Host "Removed $ext"
    }
}

Write-Host ''
Write-Host 'Done.'
