# Только опытная VM: доверие публичному сертификату, без изменения Trusted Root.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$manifest=Get-Content (Join-Path $PSScriptRoot 'build-manifest.json') -Raw | ConvertFrom-Json
if($manifest.name -ne 'Aheadscale.VpnPrototype' -or $manifest.publisher -ne 'CN=Aheadscale VPN Prototype'){throw 'Неверная идентичность артефакта'}
$package=Join-Path $PSScriptRoot $manifest.packageFile
if((Split-Path $package -Leaf) -ne $manifest.packageFile -or $manifest.packageFile -notmatch '^Aheadscale\.VpnPrototype_[\d.]+_(arm64|amd64)\.msix$'){throw 'Неверное имя пакета'}
if((Get-FileHash $package -Algorithm SHA256).Hash -ne $manifest.sha256){throw 'SHA256 не совпал'}
$cert=[Security.Cryptography.X509Certificates.X509Certificate2]::new((Join-Path $PSScriptRoot 'Aheadscale.VpnPrototype.cer'))
if($cert.HasPrivateKey -or $cert.Subject -ne $manifest.publisher -or $cert.Thumbprint -ne $manifest.certificateThumbprint){throw 'Неверный публичный сертификат'}
$signature=Get-AuthenticodeSignature $package
if(-not $signature.SignerCertificate -or $signature.SignerCertificate.Thumbprint -ne $cert.Thumbprint){throw 'Подпись пакета не соответствует сертификату'}
$receipt=Join-Path $env:LOCALAPPDATA 'AheadscaleVpnPrototype\install.json'
if(Test-Path $receipt){throw 'Сначала удалите предыдущую опытную установку через remove-test.ps1'}
if(Get-AppxPackage -Name Aheadscale.VpnPrototype){throw 'Опытный пакет уже установлен; не изменяем без квитанции'}
$trustPath="Cert:\LocalMachine\TrustedPeople\$($cert.Thumbprint)"
$alreadyTrusted=Test-Path $trustPath
$firewallRuleName='Aheadscale.VpnPrototype-WireGuard-'+[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
if(Get-NetFirewallRule -Name $firewallRuleName -PolicyStore PersistentStore -ErrorAction SilentlyContinue){throw 'Правило VPN уже существует; не изменяем состояние без квитанции'}
$createdFirewallRule=$null;$installed=$null;$packageAdded=$false
try {
 if(-not $alreadyTrusted){Import-Certificate -FilePath (Join-Path $PSScriptRoot 'Aheadscale.VpnPrototype.cer') -CertStoreLocation Cert:\LocalMachine\TrustedPeople | Out-Null}
 if((Get-AuthenticodeSignature $package).Status -ne 'Valid'){throw 'Подпись пакета не прошла проверку Windows'}
 Add-AppxPackage -Path $package
 $packageAdded=$true
 $installed=Get-AppxPackage -Name Aheadscale.VpnPrototype
 if(-not $installed -or $installed.Publisher -ne $manifest.publisher -or $installed.PackageFamilyName -ne 'Aheadscale.VpnPrototype_9kyyx27wet2q6'){throw 'Идентичность установленного пакета не совпала'}
 $nativeDaemon=Join-Path $installed.InstallLocation 'AheadscaleNativeDaemon.exe'
 if((Get-FileHash $nativeDaemon -Algorithm SHA256).Hash -ne $manifest.nativeDaemonSha256){throw 'SHA256 установленной опытной службы не совпал'}
 # SCM запускает службу как LocalSystem; пользовательское разрешение MSIX её не покрывает.
 $createdFirewallRule=New-NetFirewallRule -Name $firewallRuleName -DisplayName 'Aheadscale Prototype: VPN' -Group 'Aheadscale.VpnPrototype' -PolicyStore PersistentStore -Direction Inbound -Action Allow -Protocol UDP -LocalPort 51830 -RemoteAddress Any -Profile Any -Program $nativeDaemon
 New-Item -ItemType Directory -Force (Split-Path $receipt -Parent) | Out-Null
 [ordered]@{packageFamilyName=$installed.PackageFamilyName;packageFullName=$installed.PackageFullName;publisher=$installed.Publisher;certificateThumbprint=$cert.Thumbprint;trustAdded=(-not $alreadyTrusted);sha256=$manifest.sha256;firewallRuleName=$firewallRuleName;nativeDaemonPath=$nativeDaemon} | ConvertTo-Json | Set-Content $receipt -Encoding utf8
 $installed | Select-Object Name,Version,PackageFamilyName,InstallLocation | ConvertTo-Json
} catch {
 $failure=$_
 if($createdFirewallRule){$createdFirewallRule|Remove-NetFirewallRule}
 if($packageAdded -and $installed){
  $current=Get-AppxPackage -Name Aheadscale.VpnPrototype
  if($current -and $current.PackageFullName -eq $installed.PackageFullName -and $current.Publisher -eq $manifest.publisher){Remove-AppxPackage -Package $current.PackageFullName}
  if(Test-Path $receipt){Remove-Item -LiteralPath $receipt}
 }
 if(-not(Get-AppxPackage -Name Aheadscale.VpnPrototype) -and -not $alreadyTrusted -and (Test-Path $trustPath)){Remove-Item $trustPath}
 throw $failure
}
