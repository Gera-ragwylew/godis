package receiver

import (
	"context"
	"fmt"
	worker "godis/internal"
	"godis/internal/pipeline"
	"reflect"
	"strings"
)

const (
	HeaderSize          = 4
	jitterBufferPackets = 3
)

type Receiver struct {
	worker.BaseEntity
	worker.AudioEntity
	audioBuffer *RingBuffer[*opusData]
}

type PacketData struct {
	Addr     string
	Sequence uint32
	Audio    []byte
}

func New(config interface{}) *Receiver {
	r := &Receiver{}
	v := reflect.ValueOf(config)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		fmt.Println("Receiver.New() expects structure, received ", v.Kind())
		return r
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		fieldName := strings.ToLower(field.Name)
		switch fieldName {
		case "port":
			r.Port = fieldValue.String()
		case "samplerate":
			r.SampleRate = fieldValue.Float()
		case "framesize":
			r.FrameSize = int(fieldValue.Int())
		}
	}

	return r
}

func (r *Receiver) Start(ctx context.Context) error {
	p := pipeline.NewPipeline(ctx)

	p.AddStage(&pipeline.GenericWrapper[any, *PacketData]{
		Stage: r.ReceiveUDPStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[*PacketData, *opusData]{
		Stage: r.DecodeOpusStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[*opusData, struct{}]{
		Stage: r.AudioProcessorStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[struct{}, any]{
		Stage: r.PlaybackStage(),
	})

	p.Run()

	return nil

}
