package pipeline

import (
	"context"
	"log"
)

type TypedStage[T any, U any] interface {
	Process(ctx context.Context, in <-chan T) (<-chan U, error)
}

type GenericWrapper[T any, U any] struct {
	Stage TypedStage[T, U]
}

func (w *GenericWrapper[T, U]) Run(ctx context.Context, in <-chan any) <-chan any {
	out := make(chan any, 20)

	typedIn := make(chan T, 20)
	go func() {
		defer close(typedIn)
		for item := range in {
			if data, ok := item.(T); ok {
				typedIn <- data
			}
		}
	}()

	resultChan, err := w.Stage.Process(ctx, typedIn)
	if err != nil {
		log.Printf("Stage failed: %v", err)
		close(out)
		return out
	}

	go func() {
		defer close(out)
		for data := range resultChan {
			select {
			case <-ctx.Done():
				return
			case out <- data:
			}
		}
	}()

	return out
}

type Stage interface {
	Run(ctx context.Context, in <-chan any) <-chan any
}

type Pipeline struct {
	ctx    context.Context
	stages []Stage
}

func NewPipeline(ctx context.Context) *Pipeline {
	return &Pipeline{
		ctx:    ctx,
		stages: make([]Stage, 0),
	}
}

func (p *Pipeline) AddStage(stage Stage) *Pipeline {
	p.stages = append(p.stages, stage)
	return p
}

func (p *Pipeline) Run() {
	if len(p.stages) == 0 {
		return
	}

	startChan := make(chan any, 20)
	close(startChan)

	var outputChan <-chan any = p.stages[0].Run(p.ctx, startChan)
	for i := 1; i < len(p.stages); i++ {
		outputChan = p.stages[i].Run(p.ctx, outputChan)
	}
}
