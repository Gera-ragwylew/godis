package sender

import (
	"context"
	"godis/internal/pipeline"
	"log"
)

type convertToPCMStage struct {
	sender *Sender
}

func (s *Sender) ConvertToPCMStage() pipeline.TypedStage[[]float32, []int16] {
	return &convertToPCMStage{sender: s}
}

func (c *convertToPCMStage) Process(ctx context.Context, in <-chan []float32) (<-chan []int16, error) {
	return c.sender.convertToPCM(ctx, in)
}

func (s *Sender) convertToPCM(ctx context.Context, in <-chan []float32) (chan []int16, error) {
	out := make(chan []int16, 20)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				log.Println("Stop converting by context...")
				return
			case data, ok := <-in:
				if !ok {
					return
				}

				pcmData := make([]int16, len(data))

				for i := range data {
					sample := data[i]
					var pcm int16
					if sample >= 1.0 {
						pcm = 32767
					} else if sample <= -1.0 {
						pcm = -32767
					} else {
						pcm = int16(sample * 32767.0)
					}
					pcmData[i] = pcm
				}
				select {
				case <-ctx.Done():
					return
				case out <- pcmData:
				}
			}
		}
	}()
	return out, nil
}
