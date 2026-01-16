package sender

import (
	"context"
	"encoding/binary"
	"fmt"
	worker "godis/internal"
	"log"
	"math"
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
	sequence   uint32
	seqMutex   sync.Mutex
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

	s.bufferPool = sync.Pool{
		New: func() interface{} {
			return make([]byte, 4+s.FrameSize*2)
		},
	}
	s.sequence = 1

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

	for _, addr := range s.Ip {
		go func() {
			err := s.sendPackets(ctx, packetChan, addr+":"+s.Port)
			if err != nil {
				fmt.Println("Send packets error: %v", err)
			}
		}()
	}

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

			s.seqMutex.Lock()
			currentSeq := s.sequence
			s.sequence++
			if s.sequence == 0 { // Обработка overflow uint32
				s.sequence = 1
			}
			s.seqMutex.Unlock()

			packet := s.bufferPool.Get().([]byte)

			binary.LittleEndian.PutUint32(packet[0:4], currentSeq)
			s.convertToPCM(in, packet[4:])

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
			pcm = int16(math.Round(float64(sample * 32767.0)))
		}

		if i*2+1 < len(packet) {
			packet[i*2] = byte(pcm)
			packet[i*2+1] = byte(pcm >> 8)
		} else {
			log.Printf("Buffer overflow: i=%d, packet len=%d", i, len(packet))
			break
		}
	}
}

func (s *Sender) sendPackets(ctx context.Context, packetChan <-chan []byte, addr string) error {
	// addr := net.JoinHostPort(s.Ip, s.Port)
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
