package citest

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestCITest(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CITest Suite")
}
