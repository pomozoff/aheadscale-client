# Закрытый preview для Windows 11; вызывается единственным установщиком.
# $artifactRoot задаёт установщик, без поиска программ в PATH.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$cli='C:\Program Files\Aheadscale\aheadscale.exe'
$private='C:\ProgramData\AheadscaleVpnPrototype'
$runtime='C:\Program Files\Aheadscale\native'
$family='Aheadscale.VpnPrototype_9kyyx27wet2q6'
function Invoke-Bundled([string]$name) {
 $text=[IO.File]::ReadAllText((Join-Path $artifactRoot $name))
 & ([scriptblock]::Create($text.Replace('$PSScriptRoot','$artifactRoot')))
}
function Read-Service { Get-CimInstance Win32_Service -Filter "Name='Aheadscale'" }
function Test-StartupOwnership {
 $null=& ([scriptblock]::Create([IO.File]::ReadAllText((Join-Path $artifactRoot 'startup-task.ps1')))) -Operation inspect -Executable (Join-Path $runtime 'aheadscale-ui.exe')
}

function Set-ServiceImage([string]$image) {
 $changed=Invoke-CimMethod -InputObject (Read-Service) -MethodName Change -Arguments @{PathName=$image}
 if($changed.ReturnValue -ne 0){throw 'Не удалось изменить собственную службу Aheadscale'}
}
function Stop-OwnedPackageProcesses($package) {
 $brokers=@(Get-CimInstance Win32_Process -Filter "Name='AheadscaleVpnBroker.exe'" | Where-Object ExecutablePath -ieq (Join-Path $package.InstallLocation 'AheadscaleVpnBroker.exe'))
 $ids=@($brokers|ForEach-Object ProcessId)
 Get-CimInstance Win32_Process -Filter "Name='aheadscale.exe'" | Where-Object { $_.ParentProcessId -in $ids -and $_.ExecutablePath -ieq $cli } | ForEach-Object {Stop-Process -Id $_.ProcessId -ErrorAction SilentlyContinue}
 foreach($name in @('AheadscaleVpnBroker.exe','AheadscaleVpnHost.exe')) {
  Get-CimInstance Win32_Process -Filter ("Name='"+$name+"'") | Where-Object {
   $_.ExecutablePath -ieq (Join-Path $package.InstallLocation $name)
  } | ForEach-Object { Stop-Process -Id $_.ProcessId -ErrorAction SilentlyContinue }
 }
}
function Restore-Standard {
 $backup=Get-Content (Join-Path $private 'native-service-backup.json') -Raw|ConvertFrom-Json
 if($backup.owner -ne $family){throw 'Неизвестный владелец резерва'}
 $expectedOriginal='"'+$cli+'" service run --log-level info --daemon-addr npipe://aheadscale --log-file C:\ProgramData\Aheadscale\client.log --disable-profiles'
 if($backup.originalImagePath -ine $expectedOriginal){throw 'Неизвестный исходный путь службы; откат отменён'}
 $service=Read-Service
 if($service.PathName -ne $backup.experimentalImagePath -and $service.PathName -ne $backup.originalImagePath){throw 'Служба изменена вне установщика'}
 if((Get-FileHash $cli).Hash -ne $backup.originalBinarySha256){throw 'Исходный клиент изменился; автоматический откат отменён'}
 $package=Get-AppxPackage -Name Aheadscale.VpnPrototype
 if($package){Stop-OwnedPackageProcesses $package}
 Stop-Service Aheadscale
 (Get-Service Aheadscale).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
 $deadline=(Get-Date).AddSeconds(30)
 while(@(Get-NetIPAddress -AddressFamily IPv4 -ErrorAction SilentlyContinue|Where-Object InterfaceAlias -in @('Aheadscale','Aheadscale Prototype')).Count -gt 0) {
  if((Get-Date) -ge $deadline){throw 'Системный туннель ещё не освободил адрес; второй интерфейс не запускается'}
  Start-Sleep -Milliseconds 250
 }
 Set-ServiceImage $backup.originalImagePath
 if($backup.wasRunning){Start-Service Aheadscale}
}
function Restore-DesktopIntegration($backup) {
 $runKey='HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
 $startup=& ([scriptblock]::Create([IO.File]::ReadAllText((Join-Path $artifactRoot 'startup-task.ps1')))) -Operation remove -Executable (Join-Path $runtime 'aheadscale-ui.exe') | ConvertFrom-Json
 if($startup.exists){
  if($startup.enabled){
   # Возвращаем текущий выбор пользователя стандартному UI, независимо от старого резерва.
   Set-ItemProperty $runKey -Name Aheadscale -Value '"C:\Program Files\Aheadscale\aheadscale-ui.exe"'
  }
 }

 $run=Get-ItemProperty $runKey -Name Aheadscale -ErrorAction SilentlyContinue
 if($run -and $run.PSObject.Properties.Name -contains 'Aheadscale' -and $run.Aheadscale -eq ('"'+$runtime+'\aheadscale-ui.exe"')) {
  $previous=$null
  if($backup.PSObject.Properties.Name -contains 'oldAutorun'){$previous=$backup.oldAutorun}
  if($previous -eq ('"'+$runtime+'\aheadscale-ui.exe"')){$previous='"C:\Program Files\Aheadscale\aheadscale-ui.exe"'}
  if($previous){
   if($previous -ne '"C:\Program Files\Aheadscale\aheadscale-ui.exe"'){throw 'Неизвестный исходный автозапуск'}
   Set-ItemProperty $runKey -Name Aheadscale -Value $previous
  }else{Remove-ItemProperty $runKey -Name Aheadscale}
 }
 if($backup.PSObject.Properties.Name -contains 'shortcuts') {
  $allowed=@(foreach($folder in @([Environment]::GetFolderPath('DesktopDirectory'),[Environment]::GetFolderPath('CommonDesktopDirectory'),[Environment]::GetFolderPath('Programs'),[Environment]::GetFolderPath('CommonPrograms'))){Join-Path $folder 'Aheadscale.lnk'})
  $shell=New-Object -ComObject WScript.Shell
  foreach($saved in $backup.shortcuts){
   if($saved.path -notin $allowed -or $saved.target -ine 'C:\Program Files\Aheadscale\aheadscale-ui.exe'){throw 'Неизвестный исходный ярлык'}
   if(Test-Path $saved.path){$link=$shell.CreateShortcut($saved.path);if($link.TargetPath -ieq (Join-Path $runtime 'aheadscale-ui.exe')){$link.TargetPath=$saved.target;$link.IconLocation=$saved.icon;$link.Save()}}
  }
  # Ранние preview не сохраняли исходные ярлыки при повторном обновлении.
  foreach($path in $allowed){if(Test-Path $path){$link=$shell.CreateShortcut($path);if($link.TargetPath -ieq (Join-Path $runtime 'aheadscale-ui.exe')){$link.TargetPath='C:\Program Files\Aheadscale\aheadscale-ui.exe';$link.IconLocation=$link.TargetPath+',0';$link.Save()}}}
 }
}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$owner=$identity.User.Value
$session=(Get-Process -Id $PID).SessionId
$desktop=@(Get-CimInstance Win32_Process -Filter "Name='explorer.exe'" | Where-Object SessionId -eq $session)
if(-not $desktop -or @($desktop|ForEach-Object{(Invoke-CimMethod -InputObject $_ -MethodName GetOwnerSid).Sid}|Where-Object{$_ -ne $owner}).Count){throw 'Preview нужно запускать из своей административной учётной записи Windows'}
if(-not [Environment]::Is64BitProcess -or [Environment]::OSVersion.Version.Build -lt 26100){throw 'Этот preview требует Windows 11 24H2 или новее и 64-битный PowerShell'}
$appKey='HKLM:\Software\Microsoft\Windows\CurrentVersion\Uninstall\Aheadscale'
if($operation -eq 'remove') {
 $backup=Get-Content (Join-Path $private 'native-service-backup.json') -Raw|ConvertFrom-Json
 if($backup.ownerSid -ne $owner){throw 'Удаление preview доступно владельцу установки'}
 $originalUninstaller='C:\Program Files\Aheadscale\aheadscale_uninstall.exe'
 if((Get-FileHash $originalUninstaller).Hash -ne $backup.originalUninstallerSha256){throw 'Исходный деинсталлятор изменён; файлы сохранены'}
 Test-StartupOwnership
 Get-CimInstance Win32_Process -Filter "Name='aheadscale-ui.exe'"|Where-Object {$_.SessionId -eq $session -and $_.ExecutablePath -ieq (Join-Path $runtime 'aheadscale-ui.exe')}|ForEach-Object{Stop-Process -Id $_.ProcessId}
 Restore-Standard
 Invoke-Bundled 'remove-test.ps1'
 Restore-DesktopIntegration $backup
 Set-ItemProperty $appKey -Name UninstallString -Value $originalUninstaller
 Move-Item -LiteralPath $private -Destination ($private+'.removed-'+[guid]::NewGuid().ToString('N'))
 return
}
$manifest=Get-Content (Join-Path $artifactRoot 'build-manifest.json') -Raw|ConvertFrom-Json
$desktopManifest=Get-Content (Join-Path $artifactRoot 'desktop-manifest.json') -Raw|ConvertFrom-Json
# Все проверки файлов выполняются до изменения службы или доверия сертификатам.
foreach($pair in @(@($manifest.packageFile,$manifest.sha256),@($manifest.nativeUIFile,$manifest.nativeUISha256),@('standard-setup.exe',$desktopManifest.standardSha256))) {
 if((Split-Path $pair[0] -Leaf) -ne $pair[0] -or (Get-FileHash (Join-Path $artifactRoot $pair[0])).Hash -ne $pair[1]){throw 'Контрольная сумма установщика не совпала'}
}
$package=Get-AppxPackage -Name Aheadscale.VpnPrototype
$resume=$false
if((Read-Service) -and (Get-Service Aheadscale).Status -eq 'Running') {
 $prior=(& $cli status --json|Out-String)|ConvertFrom-Json
 $resume=$prior.daemonStatus -eq 'Connected'
}
# Завершаем только собственные интерфейсы в текущем пользовательском сеансе.
$previousReceipt=Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json'
$previousSigner=$null
if(Test-Path $previousReceipt){$previousSigner=(Get-Content $previousReceipt -Raw|ConvertFrom-Json).certificateThumbprint}
if($package){Test-StartupOwnership}
Get-CimInstance Win32_Process | Where-Object { $_.SessionId -eq $session -and $_.Name -in @('aheadscale-ui.exe','AheadscaleNativeUI.exe') } | ForEach-Object {
 $owned=$false
 if($_.ExecutablePath -ieq 'C:\Program Files\Aheadscale\aheadscale-ui.exe') {$owned=(Get-FileHash $_.ExecutablePath).Hash -eq $desktopManifest.standardUiSha256}
 elseif($previousSigner -and $_.ExecutablePath) {$signature=Get-AuthenticodeSignature $_.ExecutablePath;$owned=$signature.Status -eq 'Valid' -and $signature.SignerCertificate -and $signature.SignerCertificate.Thumbprint -eq $previousSigner}
 if($owned){
  $uiId=$_.ProcessId
  if($package){Get-CimInstance Win32_Process -Filter "Name='AheadscaleVpnController.exe'"|Where-Object {$_.ParentProcessId -eq $uiId -and $_.ExecutablePath -ieq (Join-Path $package.InstallLocation 'AheadscaleVpnController.exe')}|ForEach-Object{Stop-Process -Id $_.ProcessId -ErrorAction SilentlyContinue}}
  Stop-Process -Id $uiId
 }
}
if($package) {
 # Удалять старое приложение разрешено только с его собственной квитанцией.
 if($package.PackageFamilyName -ne $family -or -not(Test-Path (Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json')) -or -not(Test-Path (Join-Path $private 'native-service-backup.json'))){throw 'Нет проверенного резерва предыдущего preview'}
 $previousBackup=Get-Content (Join-Path $private 'native-service-backup.json') -Raw|ConvertFrom-Json
 if($previousBackup.ownerSid -ne $owner){throw 'Обновление preview доступно владельцу установки'}
 Restore-Standard
 Invoke-Bundled 'remove-test.ps1'
 Restore-DesktopIntegration $previousBackup
 # Базовый NSIS читает этот ключ и вызывает прежний деинсталлятор.
 # Native-пакет уже удалён; повторный запуск его uninstaller недопустим.
 $standardUninstaller='C:\Program Files\Aheadscale\aheadscale_uninstall.exe'
 if((Get-FileHash $standardUninstaller).Hash -ne $previousBackup.originalUninstallerSha256){throw 'Исходный деинсталлятор изменён; обновление остановлено'}
 Set-ItemProperty $appKey -Name UninstallString -Value $standardUninstaller
 $archive=$private+'.previous-'+[guid]::NewGuid().ToString('N')
 Move-Item -LiteralPath $private -Destination $archive
}
if(-not(Read-Service) -or -not(Test-Path $cli) -or (Get-FileHash $cli).Hash -ne $desktopManifest.standardCliSha256) {
 $setup=Start-Process (Join-Path $artifactRoot 'standard-setup.exe') -ArgumentList '/S' -PassThru -Wait
 if($setup.ExitCode){throw 'Базовый установщик Aheadscale завершился с ошибкой'}
}
if((Get-FileHash $cli).Hash -ne $desktopManifest.standardCliSha256){throw 'Версия исходного клиента не соответствует preview; служба сохранена'}
$service=Read-Service
if($service.PathName -notlike ('"'+$cli+'" service run *')){throw 'Неизвестная конфигурация службы Aheadscale'}
if(Test-Path $private){throw 'Неизвестный резерв службы; существующие данные сохранены'}
$originalImage=$service.PathName
$wasRunning=$service.State -eq 'Running'
$originalHash=(Get-FileHash $cli).Hash
$runKey='HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$oldRun=$null
$run=Get-ItemProperty $runKey -Name Aheadscale -ErrorAction SilentlyContinue
if($run -and $run.PSObject.Properties.Name -contains 'Aheadscale'){$oldRun=$run.Aheadscale}
$shortcuts=@()
$added=$false;$switched=$false;$privateCreated=$false
try {
 Invoke-Bundled 'install-test.ps1'
 $added=$true
 $package=Get-AppxPackage -Name Aheadscale.VpnPrototype
 $native=Join-Path $package.InstallLocation 'AheadscaleNativeDaemon.exe'
 if((Get-FileHash $native).Hash -ne $manifest.nativeDaemonSha256){throw 'Неверная служба системного VPN'}
 $next='"'+$native+'" service run --log-level info --daemon-addr npipe://aheadscale --log-file C:\ProgramData\Aheadscale\client.log --disable-profiles --vpn-prototype-owner-sid '+$owner
 New-Item -ItemType Directory -Path $private | Out-Null
 $privateCreated=$true
 $acl=[Security.AccessControl.DirectorySecurity]::new()
 $acl.SetSecurityDescriptorSddlForm('O:BAG:BAD:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)')
 (Get-Item $private).SetAccessControl($acl)
 $backup=[ordered]@{owner=$family;ownerSid=$owner;originalImagePath=$originalImage;experimentalImagePath=$next;originalBinarySha256=$originalHash;wasRunning=$wasRunning;oldAutorun=$oldRun;sourceCommit=$manifest.sourceCommit;originalUninstallerSha256=(Get-FileHash 'C:\Program Files\Aheadscale\aheadscale_uninstall.exe').Hash}
 $backup|ConvertTo-Json|Set-Content (Join-Path $private 'native-service-backup.json') -Encoding UTF8
 Stop-Service Aheadscale
 (Get-Service Aheadscale).WaitForStatus('Stopped',[TimeSpan]::FromSeconds(30))
 Set-ServiceImage $next
 $switched=$true
 Start-Service Aheadscale
 $controller=Join-Path $env:LOCALAPPDATA 'Microsoft\WindowsApps\aheadscale-vpn-prototype.exe'
 & $controller add
 if($LASTEXITCODE){throw 'Не создан системный профиль Aheadscale'}
 New-Item -ItemType Directory -Path $runtime -Force|Out-Null
 Copy-Item (Join-Path $artifactRoot $manifest.nativeUIFile) (Join-Path $runtime 'aheadscale-ui.exe') -Force
 if($oldRun){Set-ItemProperty $runKey -Name Aheadscale -Value ('"'+$runtime+'\aheadscale-ui.exe"')}
 $shell=New-Object -ComObject WScript.Shell
 foreach($folder in @([Environment]::GetFolderPath('DesktopDirectory'),[Environment]::GetFolderPath('CommonDesktopDirectory'),[Environment]::GetFolderPath('Programs'),[Environment]::GetFolderPath('CommonPrograms'))) {
  $link=Join-Path $folder 'Aheadscale.lnk'
  if(Test-Path $link){$shortcut=$shell.CreateShortcut($link);if($shortcut.TargetPath -ieq 'C:\Program Files\Aheadscale\aheadscale-ui.exe'){$shortcuts+=@{path=$link;target=$shortcut.TargetPath;icon=$shortcut.IconLocation};$shortcut.TargetPath=Join-Path $runtime 'aheadscale-ui.exe';$shortcut.IconLocation=$shortcut.TargetPath+',0';$shortcut.Save()}}
 }
 $backup.shortcuts=$shortcuts
 $backup|ConvertTo-Json -Depth 5|Set-Content (Join-Path $private 'native-service-backup.json') -Encoding UTF8
 if($resume){& $controller connect;if($LASTEXITCODE){Write-Warning 'Подключение можно повторить в Aheadscale'}}
 # Полная переустановка возвращает прежний выбор, не запуская GUI сейчас.
 $null=& ([scriptblock]::Create([IO.File]::ReadAllText((Join-Path $artifactRoot 'startup-task.ps1')))) -Operation restore -Executable (Join-Path $runtime 'aheadscale-ui.exe')
} catch {
 $failure=$_
 if($switched){Restore-Standard}
 if($added){Invoke-Bundled 'remove-test.ps1'}
 if($privateCreated){Move-Item -LiteralPath $private -Destination ($private+'.failed-'+[guid]::NewGuid().ToString('N'))}
 if($oldRun){Set-ItemProperty $runKey -Name Aheadscale -Value $oldRun}
 foreach($saved in $shortcuts){$shortcut=$shell.CreateShortcut($saved.path);if($shortcut.TargetPath -ieq (Join-Path $runtime 'aheadscale-ui.exe')){$shortcut.TargetPath=$saved.target;$shortcut.IconLocation=$saved.icon;$shortcut.Save()}}
 throw $failure
}
