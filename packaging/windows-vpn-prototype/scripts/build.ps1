param(
 [ValidateSet('arm64')][string]$Architecture='arm64',
 [string]$SdkVersion='10.0.26100.0',
 [ValidatePattern('^\d+\.\d+\.\d+\.\d+$')][string]$Version='0.1.0.0'
)
$ErrorActionPreference='Stop'
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
& cmake -S $root -B $build -G 'Visual Studio 17 2022' -A ARM64 -T v143 "-DCMAKE_SYSTEM_VERSION=$SdkVersion"
if($LASTEXITCODE){throw 'Ошибка CMake configure'}
& cmake --build $build --config Release --parallel 2
if($LASTEXITCODE){throw 'Ошибка компиляции C++/WinRT'}
Copy-Item (Join-Path $build 'Release\AheadscaleVpnPlugin.dll'),(Join-Path $build 'Release\AheadscaleVpnController.exe'),(Join-Path $build 'Release\AheadscaleVpnHost.exe') $stage
(Get-Content (Join-Path $root 'Package.appxmanifest') -Raw).Replace('@VERSION@',$Version) | Set-Content (Join-Path $stage 'AppxManifest.xml') -Encoding utf8
Add-Type -AssemblyName System.Drawing
$original=[Drawing.Image]::FromFile((Join-Path $repo 'packaging\aheadscale\icon.png'))
try {
 foreach($item in @(@('StoreLogo',50),@('Logo',150),@('SmallLogo',44))) {
  $bitmap=[Drawing.Bitmap]::new([int]$item[1],[int]$item[1]);$graphics=[Drawing.Graphics]::FromImage($bitmap)
  try {$graphics.DrawImage($original,0,0,$bitmap.Width,$bitmap.Height);$bitmap.Save((Join-Path $stage "Assets\$($item[0]).png"),[Drawing.Imaging.ImageFormat]::Png)}
  finally {$graphics.Dispose();$bitmap.Dispose()}
 }
} finally {$original.Dispose()}
$package=Join-Path $dist "Aheadscale.VpnPrototype_${Version}_arm64.msix"
& (Join-Path $tools 'makeappx.exe') pack /d $stage /p $package /o
if($LASTEXITCODE){throw 'Пакет MSIX не прошёл проверку MakeAppx'}
# Второй пакет с другой идентичностью — только для реальной отрицательной проверки.
$negativeStage=Join-Path $root 'out\negative-stage'
New-Item -ItemType Directory -Force $negativeStage | Out-Null
Copy-Item (Join-Path $stage 'Assets') $negativeStage -Recurse
Copy-Item (Join-Path $stage 'AheadscaleVpnController.exe') $negativeStage
(Get-Content (Join-Path $root 'NegativeProbe.appxmanifest') -Raw).Replace('@VERSION@',$Version) | Set-Content (Join-Path $negativeStage 'AppxManifest.xml') -Encoding utf8
$negativePackage=Join-Path $dist "Aheadscale.VpnNegativeProbe_${Version}_arm64.msix"
& (Join-Path $tools 'makeappx.exe') pack /d $negativeStage /p $negativePackage /o
if($LASTEXITCODE){throw 'Не собран отрицательный пакет'}
$cert=New-SelfSignedCertificate -Type CodeSigningCert -Subject 'CN=Aheadscale VPN Prototype' -CertStoreLocation Cert:\CurrentUser\My -KeyAlgorithm RSA -KeyLength 3072 -HashAlgorithm SHA256 -KeyExportPolicy NonExportable -NotAfter (Get-Date).AddDays(14)
try {
 & (Join-Path $tools 'signtool.exe') sign /fd SHA256 /sha1 $cert.Thumbprint $package
 if($LASTEXITCODE){throw 'Не удалось подписать опытный MSIX'}
 & (Join-Path $tools 'signtool.exe') sign /fd SHA256 /sha1 $cert.Thumbprint $negativePackage
 if($LASTEXITCODE){throw 'Не подписан отрицательный пакет'}
 Export-Certificate -Cert $cert -FilePath (Join-Path $dist 'Aheadscale.VpnPrototype.cer') | Out-Null
 $manifest=[ordered]@{name='Aheadscale.VpnPrototype';publisher=$cert.Subject;version=$Version;architecture=$Architecture;sdk=$SdkVersion;certificateThumbprint=$cert.Thumbprint;packageFile=(Split-Path $package -Leaf);sha256=(Get-FileHash $package -Algorithm SHA256).Hash;negativePackageFile=(Split-Path $negativePackage -Leaf);negativeSha256=(Get-FileHash $negativePackage -Algorithm SHA256).Hash;sourceCommit=(& git -C $repo rev-parse HEAD)}
 $manifest | ConvertTo-Json | Set-Content (Join-Path $dist 'build-manifest.json') -Encoding utf8
 Copy-Item (Join-Path $PSScriptRoot 'install-test.ps1'),(Join-Path $PSScriptRoot 'remove-test.ps1') $dist
 Get-ChildItem $dist | Select-Object Name,Length
} finally {Remove-Item "Cert:\CurrentUser\My\$($cert.Thumbprint)" -DeleteKey -ErrorAction Stop}
