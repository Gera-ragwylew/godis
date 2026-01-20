package sender

import (
	"context"
	"fmt"
	worker "godis/internal"
	"godis/internal/pipeline"
	"reflect"
	"strings"
)

type Sender struct {
	worker.BaseEntity
	worker.AudioEntity
}

const (
	sampleRate = 16000
	channels   = 1
	bitrate    = 64000
)

func New(config any) *Sender {
	s := &Sender{}
	v := reflect.ValueOf(config)

	if v.Kind() == reflect.Pointer {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		fmt.Println("Sender.New() expects structure, received ", v.Kind())
		return s
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		fieldName := strings.ToLower(field.Name)
		switch fieldName {
		case "ip":
			if fieldValue.Kind() == reflect.Slice {
				ipSlice := make([]string, fieldValue.Len())
				for j := 0; j < fieldValue.Len(); j++ {
					ipSlice[j] = fieldValue.Index(j).String()
				}
				s.Ip = ipSlice
			} else if fieldValue.Kind() == reflect.String {
				s.Ip = []string{fieldValue.String()}
			}
		case "port":
			s.Port = fieldValue.String()
		case "samplerate":
			s.SampleRate = fieldValue.Float()
		case "framesize":
			s.FrameSize = int(fieldValue.Int())
		}
	}

	return s
}

func (s *Sender) Start(ctx context.Context) error {
	p := pipeline.NewPipeline(ctx)

	p.AddStage(&pipeline.GenericWrapper[any, []float32]{
		Stage: s.RecordMicrophoneStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[[]float32, []int16]{
		Stage: s.ConvertToPCMStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[[]int16, []byte]{
		Stage: s.EncodeOpusStage(),
	})

	p.AddStage(&pipeline.GenericWrapper[[]byte, any]{
		Stage: s.SendUDPStage(),
	})

	p.Run()

	return nil
}
