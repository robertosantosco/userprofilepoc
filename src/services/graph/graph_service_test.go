package graph_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestGraphServiceSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GraphService Suite")
}
