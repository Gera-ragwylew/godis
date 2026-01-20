package sender

import (
	"context"
	"godis/internal/pipeline"
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
	encoder.SetBitrate(bitrate)

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				log.Println("Stop encoding by context...")
				return
			case data, ok := <-in:
				if !ok {
					return
				}

				encoded := make([]byte, 1275) // ???

				n, err := encoder.Encode(data, encoded)
				if err != nil {
					log.Println(err)
				}

				select {
				case <-ctx.Done():
					return
				case out <- encoded[:n]:
				}
			}
		}
	}()

	return out, nil
}
