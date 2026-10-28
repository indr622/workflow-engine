package keygen

import (
	"bytes"
	"context"
	"crypto/rand"
	"sync"
	"testing"
	"time"
)

func TestKeysAreValidVersion7AndIncreasing(t *testing.T) {
	g := New()
	keys, err := g.NextKeys(context.Background(), 10_000)
	if err != nil {
		t.Fatal(err)
	}
	for i, k := range keys {
		if !Valid(k) {
			t.Fatalf("key %q is not a canonical uuid", k)
		}
		if k[14] != '7' {
			t.Fatalf("key %q is not version 7", k)
		}
		if v := k[19]; v != '8' && v != '9' && v != 'a' && v != 'b' {
			t.Fatalf("key %q does not have the RFC 9562 variant", k)
		}
		if i > 0 && k <= keys[i-1] {
			t.Fatalf("key %d %q is not greater than %q", i, k, keys[i-1])
		}
	}
}

func TestClockGoingBackwardsKeepsOrder(t *testing.T) {
	times := []time.Time{time.UnixMilli(2_000), time.UnixMilli(1_000), time.UnixMilli(1_000)}
	i := 0
	g := NewWith(func() time.Time { t := times[min(i, len(times)-1)]; i++; return t }, rand.Reader)
	var prev string
	for range times {
		k, err := g.NewKey()
		if err != nil {
			t.Fatal(err)
		}
		if k <= prev {
			t.Fatalf("%q is not greater than %q", k, prev)
		}
		prev = k
	}
}

func TestCounterOverflowBorrowsNextMillisecond(t *testing.T) {
	fixed := time.UnixMilli(5_000)
	// Sumber acak berisi 0xff membuat counter awal maksimal (0x7ff) sehingga cepat penuh.
	g := NewWith(func() time.Time { return fixed }, bytes.NewReader(bytes.Repeat([]byte{0xff}, 16*5000)))
	var prev string
	for i := 0; i < 4500; i++ {
		k, err := g.NewKey()
		if err != nil {
			t.Fatal(err)
		}
		if k <= prev {
			t.Fatalf("key %d %q is not greater than %q", i, k, prev)
		}
		prev = k
	}
}

func TestConcurrentKeysAreUnique(t *testing.T) {
	g := New()
	var mu sync.Mutex
	seen := map[string]bool{}
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			keys, err := g.NextKeys(context.Background(), 1000)
			if err != nil {
				t.Error(err)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			for _, k := range keys {
				if seen[k] {
					t.Errorf("duplicate key %s", k)
				}
				seen[k] = true
			}
		}()
	}
	wg.Wait()
}

func TestValid(t *testing.T) {
	for s, want := range map[string]bool{
		"0190f5d2-6c1a-7b3e-8f00-123456789abc": true,
		"0190F5D2-6C1A-7B3E-8F00-123456789ABC": false,
		"0190f5d26c1a7b3e8f00123456789abc":     false,
		"0190f5d2-6c1a-7b3e-8f00-123456789ab":  false,
		"":                                     false,
	} {
		if got := Valid(s); got != want {
			t.Errorf("Valid(%q) = %v, want %v", s, got, want)
		}
	}
}
