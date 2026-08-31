<#
.SYNOPSIS
    Adds a "Find subtitles (MoanDrop)" entry to the right-click menu for
    video files, for the current user only (no admin rights needed).

.PARAMETER ExePath
    Path to moandrop.exe. Defaults to moandrop.exe next to this script.

.EXAMPLE
    .\install-context-menu.ps1
    .\install-context-menu.ps1 -ExePath 'C:\Tools\MoanDrop\moandrop.exe'
#>
param(
    [string]$ExePath = (Join-Path $PSScriptRoot 'moandrop.exe')
)

$ErrorActionPreference = 'Stop'

if (-not (Test-Path -LiteralPath $ExePath)) {
    Write-Error "moandrop.exe not found at '$ExePath'. Pass -ExePath <path to moandrop.exe>."
    exit 1
}
$ExePath = (Resolve-Path -LiteralPath $ExePath).Path

$extensions = '.mp4', '.m4v', '.mkv', '.avi', '.wmv', '.flv', '.mov', '.mpg', '.mpeg'
$menuText = 'Find subtitles (MoanDrop)'
$commandLine = '"{0}" "%1"' -f $ExePath

foreach ($ext in $extensions) {
    $shellKey = "HKCU:\Software\Classes\SystemFileAssociations\$ext\shell\MoanDrop"
    $commandKey = "$shellKey\command"

    New-Item -Path $shellKey -Force | Out-Null
    Set-Item -Path $shellKey -Value $menuText

    New-Item -Path $commandKey -Force | Out-Null
    Set-Item -Path $commandKey -Value $commandLine

    Write-Host "Registered $ext"
}

Write-Host ''
Write-Host 'Done. Run uninstall-context-menu.ps1 to remove.'
Write-Host "On Windows 11 this shows under Show more options in the context menu -" `
    "the modern (top-level) menu needs an MSIX-packaged app, which is out of scope here."
