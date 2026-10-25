param(
 [ValidateSet('arm64','amd64')][string]$Architecture='arm64',
 [string]$SdkVersion='10.0.26100.0',
 [ValidatePattern('^\d+\.\d+\.\d+\.\d+$')][string]$Version='0.1.0.0'
)
$ErrorActionPreference='Stop'
$packageArchitecture=if($Architecture -eq 'amd64'){'x64'}else{'arm64'}
Set-StrictMode -Version Latest
$root=Split-Path $PSScriptRoot -Parent
$repo=Resolve-Path (Join-Path $root '..\..')
$sdk=Join-Path ${env:ProgramFiles(x86)} 'Windows Kits\10'
$tools=Join-Path $sdk "bin\$SdkVersion\x64"
foreach($name in @('makeappx.exe','signtool.exe')) {
 if(-not(Test-Path (Join-Path $tools $name))){throw "Не найден $name для SDK $SdkVersion"}
}
if(-not(Test-Path (Join-Path $sdk "Include\$SdkVersion\cppwinrt\winrt\Windows.Networking.Vpn.h"))){throw 'Нет заголовков C++/WinRT выбранного SDK'}
$build=Join-Path $root 'out\build'
$stage=Join-Path $root 'out\stage'
$dist=Join-Path $root 'out\dist'
New-Item -ItemType Directory -Force $stage,$dist,(Join-Path $stage 'Assets') | Out-Null
& cmake -S $root -B $build -G 'Visual Studio 17 2022' -A $(if($Architecture -eq 'amd64'){'x64'}else{'ARM64'}) -T v143 "-DCMAKE_SYSTEM_VERSION=$SdkVersion"
if($LASTEXITCODE){throw 'Ошибка CMake configure'}
& cmake --build $build --config Release --parallel 2
if($LASTEXITCODE){throw 'Ошибка компиляции C++/WinRT'}
Copy-Item (Join-Path $build 'Release\AheadscaleVpnPlugin.dll'),(Join-Path $build 'Release\AheadscaleVpnController.exe'),(Join-Path $build 'Release\AheadscaleVpnHost.exe'),(Join-Path $build 'Release\AheadscaleVpnBroker.exe') $stage
# Отдельный опытный daemon; штатный Wintun-клиент не заменяется сборкой.
$env:GOTOOLCHAIN='go1.26.7';$env:GOOS='windows';$env:GOARCH=$Architecture;$env:CGO_ENABLED='0'
$goVersion=& go -C $repo version
if($LASTEXITCODE -or $goVersion -notmatch '^go version go1\.26\.7 windows/amd64$'){throw 'Нужен Go 1.26.7 для Windows build runner'}
# Выполняем регрессии UI/контроллера на архитектуре Windows runner до cross-build.
$env:GOARCH='amd64'
& go -C $repo test -tags 'devcert,windowsvpnprototype' ./client/internal/windowsvpn ./client/ui/services
if($LASTEXITCODE){throw 'Регрессии восстановления native VPN не прошли'}
$env:GOARCH=$Architecture
$flags='-s -w -X github.com/netbirdio/netbird/version.version=0.80.0-aheadscale.15 -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultManagementURL=https://aheadscale.ru -X github.com/netbirdio/netbird/client/internal/profilemanager.DefaultAdminURL=https://aheadscale.ru -X github.com/netbirdio/netbird/client/internal/profilemanager.LockedServerOrigin=https://aheadscale.ru'
& go -C $repo build -trimpath -tags windowsvpnprototype -ldflags $flags -o (Join-Path $stage 'AheadscaleNativeDaemon.exe') ./client
if($LASTEXITCODE){throw 'Не собран опытный Windows daemon'}
$standardCheck=Join-Path $root 'out\standard-check.exe'
& go -C $repo build -trimpath -o $standardCheck ./client
if($LASTEXITCODE){throw 'Нарушена компиляция обычного Windows daemon'}
Remove-Item $standardCheck

# Отдельный GUI для проверки системного профиля; установленная .8 не заменяется.
$ui=Join-Path $dist 'AheadscaleNativeUI.exe'
$env:GOARCH='amd64'
$env:GOBIN=Join-Path $root 'out\tools'
New-Item -ItemType Directory -Force $env:GOBIN|Out-Null
& go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.3
if($LASTEXITCODE){throw 'Не установлен зафиксированный генератор Wails'}
$wails=Join-Path $env:GOBIN 'wails3.exe'
Push-Location (Join-Path $repo 'client\ui')
try { & $wails generate bindings -clean=true -ts } finally { Pop-Location }
if($LASTEXITCODE){throw 'Не созданы bindings опытного GUI'}
$env:VITE_AHEADSCALE_MANAGEMENT_URL='https://aheadscale.ru'
& pnpm --dir (Join-Path $repo 'client\ui\frontend') install --frozen-lockfile
if($LASTEXITCODE){throw 'Не установлены зафиксированные зависимости GUI'}
& pnpm --dir (Join-Path $repo 'client\ui\frontend') build
if($LASTEXITCODE){throw 'Не собран frontend опытного GUI'}
$syso=Join-Path $repo "client\ui\wails_windows_${Architecture}.syso"
& $wails generate syso -arch $Architecture -icon (Join-Path $repo 'packaging\aheadscale\icon.ico') -manifest (Join-Path $repo 'client\ui\build\windows\wails.exe.manifest') -info (Join-Path $repo 'client\ui\build\windows\info.json') -out $syso
if($LASTEXITCODE){throw 'Не созданы ресурсы Windows GUI'}
$env:GOARCH=$Architecture
& go -C $repo build -trimpath -tags 'production,windowsvpnprototype' -ldflags ($flags+' -H windowsgui') -o $ui ./client/ui
if($LASTEXITCODE){throw 'Не собран опытный Windows GUI'}
Remove-Item $syso

(Get-Content (Join-Path $root 'Package.appxmanifest') -Raw).Replace('@VERSION@',$Version).Replace('@ARCHITECTURE@',$packageArchitecture) | Set-Content (Join-Path $stage 'AppxManifest.xml') -Encoding utf8
Add-Type -AssemblyName System.Drawing
$original=[Drawing.Image]::FromFile((Join-Path $repo 'packaging\aheadscale\icon.png'))
try {
 foreach($item in @(@('StoreLogo',50),@('Logo',150),@('SmallLogo',44))) {
  $bitmap=[Drawing.Bitmap]::new([int]$item[1],[int]$item[1]);$graphics=[Drawing.Graphics]::FromImage($bitmap)
  try {$graphics.DrawImage($original,0,0,$bitmap.Width,$bitmap.Height);$bitmap.Save((Join-Path $stage "Assets\$($item[0]).png"),[Drawing.Imaging.ImageFormat]::Png)}
  finally {$graphics.Dispose();$bitmap.Dispose()}
 }
} finally {$original.Dispose()}
$package=Join-Path $dist "Aheadscale.VpnPrototype_${Version}_${Architecture}.msix"
& (Join-Path $tools 'makeappx.exe') pack /d $stage /p $package /o
if($LASTEXITCODE){throw 'Пакет MSIX не прошёл проверку MakeAppx'}
# Второй пакет с другой идентичностью — только для реальной отрицательной проверки.
$negativeStage=Join-Path $root 'out\negative-stage'
New-Item -ItemType Directory -Force $negativeStage | Out-Null
Copy-Item (Join-Path $stage 'Assets') $negativeStage -Recurse
Copy-Item (Join-Path $stage 'AheadscaleVpnController.exe') $negativeStage
(Get-Content (Join-Path $root 'NegativeProbe.appxmanifest') -Raw).Replace('@VERSION@',$Version).Replace('@ARCHITECTURE@',$packageArchitecture) | Set-Content (Join-Path $negativeStage 'AppxManifest.xml') -Encoding utf8
$negativePackage=Join-Path $dist "Aheadscale.VpnNegativeProbe_${Version}_${Architecture}.msix"
& (Join-Path $tools 'makeappx.exe') pack /d $negativeStage /p $negativePackage /o
if($LASTEXITCODE){throw 'Не собран отрицательный пакет'}
$cert=New-SelfSignedCertificate -Type CodeSigningCert -Subject 'CN=Aheadscale VPN Prototype' -CertStoreLocation Cert:\CurrentUser\My -KeyAlgorithm RSA -KeyLength 3072 -HashAlgorithm SHA256 -KeyExportPolicy NonExportable -NotAfter (Get-Date).AddDays(14)
try {
 & (Join-Path $tools 'signtool.exe') sign /fd SHA256 /sha1 $cert.Thumbprint $package
 if($LASTEXITCODE){throw 'Не удалось подписать опытный MSIX'}
 & (Join-Path $tools 'signtool.exe') sign /fd SHA256 /sha1 $cert.Thumbprint $negativePackage
 if($LASTEXITCODE){throw 'Не подписан отрицательный пакет'}
 & (Join-Path $tools 'signtool.exe') sign /fd SHA256 /sha1 $cert.Thumbprint $ui
 if($LASTEXITCODE){throw 'Не подписан опытный GUI'}
 Export-Certificate -Cert $cert -FilePath (Join-Path $dist 'Aheadscale.VpnPrototype.cer') | Out-Null
 $manifest=[ordered]@{name='Aheadscale.VpnPrototype';publisher=$cert.Subject;version=$Version;architecture=$Architecture;clientVersion='0.80.0-aheadscale.15';nativeUIFile=(Split-Path $ui -Leaf);nativeUISha256=(Get-FileHash $ui -Algorithm SHA256).Hash;sdk=$SdkVersion;go='1.26.7';nativeDaemonSha256=(Get-FileHash (Join-Path $stage 'AheadscaleNativeDaemon.exe') -Algorithm SHA256).Hash;certificateThumbprint=$cert.Thumbprint;packageFile=(Split-Path $package -Leaf);sha256=(Get-FileHash $package -Algorithm SHA256).Hash;negativePackageFile=(Split-Path $negativePackage -Leaf);negativeSha256=(Get-FileHash $negativePackage -Algorithm SHA256).Hash;sourceCommit=(& git -C $repo rev-parse HEAD)}
 $manifest | ConvertTo-Json | Set-Content (Join-Path $dist 'build-manifest.json') -Encoding utf8
 Copy-Item (Join-Path $PSScriptRoot 'install-test.ps1'),(Join-Path $PSScriptRoot 'remove-test.ps1') $dist
 Get-ChildItem $dist | Select-Object Name,Length
} finally {Remove-Item "Cert:\CurrentUser\My\$($cert.Thumbprint)" -DeleteKey -ErrorAction Stop}
