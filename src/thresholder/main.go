package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"thresholder/calculator"
	"thresholder/disk"

	"code.cloudfoundry.org/grootfs/commands/config"
	yaml "gopkg.in/yaml.v2"
)

const MIN_STORE_SIZE = 15 * 1024 * 1024 * 1024

func main() {
	if len(os.Args) != 7 {
		failWithMessage("Not all input arguments provided (Expected: 6)")
	}

	reservedSpace := megabytesToBytes(parseIntParameter(os.Args[1], "Reserved space parameter must be a number"))
	routineGC, err := strconv.ParseBool(os.Args[2])
	if err != nil {
		failWithMessage(err.Error())
	}

	diskPath := os.Args[3]
	configPath := os.Args[4]
	gardenGcThreshold := megabytesToBytes(parseIntParameter(os.Args[5], "Garden GC threshold parameter must be a number"))
	grootfsGcThreshold := megabytesToBytes(parseIntParameter(os.Args[6], "GrootFS GC threshold parameter must be a number"))

	diskSize, err := disk.NewMeter().GetAvailableSpace(diskPath)
	if err != nil {
		failWithMessage(err.Error())
	}

	calc := calculator.NewModernCalculator(reservedSpace, diskSize, MIN_STORE_SIZE, routineGC)
	if gardenGcThreshold > 0 || grootfsGcThreshold > 0 {
		calc = calculator.NewOldFashionedCalculator(diskSize, gardenGcThreshold, grootfsGcThreshold)
	}

	config := parseFileParameter(configPath, "Grootfs config parameter must be path to valid grootfs config file")
	config.Create.WithClean = calc.ShouldCollectGarbageOnCreate()
	config.Clean.ThresholdBytes = calc.CalculateGCThreshold()
	config.Init.StoreSizeBytes = calc.CalculateStoreSize()

	writeConfig(config, configPath)

	if config.Init.StoreSizeBytes == diskSize {
		fmt.Printf("Warning: The GrootFS was unable to reserve space for other jobs and won't be able to enforce the requested reserved space. To avoid this, make sure GrootFS has %dGB available for its store by reducing the `grootfs.reserved_space_for_other_jobs_in_mb` or using a bigger disk.", bytesToGigabytes(MIN_STORE_SIZE))
	}
}

func parseIntParameter(parameterValue, failureMessage string) int64 {
	intValue, err := strconv.ParseInt(parameterValue, 10, 64)
	if err != nil {
		failWithMessage(failureMessage)
	}

	return intValue
}

func parseFileParameter(parameterValue, failureMessage string) *config.Config {
	configBytes, err := ioutil.ReadFile(parameterValue)
	if err != nil {
		failWithMessage(failureMessage)
	}

	var c config.Config
	if err := yaml.Unmarshal(configBytes, &c); err != nil {
		failWithMessage(failureMessage)
	}

	return &c
}

func writeConfig(config *config.Config, configPath string) {
	configBytes, err := yaml.Marshal(config)
	if err != nil {
		failWithMessage(err.Error())
	}
	if err := ioutil.WriteFile(configPath, configBytes, 0600); err != nil {
		failWithMessage(err.Error())
	}
}

func failWithMessage(failureMessage string) {
	fmt.Println(failureMessage)
	os.Exit(1)
}

func megabytesToBytes(megabytes int64) int64 {
	return megabytes * 1024 * 1024
}

func bytesToGigabytes(bytes int64) int64 {
	return bytes / (1024 * 1024 * 1024)
}
