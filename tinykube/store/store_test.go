package store_test

import (
	"context"
	"testing"
	"time"

	"github.com/krapi0314/tinybox/tinykube/store"
)

func TestPutAndGet(t *testing.T) {
	s := store.New()

	s.Put("foo/bar", "hello")
	val, ok := s.Get("foo/bar")
	if !ok {
		t.Fatal("expected key to exist")
	}
	if val.(string) != "hello" {
		t.Fatalf("expected 'hello', got %v", val)
	}
}

func TestGetMissing(t *testing.T) {
	s := store.New()
	_, ok := s.Get("nonexistent")
	if ok {
		t.Fatal("expected key to be missing")
	}
}

func TestDelete(t *testing.T) {
	s := store.New()
	s.Put("foo/bar", "hello")
	s.Delete("foo/bar")
	_, ok := s.Get("foo/bar")
	if ok {
		t.Fatal("expected key to be deleted")
	}
}

func TestListWithPrefix(t *testing.T) {
	s := store.New()
	s.Put("pods/default/pod1", "p1")
	s.Put("pods/default/pod2", "p2")
	s.Put("deployments/default/dep1", "d1")

	items := s.List("pods/default/")
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(items))
	}
}

func TestListEmpty(t *testing.T) {
	s := store.New()
	items := s.List("pods/")
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestWatchAdded(t *testing.T) {
	s := store.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := s.Watch(ctx)

	s.Put("pods/default/pod1", "p1")

	select {
	case event := <-ch:
		if event.Type != store.EventAdded {
			t.Fatalf("expected Added, got %s", event.Type)
		}
		if event.Key != "pods/default/pod1" {
			t.Fatalf("expected key pods/default/pod1, got %s", event.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Added event")
	}
}

func TestWatchModified(t *testing.T) {
	s := store.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Put("pods/default/pod1", "p1")

	ch := s.Watch(ctx)

	s.Put("pods/default/pod1", "p1-updated")

	select {
	case event := <-ch:
		if event.Type != store.EventModified {
			t.Fatalf("expected Modified, got %s", event.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Modified event")
	}
}

func TestWatchDeleted(t *testing.T) {
	s := store.New()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Put("pods/default/pod1", "p1")

	ch := s.Watch(ctx)

	s.Delete("pods/default/pod1")

	select {
	case event := <-ch:
		if event.Type != store.EventDeleted {
			t.Fatalf("expected Deleted, got %s", event.Type)
		}
		if event.Key != "pods/default/pod1" {
			t.Fatalf("expected key pods/default/pod1, got %s", event.Key)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for Deleted event")
	}
}

func TestWatchCancelledContextClosesChannel(t *testing.T) {
	s := store.New()
	ctx, cancel := context.WithCancel(context.Background())

	_ = s.Watch(ctx)
	cancel()

	// Channel should be closed or drained after cancellation
	// Give goroutine time to process
	time.Sleep(50 * time.Millisecond)

	// Subsequent puts should not block
	s.Put("foo/bar", "baz")
	// No panic or deadlock is the success condition
}
