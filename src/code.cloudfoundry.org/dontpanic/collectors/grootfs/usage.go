package grootfs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"gopkg.in/yaml.v2"
)

type UsageCollector struct {
	configPath string
	config     grootfsConfig
	runner     CommandRunner
}

//go:generate counterfeiter . CommandRunner

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

const (
	dirName               = "grootfs"
	volumesDirectory      = "volumes"
	metaDirectory         = "meta"
	dependenciesDirectory = "dependencies"
)

func NewUsageCollector(configPath string, cmdRunner CommandRunner) UsageCollector {
	return UsageCollector{
		configPath: configPath,
		runner:     cmdRunner,
	}
}

type grootfsConfig struct {
	Store      string `yaml:"store"`
	GrootFSBin string
}

func (c UsageCollector) parseGrootfsConfig() (grootfsConfig, error) {
	contents, err := os.ReadFile(c.configPath)
	if err != nil {
		return grootfsConfig{}, fmt.Errorf("failed to read grootfs config file %q: %v", c.configPath, err)
	}

	var config grootfsConfig
	err = yaml.Unmarshal(contents, &config)
	if err != nil {
		return grootfsConfig{}, fmt.Errorf("failed to unmarshal grootfs config file: %v", err)
	}

	config.GrootFSBin = "/var/vcap/packages/grootfs/bin/grootfs"
	return config, nil
}

func (c UsageCollector) Run(ctx context.Context, reportDir string, stdout io.Writer) error {
	config, err := c.parseGrootfsConfig()
	if err != nil {
		return fmt.Errorf("failed to parse grootfs config file: %v", err)
	}
	c.config = config

	grootfsDir := filepath.Join(reportDir, dirName)
	err = os.MkdirAll(grootfsDir, 0755)
	if err != nil {
		return fmt.Errorf("failed to create %q directory inside report: %v", grootfsDir, err)
	}

	storeType := filepath.Base(c.config.Store)
	outputPath := filepath.Join(grootfsDir, storeType+"-usage.txt")
	outputFile, err := os.Create(outputPath)
	if err != nil {
		return fmt.Errorf("failed to create output file %q: %v", outputPath, err)
	}
	defer outputFile.Close()

	volumesPath := filepath.Join(c.config.Store, volumesDirectory)
	totalVolumeSizeOnDisk, err := c.sizeOnDisk(volumesPath, false)
	if err != nil {
		return fmt.Errorf("failed to calculate total volume size: %v", err)
	}

	usedVolumes, unusedVolumes, err := c.getVolumes()
	if err != nil {
		return fmt.Errorf("failed to get volumes: %v", err)
	}

	usedVolumesSizeOnDisk, err := c.volumesSizeOnDisk(usedVolumes)
	if err != nil {
		return fmt.Errorf("failed to get used volume size on disk: %v", err)
	}

	unusedVolumesSizeOnDisk, err := c.volumesSizeOnDisk(unusedVolumes)
	if err != nil {
		return fmt.Errorf("failed to get unused volume size on disk: %v", err)
	}

	usedVolumesSize, err := c.volumesSizeFromMeta(usedVolumes)
	if err != nil {
		return fmt.Errorf("failed to calculate used volume size: %v", err)
	}

	unusedVolumesSize, err := c.volumesSizeFromMeta(unusedVolumes)
	if err != nil {
		return fmt.Errorf("failed to calculate unused volume size: %v", err)
	}

	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "volumes-total-on-disk:", totalVolumeSizeOnDisk)
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "volumes-used-on-disk:", usedVolumesSizeOnDisk)
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "volumes-unused-on-disk:", unusedVolumesSizeOnDisk)

	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "volumes-used-reported:", usedVolumesSize)
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "volumes-unused-reported:", unusedVolumesSize)

	imagesSize, quotasSize, err := c.imagesStats()
	if err != nil {
		return fmt.Errorf("failed to get images stats: %v", err)
	}

	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "images-exclusive:", imagesSize)
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "quotas-size:", quotasSize)

	backingStoreSize, err := c.sizeOnDisk(c.config.Store+".backing-store", false)
	if err != nil {
		return fmt.Errorf("failed to calculate backing store size: %v", err)
	}
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "backing-store-actual-size:", backingStoreSize)

	backingStoreMaxSize, err := c.sizeOnDisk(c.config.Store+".backing-store", true)
	if err != nil {
		return fmt.Errorf("failed to calculate backing store max size: %v", err)
	}
	fmt.Fprintf(outputFile, "%-30s %12d bytes\n", "backing-store-max-size:", backingStoreMaxSize)

	return nil
}

func (c UsageCollector) imagesStats() (int64, int64, error) {
	imageIDs, err := c.getImageIDs()
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get image IDs: %v", err)
	}

	var imagesSize int64
	var quotasSize int64

	for _, id := range imageIDs {
		imageStats, err := c.getImageStats(id)
		if err != nil {
			return 0, 0, fmt.Errorf("failed to get image size for %q: %v", id, err)
		}
		imagesSize += imageStats.DiskUsage.ExclusiveBytesUsed
		quotasSize += imageStats.DiskUsage.QuotaSizeBytes
	}

	return imagesSize, quotasSize, nil
}

func (c UsageCollector) getImageIDs() ([]string, error) {
	ids := []string{}

	imagesDir := filepath.Join(c.config.Store, "images")
	entries, err := os.ReadDir(imagesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read images directory %q: %v", imagesDir, err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		ids = append(ids, entry.Name())
	}

	return ids, nil
}

type stats struct {
	DiskUsage struct {
		ExclusiveBytesUsed int64 `json:"exclusive_bytes_used"`
		QuotaSizeBytes     int64 `json:"quota_size_bytes"`
	} `json:"disk_usage"`
}

func (c UsageCollector) getImageStats(id string) (stats, error) {
	output, err := c.runner.Run(context.Background(), c.config.GrootFSBin, "--config", c.configPath, "stats", id)
	if err != nil {
		return stats{}, fmt.Errorf("failed to run `grootfs --config %s stat %s`: %v", c.configPath, id, err)
	}

	var usage stats
	err = json.Unmarshal(output, &usage)
	if err != nil {
		return stats{}, fmt.Errorf("failed to unmarshal grootfs stats output: %v", err)
	}

	return usage, nil
}

type metaData struct {
	Size int64 `json:"Size"`
}

func (c UsageCollector) volumeSizeFromMeta(id string) (int64, error) {
	metaFile := filepath.Join(c.config.Store, metaDirectory, "volume-"+id)
	contents, err := os.ReadFile(metaFile)
	if err != nil {
		return 0, fmt.Errorf("cannot read meta file %q: %v", metaFile, err)
	}
	var sizedata metaData
	err = json.Unmarshal(contents, &sizedata)
	if err != nil {
		return 0, fmt.Errorf("failed to unmarshal meta file json: %v", err)
	}
	return sizedata.Size, nil
}

func (c UsageCollector) getVolumes() ([]string, []string, error) {
	usedVolumes, err := c.getUsedVolumes()
	if err != nil {
		return nil, nil, err
	}

	unusedVolumes, err := c.getUnusedVolumes(usedVolumes)
	if err != nil {
		return nil, nil, err
	}

	return usedVolumes, unusedVolumes, nil
}

func (c UsageCollector) getUsedVolumes() ([]string, error) {
	volumes := []string{}
	dependenciesDir := filepath.Join(c.config.Store, metaDirectory, dependenciesDirectory)
	depsInfos, err := os.ReadDir(dependenciesDir)
	if err != nil {
		return nil, fmt.Errorf("error reading dependencies dir %q: %v", dependenciesDir, err)
	}

	for _, di := range depsInfos {
		depFilePath := filepath.Join(dependenciesDir, di.Name())
		depBytes, err := os.ReadFile(depFilePath)
		if err != nil {
			return nil, fmt.Errorf("error reading dependencies file %q: %v", depFilePath, err)
		}

		var volIds []string
		if err := json.Unmarshal(depBytes, &volIds); err != nil {
			return nil, fmt.Errorf("error unmarshaling dependencies file %q content %q: %v", depFilePath, string(depBytes), err)
		}

		volumes = append(volumes, volIds...)
	}
	return volumes, nil
}

func (c UsageCollector) getUnusedVolumes(usedVolumes []string) ([]string, error) {
	volumesMap := map[string]struct{}{}
	volumesDir := filepath.Join(c.config.Store, volumesDirectory)
	volumeEntries, err := os.ReadDir(volumesDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read volumes dir %q: %v", volumesDir, err)
	}

	for _, v := range volumeEntries {
		volumesMap[v.Name()] = struct{}{}
	}

	for _, usedVolume := range usedVolumes {
		delete(volumesMap, usedVolume)
	}

	volumes := []string{}
	for v := range volumesMap {
		volumes = append(volumes, v)
	}

	return volumes, nil
}

func (c UsageCollector) volumesSizeFromMeta(volumes []string) (int64, error) {
	return volumesSize(volumes, func(volumeID string) (int64, error) {
		volSize, err := c.volumeSizeFromMeta(volumeID)
		if err != nil {
			return 0, fmt.Errorf("failed to get size of volume %q: %v", volumeID, err)
		}
		return volSize, err
	})
}

func (c UsageCollector) volumesSizeOnDisk(volumes []string) (int64, error) {
	return volumesSize(volumes, func(volumeID string) (int64, error) {
		path := filepath.Join(c.config.Store, volumesDirectory, volumeID)
		volSize, err := c.sizeOnDisk(path, false)
		if err != nil {
			return 0, fmt.Errorf("failed to get size of volume %q: %v", path, err)
		}
		return volSize, err
	})
}

func (c UsageCollector) sizeOnDisk(path string, apparentSize bool) (int64, error) {
	duArgs := []string{"-B1"}
	if apparentSize {
		duArgs = append(duArgs, "--apparent-size")
	}
	duArgs = append(duArgs, "-s", path)

	output, err := c.runner.Run(context.Background(), "du", duArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to run `du %s`: %v", strings.Join(duArgs, " "), err)
	}
	parts := strings.Split(string(output), "\t")
	if len(parts) < 2 {
		return 0, fmt.Errorf("unexpected `du` output %q", output)
	}
	size, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse int %q: %v", parts[0], err)
	}
	return size, nil
}

func volumesSize(volumes []string, calcSize func(string) (int64, error)) (int64, error) {
	var size int64

	for _, volumeID := range volumes {
		volSize, err := calcSize(volumeID)
		if err != nil {
			return 0, err
		}
		size += volSize
	}

	return size, nil
}
