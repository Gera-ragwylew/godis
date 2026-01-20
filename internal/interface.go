package worker

import (
	"context"
)

type BaseEntity struct {
	Ip   []string
	Port string
}

type AudioEntity struct {
	SampleRate float64
	FrameSize  int
}

type Worker interface {
	Start(ctx context.Context) error
}
