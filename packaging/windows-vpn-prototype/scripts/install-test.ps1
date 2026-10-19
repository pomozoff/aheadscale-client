# Только опытная VM: доверие публичному сертификату, без изменения Trusted Root.
$ErrorActionPreference='Stop'
Set-StrictMode -Version Latest
$manifest=Get-Content (Join-Path $PSScriptRoot 'build-manifest.json') -Raw | ConvertFrom-Json
if($manifest.name -ne 'Aheadscale.VpnPrototype' -or $manifest.publisher -ne 'CN=Aheadscale VPN Prototype'){throw 'Неверная идентичность артефакта'}
$package=Join-Path $PSScriptRoot $manifest.packageFile
if((Split-Path $package -Leaf) -ne $manifest.packageFile -or $manifest.packageFile -notmatch '^Aheadscale\.VpnPrototype_[\d.]+_arm64\.msix$'){throw 'Неверное имя пакета'}
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
try {
 if(-not $alreadyTrusted){Import-Certificate -FilePath (Join-Path $PSScriptRoot 'Aheadscale.VpnPrototype.cer') -CertStoreLocation Cert:\LocalMachine\TrustedPeople | Out-Null}
 if((Get-AuthenticodeSignature $package).Status -ne 'Valid'){throw 'Подпись пакета не прошла проверку Windows'}
 Add-AppxPackage -Path $package
 $installed=Get-AppxPackage -Name Aheadscale.VpnPrototype
 if(-not $installed -or $installed.Publisher -ne $manifest.publisher){throw 'Идентичность установленного пакета не совпала'}
 New-Item -ItemType Directory -Force (Split-Path $receipt -Parent) | Out-Null
 [ordered]@{packageFamilyName=$installed.PackageFamilyName;packageFullName=$installed.PackageFullName;publisher=$installed.Publisher;certificateThumbprint=$cert.Thumbprint;trustAdded=(-not $alreadyTrusted);sha256=$manifest.sha256} | ConvertTo-Json | Set-Content $receipt -Encoding utf8
 $installed | Select-Object Name,Version,PackageFamilyName,InstallLocation | ConvertTo-Json
} catch {
 if(-not(Get-AppxPackage -Name Aheadscale.VpnPrototype) -and -not $alreadyTrusted -and (Test-Path $trustPath)){Remove-Item $trustPath}
 throw
}
