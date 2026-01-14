package sender

import (
	"context"
	"fmt"
	worker "godis/internal"
	"log"
	"net"
	"reflect"
	"strings"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type Sender struct {
	worker.BaseEntity
	worker.AudioEntity
	conn       net.Conn
	bufferPool sync.Pool
}

func New(config interface{}) *Sender {
	s := &Sender{}
	v := reflect.ValueOf(config)

	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}

	if v.Kind() != reflect.Struct {
		fmt.Println("Sender.New() expects structure, received %v", v.Kind())
		return s
	}

	t := v.Type()
	for i := 0; i < v.NumField(); i++ {
		field := t.Field(i)
		fieldValue := v.Field(i)

		fieldName := strings.ToLower(field.Name)
		switch fieldName {
		case "ip":
			s.Ip = fieldValue.String()
		case "port":
			s.Port = fieldValue.String()
		case "samplerate":
			s.SampleRate = fieldValue.Float()
		case "framesize":
			s.FrameSize = int(fieldValue.Int())
		}
	}

	s.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, s.FrameSize*2)
		},
	}

	return s
}

func (s *Sender) Start(ctx context.Context, wg *sync.WaitGroup) error {
	in := make([]float32, s.FrameSize)
	stream, err := portaudio.OpenDefaultStream(1, 0, s.SampleRate, s.FrameSize, in)
	if err != nil {
		return fmt.Errorf("Stream creation error: %v\n", err)
	}
	defer stream.Close()

	packetChan := make(chan []byte, 20)
	defer close(packetChan)

	go func() {
		err := s.sendPackets(ctx, packetChan)
		if err != nil {
			fmt.Println("Send packets error: %v", err)
		}
	}()

	err = s.recordLoop(ctx, stream, packetChan, in)
	if err != nil {
		fmt.Println("%v", err)
	}
	return nil
}

func (s *Sender) recordLoop(ctx context.Context, stream *portaudio.Stream, packetChan chan<- []byte, in []float32) error {
	if err := stream.Start(); err != nil {
		return fmt.Errorf("Stream start error: %w", err)
	}
	defer stream.Stop()

	frameDuration := time.Duration(float64(s.FrameSize)/s.SampleRate*1000) * time.Millisecond

	for {
		select {
		case <-ctx.Done():
			log.Println("Sender stopped...")
			return nil
		default:
			if err := stream.Read(); err != nil {
				log.Printf("Stream reading error: %v\n", err)
				continue
			}

			packet := s.bufferPool.Get().([]byte)

			s.convertToPCM(in, packet)

			select {
			case packetChan <- packet:
			default:
				s.bufferPool.Put(packet)
				fmt.Println("Packet pass (buffer full)")
			}

			time.Sleep(frameDuration)
		}
	}
}

func (s *Sender) convertToPCM(in []float32, packet []byte) {
	for i := range in {
		sample := in[i]

		var pcm int16
		if sample >= 1.0 {
			pcm = 32767
		} else if sample <= -1.0 {
			pcm = -32767
		} else {
			pcm = int16(sample * 32767.0)
		}

		packet[i*2] = byte(pcm)
		packet[i*2+1] = byte(pcm >> 8)
	}
}

func (s *Sender) sendPackets(ctx context.Context, packetChan <-chan []byte) error {
	addr := net.JoinHostPort(s.Ip, s.Port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return fmt.Errorf("UDP error: %v\n", err)
	}
	defer conn.Close()
	s.conn = conn

	for {
		select {
		case <-ctx.Done():
			return nil
		case packet := <-packetChan:
			if _, err := s.conn.Write(packet); err != nil {
				fmt.Printf("Write packet error: %v\n", err)
			}
			s.bufferPool.Put(packet)
		}
	}
}
