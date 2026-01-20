package receiver

import (
	"context"
	"godis/internal/pipeline"
)

type audioProcessorStage struct {
	receiver *Receiver
}

func (r *Receiver) AudioProcessorStage() pipeline.TypedStage[*opusData, struct{}] {
	return &audioProcessorStage{receiver: r}
}

func (r *audioProcessorStage) Process(ctx context.Context, in <-chan *opusData) (<-chan struct{}, error) {
	return r.receiver.audioProcessor(ctx, in)
}

func (r *Receiver) audioProcessor(ctx context.Context, in <-chan *opusData) (<-chan struct{}, error) {
	out := make(chan struct{}, 1)
	// minBufferBeforeSignal := 3
	jitterBufferSize := 20
	r.audioBuffer = newRingBuffer[*opusData](jitterBufferSize)

	go func() {
		defer close(out)
		for {

			select {
			case <-ctx.Done():
				return

			case od, ok := <-in:
				if !ok {
					return
				}

				r.audioBuffer.Write(od)
			}
		}
	}()
	return out, nil
}
