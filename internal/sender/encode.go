package sender

import (
	"context"
	"godis/internal/utils/pipeline"
	"log"

	"github.com/hraban/opus"
)

type encodeOpusStage struct {
	sender *Sender
}

func (s *Sender) EncodeOpusStage() pipeline.TypedStage[[]int16, []byte] {
	return &encodeOpusStage{sender: s}
}

func (c *encodeOpusStage) Process(ctx context.Context, in <-chan []int16) (<-chan []byte, error) {
	return c.sender.encodeOpus(ctx, in)
}

func (s *Sender) encodeOpus(ctx context.Context, in <-chan []int16) (chan []byte, error) {
	out := make(chan []byte, 20)
	encoder, err := opus.NewEncoder(sampleRate, channels, opus.AppVoIP)
	if err != nil {
		panic(err)
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case data, ok := <-in:
				if !ok {
					return
				}

				if len(data) == 0 {
					log.Println("Empty audio frame, skipping")
					continue
				}

				encoded := make([]byte, 2000)

				n, err := encoder.Encode(data, encoded)
				if err != nil {
					log.Println(err)
				}

				encodedCopy := make([]byte, n)
				copy(encodedCopy, encoded[:n])

				select {
				case <-ctx.Done():
					return
				case out <- encodedCopy:
				}
			}
		}
	}()

	return out, nil
}
