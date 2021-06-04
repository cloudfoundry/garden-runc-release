$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

cd gr-release-develop

$env:PWD = (Get-Location)
$env:GOPATH = ${env:PWD} + "\src\gopath"
$env:PATH = $env:GOPATH + "/bin;C:/go/bin;" + $env:PATH
$env:GO111MODULE = "off"

Write-Host "Installing Ginkgo"
go.exe get ./src/gopath/src/github.com/onsi/ginkgo/ginkgo
if ($LastExitCode -ne 0) {
    throw "Ginkgo installation process returned error code: $LastExitCode"
}

cd ./src/guardian

$env:GO111MODULE = "on"

go version
go vet -mod vendor ./...
Write-Host "compiling test process: $(date)"

$env:GARDEN_TEST_ROOTFS = "N/A"
ginkgo -mod vendor -r -nodes 8 -race -keepGoing -failOnPending -skipPackage "dadoo,gqt,kawasaki,locksmith,socket2me,signals,runcontainerd\nerd"
if ($LastExitCode -ne 0) {
    throw "Ginkgo run returned error code: $LastExitCode"
}
ginkgo -mod vendor -r -nodes 8 -timeout 15m -race -keepGoing -failOnPending -randomizeSuites -randomizeAllSpecs -skipPackage "dadoo,kawasaki,locksmith" -focus "Runtime Plugin" gqt
Exit $LastExitCode
