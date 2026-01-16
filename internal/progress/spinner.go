package progress

import (
	"fmt"
	"sync"
	"time"
)

type Spinner struct {
	message string
	done    chan struct{}
	wg      sync.WaitGroup
	active  bool
	mu      sync.Mutex
}

var spinnerChars = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func NewSpinner(message string) *Spinner {
	return &Spinner{
		message: message,
		done:    make(chan struct{}),
	}
}

func (s *Spinner) Start() {
	s.mu.Lock()
	if s.active {
		s.mu.Unlock()
		return
	}
	s.active = true
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		i := 0
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-s.done:
				fmt.Print("\r\033[K") // Clear line using ANSI escape code
				return
			case <-ticker.C:
				fmt.Printf("\r%s %s", spinnerChars[i%len(spinnerChars)], s.message)
				i++
			}
		}
	}()
}

func (s *Spinner) Stop() {
	s.mu.Lock()
	if !s.active {
		s.mu.Unlock()
		return
	}
	s.active = false
	s.mu.Unlock()

	close(s.done)
	s.wg.Wait()
}
