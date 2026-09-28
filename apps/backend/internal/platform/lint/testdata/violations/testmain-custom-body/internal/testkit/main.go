package testkit

import "testing"

type MainOption func()

func Main(m *testing.M, _ ...MainOption) {
	m.Run()
}
