package temporal_data_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestTemporalDataSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "TemporalData Suite")
}
