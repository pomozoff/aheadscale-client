$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$receipt=Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json'
$record=Get-Content $receipt -Raw | ConvertFrom-Json
if($record.publisher -ne 'CN=Aheadscale VPN Prototype' -or $record.packageFamilyName -notlike 'Aheadscale.VpnPrototype_*'){throw 'Неизвестный владелец установки'}
if($record.PSObject.Properties.Name -contains 'negativePackageFullName') {
 $negative=Get-AppxPackage -Name Aheadscale.VpnNegativeProbe
 if($negative) {
  if($negative.PackageFullName -ne $record.negativePackageFullName -or $negative.Publisher -ne $record.publisher){throw 'Отрицательный пакет не соответствует квитанции'}
  Remove-AppxPackage -Package $negative.PackageFullName
 }
}
$installed=Get-AppxPackage -Name Aheadscale.VpnPrototype
if($installed) {
 if($installed.PackageFamilyName -ne $record.packageFamilyName -or $installed.PackageFullName -ne $record.packageFullName){throw 'Установлен другой опытный пакет'}
 $controller=Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\aheadscale-vpn-prototype.exe'
 # Операторский процесс вне MSIX удаляет системный профиль. Это работает и
 # при откате ранних контроллеров, видевших только приватную телефонную книгу.
 $profile=Get-VpnConnection -Name 'Aheadscale Prototype' -ErrorAction SilentlyContinue
 if($profile) {
  if($profile.PlugInApplicationID -ne $record.packageFamilyName){throw 'Системный профиль принадлежит другому приложению'}
  if($profile.ConnectionStatus -ne 'Disconnected') {
   & "$env:WINDIR\System32\rasdial.exe" 'Aheadscale Prototype' /disconnect | Out-Null
   if($LASTEXITCODE){throw 'Не удалось отключить собственный системный профиль'}
  }
  Remove-VpnConnection -Name 'Aheadscale Prototype' -Force
 }
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
