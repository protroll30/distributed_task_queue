package redis

import "testing"

func TestReadyList(t *testing.T) {
	got := ReadyList("dto:", "default")
	const want = "dto:queue:ready:default"
	if got != want {
		t.Fatalf("ReadyList: got %q want %q", got, want)
	}
}

func TestLeaseTask(t *testing.T) {
	got := LeaseTask("dto:", "550e8400-e29b-41d4-a716-446655440000")
	const want = "dto:lease:task:550e8400-e29b-41d4-a716-446655440000"
	if got != want {
		t.Fatalf("LeaseTask: got %q want %q", got, want)
	}
}
