package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"syscall"

	"code.cloudfoundry.org/grootfs/commands/config"
	yaml "gopkg.in/yaml.v2"
)

func main() {
	if len(os.Args) != 6 {
		failWithMessage("Not all input arguments provided (Expected: 5)")
	}

	gardenGcThreshold := bytesToMb(parseIntParameter(os.Args[1], "Garden GC threshold parameter must be a number"))
	grootGcThreshold := bytesToMb(parseIntParameter(os.Args[2], "Groot GC threshold parameter must be a number"))
	reservedSpace := bytesToMb(parseIntParameter(os.Args[3], "Reserved space parameter must be a number"))
	diskPath := os.Args[4]
	configPath := os.Args[5]
	config := parseFileParameter(configPath, "Grootfs config parameter must be path to valid grootfs config file")

	threshold := calculateThreshold(gardenGcThreshold, grootGcThreshold, reservedSpace, getTotalSpace(diskPath))
	if threshold >= 0 {
		config.Create.WithClean = true
		config.Clean.ThresholdBytes = threshold
	}

	writeConfig(config, configPath)

	fmt.Println(threshold)
}

func calculateThreshold(gardenGcThresholdInMb, grootGcThresholdInMb, reservedSpaceInMb, diskSize int64) int64 {
	if gardenGcThresholdInMb >= 0 {
		return gardenGcThresholdInMb
	}

	if grootGcThresholdInMb >= 0 {
		return grootGcThresholdInMb
	}

	if reservedSpaceInMb < 0 {
		return reservedSpaceInMb
	}

	if diskSize < reservedSpaceInMb {
		return 0
	}

	return diskSize - reservedSpaceInMb
}

func getTotalSpace(diskPath string) int64 {
	var fsStat syscall.Statfs_t
	if err := syscall.Statfs(diskPath, &fsStat); err != nil {
		failWithMessage(fmt.Sprintf("Cannot stat %s: %s\n", diskPath, err))
	}

	return fsStat.Bsize * int64(fsStat.Blocks)
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

func bytesToMb(bytes int64) int64 {
	return bytes * 1024 * 1024
}
