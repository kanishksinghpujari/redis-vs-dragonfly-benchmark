package main

import "testing"

func TestPercentileNearestRank(t *testing.T) {
	values := []int64{1_000, 2_000, 3_000, 4_000}
	tests := []struct {
		name string
		p    float64
		want float64
	}{
		{name: "p50", p: 0.50, want: 2},
		{name: "p95", p: 0.95, want: 4},
		{name: "p999", p: 0.999, want: 4},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := percentile(values, tt.p); got != tt.want {
				t.Fatalf("percentile(%v, %v) = %v, want %v", values, tt.p, got, tt.want)
			}
		})
	}
}

func TestPercentileEmpty(t *testing.T) {
	if got := percentile(nil, 0.99); got != 0 {
		t.Fatalf("percentile(nil, 0.99) = %v, want 0", got)
	}
}

func TestMakeValue(t *testing.T) {
	if got := makeValue(3); got != "xxx" {
		t.Fatalf("makeValue(3) = %q, want %q", got, "xxx")
	}
	if got := makeValue(0); got != "" {
		t.Fatalf("makeValue(0) = %q, want empty string", got)
	}
}
