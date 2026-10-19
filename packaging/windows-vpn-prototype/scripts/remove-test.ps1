$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$receipt=Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json'
$record=Get-Content $receipt -Raw | ConvertFrom-Json
if($record.publisher -ne 'CN=Aheadscale VPN Prototype' -or $record.packageFamilyName -notlike 'Aheadscale.VpnPrototype_*'){throw 'Неизвестный владелец установки'}
$installed=Get-AppxPackage -Name Aheadscale.VpnPrototype
if($installed) {
 if($installed.PackageFamilyName -ne $record.packageFamilyName -or $installed.PackageFullName -ne $record.packageFullName){throw 'Установлен другой опытный пакет'}
 $controller=Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\aheadscale-vpn-prototype.exe'
 & $controller disconnect
 & $controller remove
 if($LASTEXITCODE){throw 'Не удалось удалить принадлежащий пакету профиль; пакет оставлен для диагностики'}
 Remove-AppxPackage -Package $installed.PackageFullName
}
if($record.trustAdded) {
 $path="Cert:\LocalMachine\TrustedPeople\$($record.certificateThumbprint)"
 if(Test-Path $path) {
  if((Get-Item $path).Subject -ne $record.publisher){throw 'Сертификат не принадлежит прототипу'}
  Remove-Item $path
 }
}
Remove-Item $receipt
Write-Output 'Опытные пакет, профиль и добавленное ими доверие удалены; обычный Aheadscale сохранён.'
