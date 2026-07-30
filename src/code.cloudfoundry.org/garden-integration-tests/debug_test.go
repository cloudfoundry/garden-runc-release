package garden_integration_tests_test

import (
	"encoding/json"
	"fmt"
	"runtime"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type Debug struct {
	MemStats runtime.MemStats `json:"memstats"`
}

func loadDebug() Debug {
	response, err := httpGet(fmt.Sprintf("http://%s:%s/debug/vars", gardenHost, gardenDebugPort))
	Expect(err).NotTo(HaveOccurred())

	debug := Debug{}
	Expect(json.Unmarshal([]byte(response), &debug)).To(Succeed())

	return debug
}

var _ = Describe("Debug", func() {

	BeforeEach(func() {
		if runtime.GOOS == "windows" {
			Skip("skip for windows")
		}
	})

	Describe("Memory", func() {
		It("should have non-zero allocated memory", func() {
			debug := loadDebug()
			Expect(debug.MemStats.Alloc).NotTo(BeZero())
		})
	})
})
