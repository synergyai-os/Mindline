package embedding

import (
	"context"
	"errors"
	"math"
)

type Port interface {
	ModelID() string
	Embed(context.Context, []string) ([][]float64, error)
}

// RetrievalPort lets a provider adapter distinguish asymmetric query and
// document inputs without leaking model-specific prompt syntax into ranking.
// Backends may fall back to Port for providers that do not implement it.
type RetrievalPort interface {
	Port
	EmbedQuery(context.Context, string) ([]float64, error)
	EmbedDocuments(context.Context, []string) ([][]float64, error)
}

func Cosine(left, right []float64) (float64, error) {
	if len(left) == 0 || len(left) != len(right) {
		return 0, errors.New("embedding dimensions do not match")
	}
	dot := 0.0
	leftNorm := 0.0
	rightNorm := 0.0
	for index := range left {
		if math.IsNaN(left[index]) || math.IsInf(left[index], 0) ||
			math.IsNaN(right[index]) || math.IsInf(right[index], 0) {
			return 0, errors.New("embedding contains a non-finite value")
		}
		dot += left[index] * right[index]
		leftNorm += left[index] * left[index]
		rightNorm += right[index] * right[index]
	}
	if leftNorm == 0 || rightNorm == 0 {
		return 0, errors.New("embedding has zero magnitude")
	}
	return dot / (math.Sqrt(leftNorm) * math.Sqrt(rightNorm)), nil
}
