package e2e

import (
	"os"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "streamline-opds end-to-end")
}

var shared *instance

var _ = BeforeSuite(func() {
	Expect(os.Getenv("STREAMLINE_BIN")).NotTo(BeEmpty(),
		"STREAMLINE_BIN must name a Streamline binary")
	shared = startInstance(instanceOptions{enabled: true})
})
