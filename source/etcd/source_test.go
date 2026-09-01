package etcd

import (
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

func TestNewValidatesConfiguration(t *testing.T) {
	if _, err := New(Config{}); err == nil {
		t.Fatal("expected client error")
	}
	client := &clientv3.Client{}
	if _, err := New(Config{Client: client}); err == nil {
		t.Fatal("expected key error")
	}
	if _, err := New(Config{Client: client, Key: "/runtime", Timeout: -time.Second}); err == nil {
		t.Fatal("expected timeout error")
	}
	source, err := New(Config{Client: client, Key: "/runtime"})
	if err != nil {
		t.Fatal(err)
	}
	if source.Name() != "etcd:/runtime" {
		t.Fatalf("unexpected name: %s", source.Name())
	}
}
