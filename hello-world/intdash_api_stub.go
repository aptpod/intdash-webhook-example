package main

import (
	"context"
	"math/rand"
)

type IntdashAPIStub struct{}

// FetchFloat64DataPoints generates float64 data points randomly from the normal distribution (mean = 100, stddev = 15).
func (s *IntdashAPIStub) FetchFloat64DataPoints(ctx context.Context, measurementUUID string) ([]float64, error) {
	r := rand.New(rand.NewSource(0))
	res := make([]float64, 1000)
	for i := range res {
		res[i] = r.NormFloat64()*15 + 100
	}
	return res, nil
}
