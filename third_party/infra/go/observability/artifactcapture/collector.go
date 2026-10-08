// Package artifactcapture owns explicitly enabled raw diagnostic buffering and
// asynchronous artifact effects. It does not sanitize data or authorize capture.
package artifactcapture

import (
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Writer is called only by the collector's background owner.
type Writer interface {
	WriteArtifact(name string, data []byte) error
}

// Config bounds queued jobs, active streams, each artifact, and total retained
// payload/name/key bytes across queued, buffered, and currently written jobs.
type Config struct {
	QueueJobs        int
	MaxBytes         int64
	MaxStreams       int
	MaxArtifactBytes int
	IdleTimeout      time.Duration
}

type chunk struct {
	key, name string
	bytes     []byte
	complete  bool
	charge    int64
	barrier   chan error
}
type stream struct {
	name      string
	bytes     []byte
	charge    int64
	touched   time.Time
	oversized bool
}

// Collector has one goroutine per explicitly opened owner, not per stream.
// TryAppend never waits for a queue, mutex, file, flush or retention operation.
// Close is an owner shutdown operation and waits for accepted work and disk I/O.
type Collector struct {
	config   Config
	writer   Writer
	jobs     chan chunk
	stop     chan struct{}
	done     chan struct{}
	once     sync.Once
	state    atomic.Uint64
	retained atomic.Int64
	dropped  atomic.Uint64
	failures atomic.Uint64
	poisoned atomic.Bool
	err      error // read only after done closes
}

var ErrCaptureLoss = errors.New("artifact capture lost data; recreate the owner")

const closingBit uint64 = 1 << 63

func NewCollector(writer Writer, config Config) (*Collector, error) {
	if writer == nil || config.QueueJobs <= 0 || config.MaxBytes <= 0 || config.MaxStreams <= 0 || config.MaxArtifactBytes <= 0 || config.IdleTimeout < 0 {
		return nil, errors.New("artifact capture requires writer and positive byte/job/stream limits")
	}
	c := &Collector{writer: writer, config: config, jobs: make(chan chunk, config.QueueJobs), stop: make(chan struct{}), done: make(chan struct{})}
	go c.run()
	return c, nil
}

// TryAppend copies accepted data; caller slices can be reused after return.
// key identifies one stream and name is a single artifact filename, fixed by its
// first accepted chunk. delimiter is retained verbatim after each payload.
// Any known queue/budget/stream loss poisons the owner, discarding all pending
// artifacts instead of writing a misleading prefix. Recreate the owner to resume.
// Previously completed files are unaffected; raw capture remains explicit.
func (c *Collector) TryAppend(key, name string, payload, delimiter []byte, complete bool) bool {
	if c == nil {
		return false
	}
	for {
		state := c.state.Load()
		if state&closingBit != 0 {
			return false
		}
		if c.state.CompareAndSwap(state, state+1) {
			break
		}
	}
	defer c.state.Add(^uint64(0))
	if c.poisoned.Load() {
		c.dropped.Add(1)
		return false
	}
	if key == "" || len(key) > 512 || !validName(name) || len(payload) > c.config.MaxArtifactBytes || len(delimiter) > c.config.MaxArtifactBytes-len(payload) {
		c.drop()
		return false
	}
	charge := int64(len(key) + len(name) + len(payload) + len(delimiter))
	for {
		retained := c.retained.Load()
		if charge > c.config.MaxBytes-retained {
			c.drop()
			return false
		}
		if c.retained.CompareAndSwap(retained, retained+charge) {
			break
		}
	}
	// This newly allocated private buffer cannot alias payload, delimiter, or
	// gjson caller memory; both copies write exclusively into owned storage.
	data := make([]byte, len(payload)+len(delimiter))
	copy(data, payload)
	copy(data[len(payload):], delimiter)
	job := chunk{key: strings.Clone(key), name: strings.Clone(name), bytes: data, complete: complete, charge: charge}
	select {
	case c.jobs <- job:
		return true
	default:
		c.retained.Add(-charge)
		c.drop()
		return false
	}
}
func validName(name string) bool {
	return name != "" && len(name) <= 255 && name != "." && name != ".." && filepath.Base(name) == name && !strings.ContainsAny(name, "/\\\x00")
}
func (c *Collector) drop() { c.poisoned.Store(true); c.dropped.Add(1) }

// Poisoned means a known loss invalidated this owner; create a new owner to resume.
func (c *Collector) Poisoned() bool { return c != nil && c.poisoned.Load() }
func (c *Collector) Dropped() uint64 {
	if c == nil {
		return 0
	}
	return c.dropped.Load()
}
func (c *Collector) Failures() uint64 {
	if c == nil {
		return 0
	}
	return c.failures.Load()
}
func (c *Collector) RetainedBytes() int64 {
	if c == nil {
		return 0
	}
	return c.retained.Load()
}

// Close rejects new submissions, drains accepted jobs and buffered streams, then
// reports background write failures. It is safe to call concurrently/repeatedly.
func (c *Collector) Close() error {
	if c == nil {
		return nil
	}
	c.once.Do(func() {
		for {
			state := c.state.Load()
			if c.state.CompareAndSwap(state, state|closingBit) {
				break
			}
		}
		for c.state.Load()&^closingBit != 0 {
			runtime.Gosched()
		}
		close(c.stop)
	})
	<-c.done
	return c.captureError()
}

func (c *Collector) captureError() error {
	if c.poisoned.Load() {
		return errors.Join(c.err, ErrCaptureLoss)
	}
	return c.err
}

// Flush is an owner/test barrier: drain prior jobs and current buffers while
// leaving the collector open. It may wait for disk and must not run per chunk.
func (c *Collector) Flush() error {
	if c == nil {
		return nil
	}
	for {
		state := c.state.Load()
		if state&closingBit != 0 {
			<-c.done
			return c.captureError()
		}
		if c.state.CompareAndSwap(state, state+1) {
			break
		}
	}
	defer c.state.Add(^uint64(0))
	barrier := make(chan error, 1)
	c.jobs <- chunk{barrier: barrier}
	return <-barrier
}

func (c *Collector) run() {
	defer close(c.done)
	streams := make(map[string]*stream)
	flush := func(key string) {
		s := streams[key]
		if s == nil {
			return
		}
		if !c.poisoned.Load() && !s.oversized && len(s.bytes) > 0 {
			if err := c.writer.WriteArtifact(s.name, s.bytes); err != nil {
				c.failures.Add(1)
				if c.err == nil {
					c.err = fmt.Errorf("write diagnostic artifact: %w", err)
				}
			}
		}
		c.retained.Add(-s.charge)
		delete(streams, key)
	}
	accept := func(job chunk) {
		if job.barrier != nil {
			for key := range streams {
				flush(key)
			}
			job.barrier <- c.captureError()
			return
		}
		if c.poisoned.Load() {
			c.retained.Add(-job.charge)
			for key := range streams {
				flush(key)
			}
			return
		}
		s := streams[job.key]
		if s == nil {
			if len(streams) >= c.config.MaxStreams {
				c.retained.Add(-job.charge)
				c.drop()
				return
			}
			s = &stream{name: job.name, charge: int64(len(job.key) + len(job.name))}
			streams[job.key] = s
			job.charge -= s.charge
		}
		s.touched = time.Now()
		if !s.oversized && len(job.bytes) <= c.config.MaxArtifactBytes-len(s.bytes) {
			s.bytes = append(s.bytes, job.bytes...)
			s.charge += int64(len(job.bytes))
			job.charge -= int64(len(job.bytes))
		} else if !s.oversized {
			c.retained.Add(-int64(len(s.bytes)))
			s.charge -= int64(len(s.bytes))
			s.bytes = nil
			s.oversized = true
			c.drop()
		}
		c.retained.Add(-job.charge)
		if job.complete {
			flush(job.key)
		}
	}
	var tick <-chan time.Time
	var timer *time.Ticker
	if c.config.IdleTimeout > 0 {
		interval := c.config.IdleTimeout / 2
		if interval < time.Millisecond {
			interval = time.Millisecond
		}
		if interval > time.Second {
			interval = time.Second
		}
		timer = time.NewTicker(interval)
		tick = timer.C
		defer timer.Stop()
	}
	for {
		select {
		case job := <-c.jobs:
			accept(job)
		case now := <-tick:
			for key, s := range streams {
				if now.Sub(s.touched) >= c.config.IdleTimeout {
					flush(key)
				}
			}
		case <-c.stop:
			for {
				select {
				case job := <-c.jobs:
					accept(job)
				default:
					for key := range streams {
						flush(key)
					}
					return
				}
			}
		}
	}
}
