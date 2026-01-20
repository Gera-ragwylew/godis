package receiver

import (
	"context"
	"fmt"
	"godis/internal/pipeline"
	"log"

	"github.com/hraban/opus"
)

type decodeOpusStage struct {
	receiver *Receiver
}

func (r *Receiver) DecodeOpusStage() pipeline.TypedStage[*PacketData, *opusData] {
	return &decodeOpusStage{receiver: r}
}

func (r *decodeOpusStage) Process(ctx context.Context, in <-chan *PacketData) (<-chan *opusData, error) {
	return r.receiver.decodeOpus(ctx, in)
}

type opusData struct {
	Addr     string
	Sequence uint32
	Audio    []int16
}

func (r *Receiver) decodeOpus(ctx context.Context, in <-chan *PacketData) (<-chan *opusData, error) {
	out := make(chan *opusData, 20)

	decoder, err := opus.NewDecoder(int(r.SampleRate), 1)
	if err != nil {
		return nil, fmt.Errorf("Failed to create decoder: %v", err)
	}

	go func() {
		defer close(out)

		for {
			select {
			case <-ctx.Done():
				return
			case pd, ok := <-in:
				if !ok {
					return
				}

				buffer := make([]int16, r.FrameSize)
				decodedSamples, err := decoder.Decode(pd.Audio, buffer)
				if err != nil {
					log.Println("Decode error: ", err)
					continue
				}
				log.Printf("Decoded %d samples from %d bytes\n", decodedSamples, len(pd.Audio))

				od := &opusData{
					Addr:     pd.Addr,
					Sequence: pd.Sequence,
					Audio:    make([]int16, decodedSamples),
				}
				if decodedSamples > 0 && decodedSamples <= len(buffer) {
					copy(od.Audio, buffer[:decodedSamples])
				}

				select {
				case <-ctx.Done():
					return
				case out <- od:
				}
			}
		}
	}()

	return out, nil
}
