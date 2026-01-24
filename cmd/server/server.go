package server

import (
	"context"
	worker "godis/internal"
	"godis/internal/receiver"
	"godis/internal/sender"
	"log"
	"sync"
	"time"

	"github.com/gordonklaus/portaudio"
)

type Server struct {
	workers []worker.Worker
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewServer(config any) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		workers: []worker.Worker{
			receiver.New(config),
			sender.New(config),
		},
		ctx:    ctx,
		cancel: cancel,
	}
}

func (s *Server) Start() {
	portaudio.Initialize()

	for _, worker := range s.workers {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			if err := worker.Start(s.ctx); err != nil {
				log.Println("Worker error (continuing): ", err)
			}
		}()
	}
}

func (s *Server) Stop() {
	log.Println("Shutting down server...")
	defer portaudio.Terminate()
	s.cancel()

	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("All workers stopped gracefully")
	case <-time.After(5 * time.Second):
		log.Println("Warning: some workers didn't stop in time")
	}

	time.Sleep(time.Second * 2) // for portaudio.Terminate()
}
