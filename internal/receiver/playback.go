package receiver

import (
	"context"
	"fmt"
	"godis/internal/utils/pipeline"

	"github.com/gordonklaus/portaudio"
)

type playbackStage struct {
	receiver *Receiver
}

func (r *Receiver) PlaybackStage() pipeline.TypedStage[[][]int16, any] {
	return &playbackStage{receiver: r}
}

func (r *playbackStage) Process(ctx context.Context, in <-chan [][]int16) (<-chan any, error) {
	return r.receiver.playback(ctx, in)
}

func (r *Receiver) playback(_ context.Context, _ <-chan [][]int16) (<-chan any, error) {
	stream, err := portaudio.OpenDefaultStream(0, 1, r.SampleRate, r.FrameSize, r.audioCallback)
	if err != nil {
		return nil, fmt.Errorf("Stream creation error: %v\n", err)
	}

	if err := stream.Start(); err != nil {
		stream.Stop()
		stream.Close()
		return nil, fmt.Errorf("Stream start error: %w", err)
	}
	return nil, nil
}

func (r *Receiver) audioCallback(out []int16) {
	for i := range out {
		out[i] = 0
	}

	activeSSRCs := r.audioBuffer.GetActiveStreams()

	for _, ssrc := range activeSSRCs {
		packet, err := r.audioBuffer.GetPacket(ssrc)
		if err != nil || packet == nil {
			continue
		}

		temp := make([]int16, len(out))
		copy(temp, packet.PCMData)

		r.mixAudio(out, temp)
	}
}

func (r *Receiver) mixAudio(output, input []int16) {
	limit := min(len(input), len(output))

	for i := range limit {
		mixed := int32(output[i]) + int32(input[i])

		if mixed > 32767 {
			mixed = 32767
		} else if mixed < -32768 {
			mixed = -32768
		}

		output[i] = int16(mixed)
	}
}
