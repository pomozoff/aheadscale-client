$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$receipt=Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json'
$record=Get-Content $receipt -Raw | ConvertFrom-Json
if($record.publisher -ne 'CN=Aheadscale VPN Prototype' -or $record.packageFamilyName -notlike 'Aheadscale.VpnPrototype_*'){throw 'Неизвестный владелец установки'}
function Get-OwnedFirewallRule([object]$record) {
 $expectedName='Aheadscale.VpnPrototype-WireGuard-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
 if($record.firewallRuleName -ne $expectedName -or $record.packageFullName -notmatch '^Aheadscale\.VpnPrototype_[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+_(arm64|x64)__9kyyx27wet2q6$'){throw 'Неизвестный владелец правила VPN'}
 $expectedProgram=Join-Path (Join-Path (Join-Path $env:ProgramFiles 'WindowsApps') $record.packageFullName) 'AheadscaleNativeDaemon.exe'
 if($record.nativeDaemonPath -ine $expectedProgram){throw 'Правило не относится к опытной службе'}
 $ownedRules=@(Get-NetFirewallRule -Name $expectedName -PolicyStore PersistentStore -ErrorAction SilentlyContinue)
 if($ownedRules.Count -eq 0){return $null}
 if($ownedRules.Count -ne 1){throw 'Имя правила неоднозначно; не удаляем'}
 $rule=$ownedRules[0];$app=$rule|Get-NetFirewallApplicationFilter;$port=$rule|Get-NetFirewallPortFilter;$address=$rule|Get-NetFirewallAddressFilter
 if($rule.Group -ne 'Aheadscale.VpnPrototype' -or $app.Program -ine $expectedProgram -or [string]$rule.Direction -ne 'Inbound' -or [string]$rule.Action -ne 'Allow' -or [string]$rule.Enabled -ne 'True' -or [string]$rule.Profile -ne 'Any' -or [string]$port.Protocol -notin @('UDP','17') -or @($port.LocalPort).Count -ne 1 -or [string]$port.LocalPort -ne '51830' -or @($port.RemotePort).Count -ne 1 -or [string]$port.RemotePort -ne 'Any' -or @($address.RemoteAddress).Count -ne 1 -or [string]$address.RemoteAddress -ne 'Any' -or @($address.LocalAddress).Count -ne 1 -or [string]$address.LocalAddress -ne 'Any'){throw 'Правило изменено вне установки; не удаляем'}
 return $rule
}
$hasFirewallRule=$record.PSObject.Properties.Name -contains 'firewallRuleName'
# Проверяем до удаления профиля/пакета, чтобы отказ не оставлял частичное удаление.
if($hasFirewallRule){$null=Get-OwnedFirewallRule $record}
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
 $profiles=@(foreach($name in @('Aheadscale','Aheadscale Prototype')){Get-VpnConnection -Name $name -ErrorAction SilentlyContinue})
 foreach($profile in $profiles){if($profile.PlugInApplicationID -ne $record.packageFamilyName){throw 'Системный профиль принадлежит другому приложению'}}
 foreach($profile in $profiles) {
  if($profile.ConnectionStatus -ne 'Disconnected') {
   & "$env:WINDIR\System32\rasdial.exe" $profile.Name /disconnect | Out-Null
   if($LASTEXITCODE){throw 'Не удалось отключить собственный системный профиль'}
  }
  Remove-VpnConnection -Name $profile.Name -Force
 }
 & $controller remove
 if($LASTEXITCODE){throw 'Не удалось удалить принадлежащий пакету профиль; пакет оставлен для диагностики'}
 Remove-AppxPackage -Package $installed.PackageFullName
}
if($hasFirewallRule){
 # Повторная проверка перед удалением не принимает изменённое за это время правило.
 $rule=Get-OwnedFirewallRule $record
 if($rule){$rule|Remove-NetFirewallRule}
}
if($record.trustAdded) {
 $path="Cert:\LocalMachine\TrustedPeople\$($record.certificateThumbprint)"
 if(Test-Path $path) {
  if((Get-Item $path).Subject -ne $record.publisher){throw 'Сертификат не принадлежит прототипу'}
  Remove-Item $path
 }
}
Remove-Item $receipt
Write-Output 'Опытные пакет, профиль, правило UDP и добавленное ими доверие удалены; обычный Aheadscale сохранён.'
