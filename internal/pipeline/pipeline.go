package pipeline

import (
	"context"
	"sync"
)

type Pipeline struct {
	stages []func(<-chan interface{}) <-chan interface{}
	ctx    context.Context
	cancel context.CancelFunc
}

func NewPipeline(ctx context.Context) *Pipeline {
	ctx, cancel := context.WithCancel(ctx)
	return &Pipeline{
		ctx:    ctx,
		cancel: cancel,
	}
}

func (p *Pipeline) AddStage(process func(interface{}) interface{}, workers int) *Pipeline {
	stage := func(in <-chan interface{}) <-chan interface{} {
		out := make(chan interface{})

		var wg sync.WaitGroup
		wg.Add(workers)

		for i := 0; i < workers; i++ {
			go func() {
				defer wg.Done()
				for {
					select {
					case <-p.ctx.Done():
						return
					case item, ok := <-in:
						if !ok {
							return
						}
						result := process(item)

						select {
						case <-p.ctx.Done():
							return
						case out <- result:
						}
					}
				}
			}()
		}

		go func() {
			wg.Wait()
			close(out)
		}()

		return out
	}

	p.stages = append(p.stages, stage)
	return p
}

func (p *Pipeline) Run(input <-chan interface{}) <-chan interface{} {
	current := input
	for _, stage := range p.stages {
		current = stage(current)
	}
	return current
}

func (p *Pipeline) Stop() {
	p.cancel()
}
