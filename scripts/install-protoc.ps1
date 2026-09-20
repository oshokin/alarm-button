$ErrorActionPreference = "Stop"

if ($env:PROTOC_CURRENT_TAG -eq $env:PROTOC_VERSION) {
    exit 0
}

Write-Host "Installing protoc, version: $($env:PROTOC_VERSION)..."

Invoke-WebRequest -Uri $env:PROTOC_URL -OutFile $env:PROTOC_ZIP_FULL_PATH

Expand-Archive -Path $env:PROTOC_ZIP_FULL_PATH -DestinationPath $env:LOCAL_BIN -Force
Remove-Item -Force $env:PROTOC_ZIP_FULL_PATH

Move-Item -Force $env:PROTOC_BIN_IN_ZIP $env:PROTOC_BIN

if (Test-Path "$($env:LOCAL_BIN)\bin") {
    Remove-Item -Force -Recurse "$($env:LOCAL_BIN)\bin"
}
if (Test-Path "$($env:LOCAL_BIN)\readme.txt") {
    Remove-Item -Force "$($env:LOCAL_BIN)\readme.txt"
}
