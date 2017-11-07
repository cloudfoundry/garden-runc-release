$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

cd gr-release-develop

$env:GOPATH = $PWD
$env:PATH = $env:GOPATH + "/bin;C:/go/bin;" + $env:PATH

if ((Get-Command "go.exe" -ErrorAction SilentlyContinue) -eq $null) {
  Write-Host "Installing Go 1.9.1!"
  Invoke-WebRequest https://storage.googleapis.com/golang/go1.9.1.windows-amd64.msi -OutFile go.msi

  $p = Start-Process -FilePath "msiexec" -ArgumentList "/passive /norestart /i go.msi" -Wait -PassThru

  if($p.ExitCode -ne 0) {
    throw "Golang MSI installation process returned error code: $($p.ExitCode)"
  }

  Write-Host "Go is installed!"
}

Write-Host "Installing Ginkgo"
go.exe install ./src/github.com/onsi/ginkgo/ginkgo
if ($LastExitCode -ne 0) {
    throw "Ginkgo installation process returned error code: $LastExitCode"
}

cd src/code.cloudfoundry.org/guardian

go version
go vet ./...
Write-Host "compiling test process: $(date)"

$env:GARDEN_TEST_ROOTFS = "N/A"
ginkgo -r -p -race -keepGoing -failOnPending -skipPackage "dadoo,gqt,kawasaki,locksmith,socket2me,signals"
if ($LastExitCode -ne 0) {
    throw "Ginkgo run returned error code: $LastExitCode"
}
ginkgo -r -p -race -keepGoing -failOnPending -randomizeSuites -randomizeAllSpecs -skipPackage "dadoo,kawasaki,locksmith" -focus "Runtime Plugin" gqt
Exit $LastExitCode
