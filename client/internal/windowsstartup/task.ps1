# Общий контракт UI и установщика. Запускается только от текущего пользователя.
param([ValidateSet('state','enable','disable','remove','inspect','restore')][string]$Operation='state',[string]$Executable,[string]$TestSuffix='')
$ErrorActionPreference='Stop';$ProgressPreference='SilentlyContinue'
Set-StrictMode -Version Latest
if($TestSuffix -notmatch '^(-test-[a-f0-9]{16})?$'){throw 'Недопустимое имя тестовой задачи'}
$expected=Join-Path ([Environment]::GetFolderPath('ProgramFiles')) 'Aheadscale\native\aheadscale-ui.exe'
if($Executable -ine $expected){throw 'Неизвестный путь клиента автозапуска'}
$identity=[Security.Principal.WindowsIdentity]::GetCurrent()
$sid=$identity.User.Value
$name='Aheadscale-Startup-'+$sid+$TestSuffix
$legacyName='Aheadscale'+$TestSuffix
$runPath='Software\Microsoft\Windows\CurrentVersion\Run'
$marker='AheadscaleUserStartupV1'
$selectionPath='Software\Aheadscale\DesktopInstallerStartupV1'
$selectionName='Enabled'+$TestSuffix
function Resolve-Sid([string]$value){
 if($value -match '^S-1-'){return [Security.Principal.SecurityIdentifier]::new($value).Value}
 return [Security.Principal.NTAccount]::new($value).Translate([Security.Principal.SecurityIdentifier]).Value
}
$service=New-Object -ComObject Schedule.Service
$service.Connect()
$root=$service.GetFolder('\')
function Read-Task {
 try{return $root.GetTask($name)}catch{
  if($_.Exception.GetBaseException().HResult -eq -2147024894){return $null}
  throw
 }
}
function Assert-Task($task) {
 $d=$task.Definition
 if($d.RegistrationInfo.Author -ne 'Aheadscale' -or $d.RegistrationInfo.Description -ne $marker -or
  (Resolve-Sid $d.Principal.UserId) -ne $sid -or $d.Principal.LogonType -ne 3 -or $d.Principal.RunLevel -ne 0 -or
  $d.Actions.Count -ne 1 -or $d.Actions.Item(1).Type -ne 0 -or $d.Actions.Item(1).Path -ine $Executable -or
  [string]$d.Actions.Item(1).Arguments -ne '' -or [string]$d.Actions.Item(1).WorkingDirectory -ne '' -or
  $d.Triggers.Count -ne 1 -or $d.Triggers.Item(1).Type -ne 9 -or
  (Resolve-Sid $d.Triggers.Item(1).UserId) -ne $sid -or -not $d.Triggers.Item(1).Enabled -or
  [string]$d.Triggers.Item(1).Delay -notin @('','PT0S') -or $d.Settings.RunOnlyIfIdle -or $d.Settings.RunOnlyIfNetworkAvailable -or
  $d.Settings.DisallowStartIfOnBatteries -or $d.Settings.StopIfGoingOnBatteries -or
  $d.Settings.ExecutionTimeLimit -ne 'PT0S' -or $d.Settings.MultipleInstances -ne 2){
  throw 'Задача автозапуска Aheadscale изменена извне; она сохранена'
 }
}
function Read-Legacy {
 $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($runPath)
 if(-not $key){return $null}
 try{$value=$key.GetValue($legacyName,$null,[Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)}finally{$key.Dispose()}
 if($null -ne $value -and $value -ine ('"'+$Executable+'"')){throw 'Запись автозапуска Aheadscale принадлежит другому приложению'}
 return $value
}
function Legacy-Approved {
 $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run')
 if(-not $key){return $true}
 try{$value=$key.GetValue($legacyName)}finally{$key.Dispose()}
 # Не переносим отключённое или неизвестное состояние Windows Startup Apps.
 return $null -eq $value -or ($value -is [byte[]] -and $value.Length -ge 4 -and $value[0] -in @(2,6))
}
function Remove-Legacy {
 $current=Read-Legacy
 if($null -ne $current){
  $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($runPath,$true)
  try{$key.DeleteValue($legacyName,$false)}finally{$key.Dispose()}
 }
 if($null -ne $current -or $Operation -eq 'enable'){
  # Выбор уже хранится в задаче; прежний запрет Run не должен отменить его при обновлении.
  $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run',$true)
  if($key){try{$key.DeleteValue($legacyName,$false)}finally{$key.Dispose()}}
 }
}
# Удаление убирает системную запись, сохраняя только выбор текущего пользователя.
function Save-Selection([bool]$enabled) {
 $key=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($selectionPath)
 try{$key.SetValue($selectionName,[int]$enabled,[Microsoft.Win32.RegistryValueKind]::DWord)}finally{$key.Dispose()}
}
function Read-Selection {
 $key=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($selectionPath)
 if(-not $key){return $null}
 try {
  $value=$key.GetValue($selectionName,$null)
  if($null -eq $value){return $null}
  if($key.GetValueKind($selectionName) -ne [Microsoft.Win32.RegistryValueKind]::DWord -or $value -notin @(0,1)){
   throw 'Сохранённый выбор автозапуска Aheadscale имеет неверный формат'
  }
  return [bool]$value
 }finally{$key.Dispose()}
}
function Create-Task {
 $d=$service.NewTask(0)
 $d.RegistrationInfo.Author='Aheadscale';$d.RegistrationInfo.Description=$marker
 $d.Principal.UserId=$sid;$d.Principal.LogonType=3;$d.Principal.RunLevel=0
 $d.Settings.Enabled=$true;$d.Settings.RunOnlyIfIdle=$false;$d.Settings.RunOnlyIfNetworkAvailable=$false
 $d.Settings.DisallowStartIfOnBatteries=$false;$d.Settings.StopIfGoingOnBatteries=$false
 $d.Settings.ExecutionTimeLimit='PT0S';$d.Settings.MultipleInstances=2
 $t=$d.Triggers.Create(9);$t.UserId=$sid;$t.Delay='PT0S';$t.Enabled=$true
 $a=$d.Actions.Create(0);$a.Path=$Executable
 # Только CREATE: существующая задача никогда не перезаписывается вслепую.
 $task=$root.RegisterTaskDefinition($name,$d,2,$sid,$null,3)
 Assert-Task $task
 return $task
}
$task=Read-Task
if($task){Assert-Task $task}
$legacy=Read-Legacy
$before=[ordered]@{exists=([bool]$task);enabled=([bool]$task -and $task.Enabled)}
switch($Operation){
 'inspect' {$before|ConvertTo-Json -Compress;return}
 'state' {
  if(-not $task -and $null -ne $legacy -and (Legacy-Approved)){$task=Create-Task}
  if($task){Remove-Legacy}
 }
 'enable' {
  if(-not $task){$task=Create-Task}else{$task.Enabled=$true}
  Remove-Legacy
 }
 'disable' {
  if($task){$root.DeleteTask($name,0);$task=$null}
  Remove-Legacy
 }
 'restore' {
  $choice=Read-Selection
  if($null -ne $choice){
   if($choice){
    if(-not $task){$task=Create-Task}else{$task.Enabled=$true}
   }else{
    if($task){$root.DeleteTask($name,0);$task=$null}
   }
   Remove-Legacy
  }
 }
 'remove' {
  $enabled=if($task){[bool]$task.Enabled}else{$null -ne $legacy -and (Legacy-Approved)}
  Save-Selection $enabled
  if($task){$root.DeleteTask($name,0)}
  $before|ConvertTo-Json -Compress
  return
 }
}
[ordered]@{exists=([bool]$task);enabled=([bool]$task -and $task.Enabled)}|ConvertTo-Json -Compress
