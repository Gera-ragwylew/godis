package receiver

import (
	"context"
	"fmt"
	worker "godis/internal"
	"godis/internal/utils/jitterbuffer"
	"godis/internal/utils/pipeline"
	"godis/internal/utils/rtp"
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
	audioBuffer *jitterbuffer.JitterBuffer
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

	p.AddStage(&pipeline.GenericWrapper[any, []byte]{
		Stage: r.ReceiveUDPStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[[]byte, *rtputils.RTPPacket]{
		Stage: r.UnpackRTPStage(),
	})

	// p.AddStage(&pipeline.GenericWrapper[*rtputils.RTPPacket, struct{}]{
	// 	Stage: r.AudioProcessorStage(),
	// })

	// p.AddStage(&pipeline.GenericWrapper[struct{}, [][]int16]{
	// 	Stage: r.DecodeOpusStage(),
	// })

	p.AddStage(&pipeline.GenericWrapper[[][]int16, any]{
		Stage: r.PlaybackStage(),
	})

	p.Run()

	return nil

}
