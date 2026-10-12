// Package notify owns OS-backed file change subscriptions. It has no polling loop.
package notify

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"

	"github.com/fsnotify/fsnotify"
)

var ErrClosed = errors.New("file change subscription closed")

type Operation uint32

const (
	Create Operation = 1 << iota
	Write
	Remove
	Rename
	Chmod
)

type Event struct {
	Path      string
	Operation Operation
}

// FileSubscription observes directory replacement and the current file inode
// with independent OS watches. Call Close to release both. Use one reader.
type FileSubscription struct {
	path      string
	directory *fsnotify.Watcher
	mu        sync.Mutex
	file      *fsnotify.Watcher
	identity  os.FileInfo
	closed    chan struct{}
	once      sync.Once
	closeErr  error
}

func SubscribeFile(path string) (*FileSubscription, error) {
	if path == "" {
		return nil, errors.New("file path is required")
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	directory, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	if err = directory.Add(filepath.Dir(absolute)); err != nil {
		directory.Close()
		return nil, err
	}
	s := &FileSubscription{path: filepath.Clean(absolute), directory: directory, closed: make(chan struct{})}
	if err = s.bindCurrentFile(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

// bindCurrentFile never changes the directory watch's child registrations.
// On kqueue, doing that races its own automatic inode replacement handling.
func (s *FileSubscription) bindCurrentFile() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	select {
	case <-s.closed:
		return ErrClosed
	default:
	}
	identity, err := os.Stat(s.path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if err == nil && s.identity != nil && os.SameFile(s.identity, identity) {
		return nil
	}
	var next *fsnotify.Watcher
	if err == nil {
		if identity.IsDir() {
			return errors.New("file subscription requires a file path")
		}
		next, err = fsnotify.NewWatcher()
		if err != nil {
			return err
		}
		if err = next.Add(s.path); err != nil {
			next.Close()
			if !os.IsNotExist(err) {
				return err
			}
			next = nil
			identity = nil
		}
	} else {
		identity = nil
	}
	old := s.file
	s.file = next
	s.identity = identity
	if old != nil {
		old.Close()
	}
	return nil
}

// Read blocks for an OS event, cancellation, watcher error, or Close.
func (s *FileSubscription) Read(ctx context.Context) (Event, error) {
	for {
		s.mu.Lock()
		var events <-chan fsnotify.Event
		var failures <-chan error
		if s.file != nil {
			events = s.file.Events
			failures = s.file.Errors
		}
		s.mu.Unlock()
		var event fsnotify.Event
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case <-s.closed:
			return Event{}, ErrClosed
		case e, ok := <-s.directory.Events:
			if !ok {
				return Event{}, ErrClosed
			}
			event = e
		case e, ok := <-events:
			if !ok {
				return Event{}, ErrClosed
			}
			event = e
		case err, ok := <-s.directory.Errors:
			if !ok {
				return Event{}, ErrClosed
			}
			return Event{}, err
		case err, ok := <-failures:
			if !ok {
				return Event{}, ErrClosed
			}
			return Event{}, err
		}
		if filepath.Clean(event.Name) != s.path {
			continue
		}
		var operation Operation
		for _, pair := range []struct {
			source fsnotify.Op
			target Operation
		}{{fsnotify.Create, Create}, {fsnotify.Write, Write}, {fsnotify.Remove, Remove}, {fsnotify.Rename, Rename}, {fsnotify.Chmod, Chmod}} {
			if event.Op.Has(pair.source) {
				operation |= pair.target
			}
		}
		if operation == 0 {
			continue
		}
		if operation&(Create|Rename|Remove) != 0 {
			if err := s.bindCurrentFile(); err != nil {
				return Event{}, err
			}
		}
		return Event{Path: s.path, Operation: operation}, nil
	}
}
func (s *FileSubscription) Close() error {
	s.once.Do(func() {
		close(s.closed)
		s.mu.Lock()
		file := s.file
		s.file = nil
		s.mu.Unlock()
		if file != nil {
			s.closeErr = file.Close()
		}
		if err := s.directory.Close(); s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}
