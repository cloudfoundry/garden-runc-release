$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

cd gr-release-develop

$env:GOPATH = $PWD
$env:PATH = $env:GOPATH + "/bin;C:/go/bin;" + $env:PATH

if ((Get-Command "go.exe" -ErrorAction SilentlyContinue) -eq $null) {
  Write-Host "Installing Go 1.8.1!"
  Invoke-WebRequest https://storage.googleapis.com/golang/go1.8.1.windows-amd64.msi -OutFile go.msi

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

ginkgo -r -p -race -keepGoing -nodes=8 -failOnPending -randomizeSuites gqt
Exit $LastExitCode
