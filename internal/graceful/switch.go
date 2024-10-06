package graceful

import (
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ErrTimeout describes tasks that failed to finish in time.
type ErrTimeout []string

// Error returns an error message.
func (e ErrTimeout) Error() string {
	return "shutdown timeout, tasks left: " + strings.Join(e, ", ")
}

// Switch is graceful shutdown handler.
//
// Please use NewSwitch to create an instance.
type Switch struct {
	Signals []os.Signal  // Defaults to syscall.SIGTERM, syscall.SIGINT when empty.
	Done    chan<- error // Aborts app exit and closes or sends ErrTimeout over the channel, calls os.Exit if empty.

	mu     sync.Mutex
	sig    chan os.Signal
	done   <-chan error
	closed bool
	tasks  map[string]func()
}

// NewSwitch creates shutdown handler that triggers on any of provided OS signals
// and allows registered tasks to take up to provided timeout.
//
// When switch is triggered, tasks are invoked concurrently.
func NewSwitch(timeout time.Duration, options ...func(s *Switch)) *Switch {
	s := &Switch{}

	for _, option := range options {
		option(s)
	}

	if s.Signals == nil {
		s.Signals = []os.Signal{syscall.SIGTERM, syscall.SIGINT}
	}

	done := make(chan error, 1)

	sh := &Switch{
		sig:   make(chan os.Signal, 1),
		tasks: make(map[string]func()),
		done:  done,
	}

	signal.Notify(sh.sig, s.Signals...)

	go sh.waitForSignal(done, timeout)

	if s.Done != nil {
		go func() {
			err, ok := <-done
			if !ok {
				close(s.Done)
			} else {
				s.Done <- err
				close(s.Done)
			}
		}()
	} else {
		go func() {
			err := <-done
			if err != nil {
				println(err.Error())
				os.Exit(1)
			} else {
				os.Exit(0)
			}
		}()
	}

	return sh
}

func (s *Switch) waitForSignal(done chan error, timeout time.Duration) {
	<-s.sig

	signal.Stop(s.sig)
	s.Shutdown()

	s.mu.Lock()

	sem := make(chan struct{}, len(s.tasks))
	active := make(map[string]struct{})

	for name, fn := range s.tasks {
		fn := fn
		name := name

		sem <- struct{}{}

		active[name] = struct{}{}

		go func() {
			defer func() {
				s.mu.Lock()
				delete(active, name)
				s.mu.Unlock()
				<-sem
			}()

			fn()
		}()
	}
	s.mu.Unlock()

	deadline := time.After(timeout)

	for i := 0; i < cap(sem); i++ {
		select {
		case sem <- struct{}{}:
		case <-deadline:
			var err ErrTimeout

			s.mu.Lock()

			for k := range active {
				err = append(err, k)
				sort.Strings(err)
			}

			s.mu.Unlock()

			done <- err
			close(done)

			return
		}
	}

	close(done)
}

// OnShutdown adds a named task to run on shutdown.
func (s *Switch) OnShutdown(name string, fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.tasks == nil {
		panic("graceful: Switch is not initialized, did you call NewSwitch?")
	}

	s.tasks[name] = fn
}

// Shutdown triggers the switch and stops listening to OS signals.
func (s *Switch) Shutdown() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.closed {
		s.closed = true
		close(s.sig)
	}
}

func (s *Switch) Wait() {
	<-s.done
}
