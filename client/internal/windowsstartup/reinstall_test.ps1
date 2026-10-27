param([Parameter(Mandatory=$true)][string]$Helper,[string]$Output)
$ErrorActionPreference='Stop'
$exe='C:\Program Files\Aheadscale\native\aheadscale-ui.exe'
$code=[IO.File]::ReadAllText($Helper)
$suffix='-test-'+[guid]::NewGuid().ToString('N').Substring(0,16)
$sid=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
$name='Aheadscale-Startup-'+$sid+$suffix;$value='Aheadscale'+$suffix
$savedPath='Software\Aheadscale\DesktopInstallerStartupV1';$savedName='Enabled'+$suffix
$svc=New-Object -ComObject Schedule.Service;$svc.Connect();$tasks=$svc.GetFolder('\')
function Invoke-Startup([string]$op){ & ([scriptblock]::Create($code)) -Operation $op -Executable $exe -TestSuffix $suffix|ConvertFrom-Json }
function Clear-Fixture {
 try{$tasks.DeleteTask($name,0)}catch{if($_.Exception.GetBaseException().HResult-ne-2147024894){throw}}
 foreach($path in @('Software\Microsoft\Windows\CurrentVersion\Run','Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run')){
  $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($path,$true);if($k){try{$k.DeleteValue($value,$false)}finally{$k.Dispose()}}
 }
 $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey($savedPath,$true);if($k){try{$k.DeleteValue($savedName,$false)}finally{$k.Dispose()}}
}
$rows=@();$failure=$null
try {
 Clear-Fixture
 $s=Invoke-Startup 'restore';if($s.enabled-or$s.exists){throw 'fresh restore enabled startup'};$rows+=@{case='no saved choice';pass=$true}
 foreach($enabled in @($true,$false)){
  Clear-Fixture
  if($enabled){$null=Invoke-Startup 'enable'}else{$null=Invoke-Startup 'disable'}
  $null=Invoke-Startup 'remove';$s=Invoke-Startup 'inspect';if($s.exists){throw 'uninstall left task'}
  $s=Invoke-Startup 'restore';if($s.enabled-ne$enabled-or$s.exists-ne$enabled){throw "reinstall lost explicit choice $enabled"}
  $rows+=@{case=('roundtrip '+$enabled);pass=$true}
 }
 Clear-Fixture
 $k=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Microsoft\Windows\CurrentVersion\Run');try{$k.SetValue($value,'"'+$exe+'"')}finally{$k.Dispose()}
 $k=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run');try{$k.SetValue($value,[byte[]]@(3,0,0,0,0,0,0,0,0,0,0,0),[Microsoft.Win32.RegistryValueKind]::Binary)}finally{$k.Dispose()}
 $null=Invoke-Startup 'remove'
 $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Run',$true);try{$k.DeleteValue($value,$false)}finally{$k.Dispose()}
 $s=Invoke-Startup 'restore';if($s.enabled-or$s.exists){throw 'Windows disabled Run restored as enabled'};$rows+=@{case='Windows disabled Run';pass=$true}
 Clear-Fixture
 $k=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($savedPath);try{$k.SetValue($savedName,'1',[Microsoft.Win32.RegistryValueKind]::String)}finally{$k.Dispose()}
 $rejected=$false;try{$null=Invoke-Startup 'restore'}catch{$rejected=$true}
 if(-not$rejected){throw 'invalid saved choice accepted'}
 $s=Invoke-Startup 'inspect';if($s.exists){throw 'invalid choice created task'};$rows+=@{case='invalid saved type';pass=$true}
 Clear-Fixture
 $k=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey($savedPath);try{$k.SetValue($savedName,2,[Microsoft.Win32.RegistryValueKind]::DWord)}finally{$k.Dispose()}
 $rejected=$false;try{$null=Invoke-Startup 'restore'}catch{$rejected=$true};if(-not$rejected){throw 'invalid saved value accepted'}
 $s=Invoke-Startup 'inspect';if($s.exists){throw 'invalid value created task'};$rows+=@{case='invalid saved value';pass=$true}
 Clear-Fixture
 $null=Invoke-Startup 'enable';$d=$tasks.GetTask($name).Definition;$d.Actions.Item(1).Path='C:\Windows\notepad.exe';$null=$tasks.RegisterTaskDefinition($name,$d,4,$sid,$null,3)
 $rejected=$false;try{$null=Invoke-Startup 'restore'}catch{$rejected=$true}
 if(-not$rejected-or$tasks.GetTask($name).Definition.Actions.Item(1).Path-ne'C:\Windows\notepad.exe'){throw 'changed task was modified'};$rows+=@{case='changed task preserved';pass=$true}
 Clear-Fixture
 $k=[Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Software\Microsoft\Windows\CurrentVersion\Run');try{$k.SetValue($value,'C:\Windows\notepad.exe')}finally{$k.Dispose()}
 $rejected=$false;try{$null=Invoke-Startup 'restore'}catch{$rejected=$true}
 $k=[Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Software\Microsoft\Windows\CurrentVersion\Run');try{$kept=$k.GetValue($value)}finally{$k.Dispose()}
 if(-not$rejected-or$kept-ne'C:\Windows\notepad.exe'){throw 'foreign Run was modified'};$rows+=@{case='foreign Run preserved';pass=$true}

}catch{$failure=$_.Exception.Message}finally{Clear-Fixture}
$result=@{pass=($null-eq$failure);cases=$rows;error=$failure;fixtureSuffix=$suffix;cleaned=$true}
if($Output){$result|ConvertTo-Json -Depth 8|Set-Content -Encoding UTF8 $Output}else{$result|ConvertTo-Json -Depth 8}
if($failure){exit 1}
