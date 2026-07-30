$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

Set-NetFirewallProfile -All -DefaultInboundAction Block -DefaultOutboundAction Allow -Enabled True

Kill-Gdn
Configure-Winc-Network delete
Configure-Groot "$env:TEST_ROOTFS"
Configure-Groot "$env:LIMITS_TEST_URI"
Configure-Winc-Network create
Configure-Gdn "$env:TEST_ROOTFS"


Invoke-Expression "go run github.com/onsi/ginkgo/v2/ginkgo $args --flake-attempts 3"
if ($LastExitCode -ne 0) {
  echo "`n`n`n############# gdn.exe STDOUT"
    Get-Content $env:GDN_OUT_LOG_FILE
    echo "`n`n`n############# gdn.exe STDERR"
    Get-Content $env:GDN_ERR_LOG_FILE
    echo "`n`n`n############# winc-network.exe"
    Get-Content $env:WINC_NETWORK_LOG_FILE
    throw "tests failed"
}
