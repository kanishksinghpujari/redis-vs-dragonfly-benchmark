package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
)

type benchmarkResult struct {
	Server       string  `json:"server"`
	Address      string  `json:"address"`
	Operation    string  `json:"operation"`
	Operations   int     `json:"operations"`
	Concurrency  int     `json:"concurrency"`
	ValueSize    int     `json:"value_size_bytes"`
	Warmup       int     `json:"warmup_operations"`
	ElapsedMS    float64 `json:"elapsed_ms"`
	OpsPerSecond float64 `json:"ops_per_second"`
	P50US        float64 `json:"p50_us"`
	P95US        float64 `json:"p95_us"`
	P99US        float64 `json:"p99_us"`
	P999US       float64 `json:"p999_us"`
	Errors       int64   `json:"errors"`
	GoVersion    string  `json:"go_version"`
	Timestamp    string  `json:"timestamp_utc"`
}

type target struct {
	name string
	addr string
}

func percentile(sortedNanos []int64, p float64) float64 {
	if len(sortedNanos) == 0 {
		return 0
	}
	// Nearest-rank percentile, converted from nanoseconds to microseconds.
	rank := int(float64(len(sortedNanos))*p + 0.999999)
	if rank < 1 {
		rank = 1
	}
	if rank > len(sortedNanos) {
		rank = len(sortedNanos)
	}
	return float64(sortedNanos[rank-1]) / 1000
}

func makeValue(size int) string {
	value := make([]byte, size)
	for i := range value {
		value[i] = 'x'
	}
	return string(value)
}

func seedKeys(ctx context.Context, client *redis.Client, n int, value string) error {
	const batchSize = 1000
	for start := 0; start < n; start += batchSize {
		end := start + batchSize
		if end > n {
			end = n
		}
		pipe := client.Pipeline()
		for i := start; i < end; i++ {
			pipe.Set(ctx, fmt.Sprintf("bench:key:%d", i), value, 0)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return err
		}
	}
	return nil
}

func run(ctx context.Context, t target, op string, n, concurrency, valueSize, warmup int) (benchmarkResult, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         t.addr,
		PoolSize:     concurrency,
		MinIdleConns: concurrency,
	})
	defer client.Close()

	if err := client.Ping(ctx).Err(); err != nil {
		return benchmarkResult{}, fmt.Errorf("%s at %s is not reachable: %w", t.name, t.addr, err)
	}

	value := makeValue(valueSize)
	if op == "get" {
		if err := seedKeys(ctx, client, n, value); err != nil {
			return benchmarkResult{}, fmt.Errorf("%s GET seed failed: %w", t.name, err)
		}
	}

	// Warm-up is deliberately outside the measurement interval.
	for i := 0; i < warmup; i++ {
		keyIndex := i
		if op == "get" {
			keyIndex %= n
		}
		key := fmt.Sprintf("bench:warmup:%d", keyIndex)
		if op == "get" {
			key = fmt.Sprintf("bench:key:%d", keyIndex)
		}
		if op == "set" {
			if err := client.Set(ctx, key, value, 0).Err(); err != nil {
				return benchmarkResult{}, fmt.Errorf("%s SET warm-up failed: %w", t.name, err)
			}
		} else if err := client.Get(ctx, key).Err(); err != nil {
			return benchmarkResult{}, fmt.Errorf("%s GET warm-up failed: %w", t.name, err)
		}
	}

	latencies := make([]int64, n)
	var next atomic.Int64
	var errorCount atomic.Int64
	var wait sync.WaitGroup

	doOne := func(i int) error {
		key := fmt.Sprintf("bench:key:%d", i)
		switch op {
		case "set":
			return client.Set(ctx, key, value, 0).Err()
		case "get":
			_, err := client.Get(ctx, key).Result()
			return err
		default:
			return errors.New("unsupported operation")
		}
	}

	start := time.Now()
	for worker := 0; worker < concurrency; worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				i := int(next.Add(1) - 1)
				if i >= n {
					return
				}
				begin := time.Now()
				err := doOne(i)
				latencies[i] = time.Since(begin).Nanoseconds()
				if err != nil {
					errorCount.Add(1)
				}
			}
		}()
	}
	wait.Wait()
	elapsed := time.Since(start)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })

	failures := errorCount.Load()
	successes := int64(n) - failures
	opsPerSecond := 0.0
	if elapsed > 0 {
		opsPerSecond = float64(successes) / elapsed.Seconds()
	}

	return benchmarkResult{
		Server: t.name, Address: t.addr, Operation: op, Operations: n,
		Concurrency: concurrency, ValueSize: valueSize, Warmup: warmup,
		ElapsedMS: float64(elapsed.Microseconds()) / 1000,
		OpsPerSecond: opsPerSecond,
		P50US: percentile(latencies, .50), P95US: percentile(latencies, .95),
		P99US: percentile(latencies, .99), P999US: percentile(latencies, .999),
		Errors: failures, GoVersion: runtime.Version(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func main() {
	op := flag.String("op", "set", "operation: set or get")
	n := flag.Int("n", 100000, "number of measured operations per server")
	concurrency := flag.Int("c", 32, "number of concurrent workers")
	valueSize := flag.Int("value-size", 128, "value size in bytes")
	warmup := flag.Int("warmup", 10000, "number of warm-up operations per server")
	timeout := flag.Duration("timeout", 5*time.Minute, "timeout per server")
	flag.Parse()

	if *op != "set" && *op != "get" {
		log.Fatal("-op must be set or get")
	}
	if *n <= 0 || *concurrency <= 0 || *valueSize < 0 || *warmup < 0 {
		log.Fatal("n and concurrency must be positive; value-size and warmup cannot be negative")
	}

	targets := []target{{name: "redis", addr: "localhost:6379"}, {name: "dragonfly", addr: "localhost:6380"}}
	encoder := json.NewEncoder(os.Stdout)
	for _, server := range targets {
		ctx, cancel := context.WithTimeout(context.Background(), *timeout)
		result, err := run(ctx, server, *op, *n, *concurrency, *valueSize, *warmup)
		cancel()
		if err != nil {
			log.Fatal(err)
		}
		if err := encoder.Encode(result); err != nil {
			log.Fatal(err)
		}
	}
}
