$ErrorActionPreference = "Stop";
trap { $host.SetShouldExit(1) }

cd release/src/guardian

go version
go vet -mod vendor ./...

Write-Host "compiling test process: $(date)"

$env:GARDEN_TEST_ROOTFS = "N/A"
go run github.com/onsi/ginkgo/v2/ginkgo -r -nodes 8 -race -keepGoing -failOnPending -skipPackage "dadoo,gqt,kawasaki,locksmith,socket2me,signals,runcontainerd\nerd"
if ($LastExitCode -ne 0) {
    throw "Ginkgo run returned error code: $LastExitCode"
}
go run github.com/onsi/ginkgo/v2/ginkgo -r -nodes 8 -timeout 15m -race -keepGoing -failOnPending -randomizeSuites -randomizeAllSpecs -skipPackage "dadoo,kawasaki,locksmith" -focus "Runtime Plugin" gqt
Exit $LastExitCode
