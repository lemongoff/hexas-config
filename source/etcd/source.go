// Package etcd provides an exact-key runtime configuration source.
package etcd

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	config "github.com/lemongoff/hexas-config"
	clientv3 "go.etcd.io/etcd/client/v3"
)

const defaultTimeout = 5 * time.Second

// Config defines a bounded exact-key etcd source.
type Config struct {
	Client  *clientv3.Client
	Key     string
	Timeout time.Duration
}

// Source loads and watches one exact etcd key.
type Source struct {
	client  *clientv3.Client
	key     string
	timeout time.Duration
}

// New creates an etcd source. The caller owns and closes Client.
func New(configuration Config) (*Source, error) {
	if configuration.Client == nil {
		return nil, errors.New("config/etcd: client is required")
	}
	if strings.TrimSpace(configuration.Key) == "" {
		return nil, errors.New("config/etcd: key is required")
	}
	timeout := configuration.Timeout
	if timeout == 0 {
		timeout = defaultTimeout
	}
	if timeout < 0 {
		return nil, errors.New("config/etcd: timeout must be positive")
	}
	return &Source{client: configuration.Client, key: configuration.Key, timeout: timeout}, nil
}

// Name returns the stable source identity.
func (s *Source) Name() string { return "etcd:" + s.key }

// Load reads the current exact-key YAML document.
func (s *Source) Load(ctx context.Context) (config.Document, error) {
	loadCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	response, err := s.client.Get(loadCtx, s.key)
	if err != nil {
		return config.Document{}, fmt.Errorf("get %q: %w", s.key, err)
	}
	if len(response.Kvs) == 0 {
		return config.Document{}, fmt.Errorf("key %q does not exist", s.key)
	}
	values, err := config.ParseYAML(response.Kvs[0].Value)
	if err != nil {
		return config.Document{}, fmt.Errorf("parse key %q: %w", s.key, err)
	}
	return config.Document{Values: values, Revision: config.Revision(strconv.FormatInt(response.Kvs[0].ModRevision, 10))}, nil
}

// Watch watches changes after the supplied source revision. Transient transport
// recovery is delegated to the etcd client; cancellation and compaction are surfaced.
func (s *Source) Watch(ctx context.Context, after config.Revision) (<-chan config.Event, error) {
	start := int64(0)
	if after != "" {
		parsed, err := strconv.ParseInt(string(after), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("invalid revision %q: %w", after, err)
		}
		start = parsed + 1
	}
	options := []clientv3.OpOption{}
	if start > 0 {
		options = append(options, clientv3.WithRev(start))
	}
	responses := s.client.Watch(ctx, s.key, options...)
	events := make(chan config.Event, 1)
	go func() {
		defer close(events)
		for {
			select {
			case <-ctx.Done():
				return
			case response, ok := <-responses:
				if !ok {
					return
				}
				if err := response.Err(); err != nil {
					select {
					case events <- config.Event{Err: err}:
					case <-ctx.Done():
					}
					return
				}
				if len(response.Events) == 0 {
					continue
				}
				revision := config.Revision(strconv.FormatInt(response.Header.Revision, 10))
				select {
				case events <- config.Event{Revision: revision}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return events, nil
}

var _ config.WatchSource = (*Source)(nil)
