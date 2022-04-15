package disk_test

import (
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"thresholder/disk"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

var _ = Describe("SysFS", func() {
	It("uses statfs(2) to get the FS stats", func() {
		fs := disk.NewSysFS()
		stat, err := fs.Stat("/")

		Expect(err).NotTo(HaveOccurred())
		Expect(stat.AvailableBlocks).To(BeMoreOrLess(dfAvailBlocks("/", stat.BlockSize)))
	})
})

func dfAvailBlocks(path string, blockSize int64) int64 {
	cmd := exec.Command("df", path, "--output=avail", fmt.Sprintf("--block-size=%d", blockSize))
	output, err := cmd.Output()
	Expect(err).NotTo(HaveOccurred())

	blocksStr := regexp.MustCompile(`\d+`).FindString(string(output))
	blocks, err := strconv.ParseInt(blocksStr, 10, 64)
	Expect(err).NotTo(HaveOccurred())

	return blocks
}

func BeMoreOrLess(n int64) types.GomegaMatcher {
	return BeNumerically("~", n, float64(n)*0.001)
}
