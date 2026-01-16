package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"net"
	"time"
)

func main() {
	conn, err := net.Dial("udp", "localhost:4899")
	if err != nil {
		panic(err)
	}
	defer conn.Close()

	sequence := uint32(0)
	frameSize := 256

	for i := 0; i < 50; i++ {
		packet := make([]byte, 4+frameSize*2)

		binary.LittleEndian.PutUint32(packet[0:4], sequence)

		for j := 0; j < frameSize; j++ {
			t := float64(j) / 16000.0
			sample := int16(math.Sin(2*math.Pi*440*t) * 32767.0 * 0.5)

			idx := 4 + j*2
			packet[idx] = byte(sample)
			packet[idx+1] = byte(sample >> 8)
		}

		_, err := conn.Write(packet)
		if err != nil {
			fmt.Printf("Send error: %v\n", err)
		} else {
			fmt.Printf("Sent packet seq=%d\n", sequence)
		}

		sequence++
		time.Sleep(20 * time.Millisecond)
	}
}
