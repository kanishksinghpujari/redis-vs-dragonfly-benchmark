package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/redis/rueidis"
)

type benchmarkResult struct {
	Server          string  `json:"server"`
	Address         string  `json:"address"`
	Client          string  `json:"client"`
	Operation       string  `json:"operation"`
	Operations      int     `json:"operations"`
	Concurrency     int     `json:"concurrency"`
	PipelineSize    int     `json:"pipeline_size"`
	LatencyUnit     string  `json:"latency_sample_unit"`
	ValueSize       int     `json:"value_size_bytes"`
	Warmup          int     `json:"warmup_operations"`
	ElapsedMS       float64 `json:"elapsed_ms"`
	OpsPerSecond    float64 `json:"ops_per_second"`
	P50US           float64 `json:"p50_us"`
	P95US           float64 `json:"p95_us"`
	P99US           float64 `json:"p99_us"`
	P999US          float64 `json:"p999_us"`
	Errors          int64   `json:"errors"`
	GoVersion       string  `json:"go_version"`
	Timestamp       string  `json:"timestamp_utc"`
}

type target struct {
	name string
	addr string
}

type benchClient interface {
	Ping(context.Context) error
	Set(context.Context, string, string) error
	Get(context.Context, string) error
	SetBatch(context.Context, []string, string) []error
	GetBatch(context.Context, []string) []error
	Close() error
}

type goRedisClient struct { client *redis.Client }

func newGoRedisClient(addr string, concurrency int) (*goRedisClient, error) {
	c := redis.NewClient(&redis.Options{Addr: addr, PoolSize: concurrency, MinIdleConns: concurrency})
	return &goRedisClient{client:c}, nil
}
func (c *goRedisClient) Ping(ctx context.Context) error { return c.client.Ping(ctx).Err() }
func (c *goRedisClient) Set(ctx context.Context, key, value string) error { return c.client.Set(ctx,key,value,0).Err() }
func (c *goRedisClient) Get(ctx context.Context, key string) error { return c.client.Get(ctx,key).Err() }
func (c *goRedisClient) Close() error { return c.client.Close() }
func (c *goRedisClient) SetBatch(ctx context.Context, keys []string, value string) []error {
	pipe:=c.client.Pipeline()
	cmds:=make([]*redis.StatusCmd,0,len(keys))
	for _,key:=range keys { cmds=append(cmds,pipe.Set(ctx,key,value,0)) }
	_,_ = pipe.Exec(ctx)
	errs:=make([]error,len(cmds))
	for i,cmd:=range cmds { errs[i]=cmd.Err() }
	return errs
}
func (c *goRedisClient) GetBatch(ctx context.Context, keys []string) []error {
	pipe:=c.client.Pipeline()
	cmds:=make([]*redis.StringCmd,0,len(keys))
	for _,key:=range keys { cmds=append(cmds,pipe.Get(ctx,key)) }
	_,execErr:=pipe.Exec(ctx)
	errs:=make([]error,len(cmds))
	for i,cmd:=range cmds { errs[i]=cmd.Err(); if errs[i]==nil && execErr!=nil { errs[i]=execErr } }
	return errs
}

type rueidisClient struct { client rueidis.Client }

func newRueidisClient(addr string, _ int) (*rueidisClient,error) {
	c,err:=rueidis.NewClient(rueidis.ClientOption{
		InitAddress: []string{addr},
		ForceSingleClient: true,
		DisableCache: true,
		DisableRetry: true,
		DisableAutoPipelining: true,
	})
	if err!=nil { return nil,err }
	return &rueidisClient{client:c},nil
}
func (c *rueidisClient) Ping(ctx context.Context) error {
	return c.client.Do(ctx,c.client.B().Ping().Build()).Error()
}
func (c *rueidisClient) Set(ctx context.Context,key,value string) error {
	return c.client.Do(ctx,c.client.B().Set().Key(key).Value(value).Build()).Error()
}
func (c *rueidisClient) Get(ctx context.Context,key string) error {
	return c.client.Do(ctx,c.client.B().Get().Key(key).Build()).Error()
}
func (c *rueidisClient) Close() error { c.client.Close(); return nil }
func (c *rueidisClient) SetBatch(ctx context.Context,keys []string,value string) []error {
	cmds:=make(rueidis.Commands,0,len(keys))
	for _,key:=range keys { cmds=append(cmds,c.client.B().Set().Key(key).Value(value).Build()) }
	responses:=c.client.DoMulti(ctx,cmds...)
	errs:=make([]error,len(keys))
	for i:=range errs { if i>=len(responses) { errs[i]=errors.New("missing pipeline response") } else { errs[i]=responses[i].Error() } }
	return errs
}
func (c *rueidisClient) GetBatch(ctx context.Context,keys []string) []error {
	cmds:=make(rueidis.Commands,0,len(keys))
	for _,key:=range keys { cmds=append(cmds,c.client.B().Get().Key(key).Build()) }
	responses:=c.client.DoMulti(ctx,cmds...)
	errs:=make([]error,len(keys))
	for i:=range errs { if i>=len(responses) { errs[i]=errors.New("missing pipeline response") } else { errs[i]=responses[i].Error() } }
	return errs
}

func percentile(sortedNanos []int64,p float64) float64 {
	if len(sortedNanos)==0 { return 0 }
	rank:=int(math.Ceil(float64(len(sortedNanos))*p))
	if rank<1 { rank=1 }
	if rank>len(sortedNanos) { rank=len(sortedNanos) }
	return float64(sortedNanos[rank-1])/1000
}
func makeValue(size int) string {
	value:=make([]byte,size)
	for i:=range value { value[i]='x' }
	return string(value)
}
func makeKeys(start,end int) []string {
	keys:=make([]string,0,end-start)
	for i:=start;i<end;i++ { keys=append(keys,fmt.Sprintf("bench:key:%d",i)) }
	return keys
}
func countErrors(errs []error) int64 {
	var count int64
	for _,err:=range errs { if err!=nil { count++ } }
	return count
}

func run(ctx context.Context,t target,clientName,op string,n,concurrency,valueSize,warmup,pipelineSize int) (benchmarkResult,error) {
	var client benchClient
	var err error
	switch clientName {
	case "go-redis": client,err=newGoRedisClient(t.addr,concurrency)
	case "rueidis": client,err=newRueidisClient(t.addr,concurrency)
	default: return benchmarkResult{},fmt.Errorf("unknown client %q",clientName)
	}
	if err!=nil { return benchmarkResult{},fmt.Errorf("create %s client: %w",clientName,err) }
	defer client.Close()
	if err=client.Ping(ctx); err!=nil { return benchmarkResult{},fmt.Errorf("%s at %s is not reachable: %w",t.name,t.addr,err) }

	value:=makeValue(valueSize)
	if op=="get" {
		const seedBatch=500
		for start:=0;start<n;start+=seedBatch {
			end:=start+seedBatch
			if end>n { end=n }
			errs:=client.SetBatch(ctx,makeKeys(start,end),value)
			if failed:=countErrors(errs); failed>0 { return benchmarkResult{},fmt.Errorf("%s GET seed failed for %d/%d keys",t.name,failed,len(errs)) }
		}
	}
	for i:=0;i<warmup;i++ {
		key:=fmt.Sprintf("bench:warmup:%d",i)
		if op=="get" { key=fmt.Sprintf("bench:key:%d",i%n) }
		if op=="set" { err=client.Set(ctx,key,value) } else { err=client.Get(ctx,key) }
		if err!=nil { return benchmarkResult{},fmt.Errorf("%s warm-up failed: %w",t.name,err) }
	}

	batchCount:=(n+pipelineSize-1)/pipelineSize
	latencies:=make([]int64,batchCount)
	var next atomic.Int64
	var errorCount atomic.Int64
	var wait sync.WaitGroup
	started:=time.Now()
	for worker:=0;worker<concurrency;worker++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for {
				start:=int(next.Add(int64(pipelineSize))-int64(pipelineSize))
				if start>=n { return }
				end:=start+pipelineSize
				if end>n { end=n }
				keys:=makeKeys(start,end)
				begin:=time.Now()
				var errs []error
				if op=="set" { errs=client.SetBatch(ctx,keys,value) } else { errs=client.GetBatch(ctx,keys) }
				latencies[start/pipelineSize]=time.Since(begin).Nanoseconds()
				errorCount.Add(countErrors(errs))
			}
		}()
	}
	wait.Wait()
	elapsed:=time.Since(started)
	// Sort a private slice only after all workers have stopped writing.
	sortNanos(latencies)
	failures:=errorCount.Load()
	successes:=int64(n)-failures
	if successes<0 { successes=0 }
	opsPerSecond:=0.0
	if elapsed>0 { opsPerSecond=float64(successes)/elapsed.Seconds() }
	return benchmarkResult{
		Server:t.name,Address:t.addr,Client:clientName,Operation:op,Operations:n,
		Concurrency:concurrency,PipelineSize:pipelineSize,LatencyUnit:"batch_round_trip",
		ValueSize:valueSize,Warmup:warmup,ElapsedMS:float64(elapsed.Microseconds())/1000,
		OpsPerSecond:opsPerSecond,P50US:percentile(latencies,.50),P95US:percentile(latencies,.95),
		P99US:percentile(latencies,.99),P999US:percentile(latencies,.999),Errors:failures,
		GoVersion:runtime.Version(),Timestamp:time.Now().UTC().Format(time.RFC3339),
	},nil
}
func sortNanos(values []int64) {
	// Insertion sort is too slow for large sample counts; use the standard library.
	sort.Slice(values,func(i,j int)bool{return values[i]<values[j]})
}

func main() {
	op:=flag.String("op","set","operation: set or get")
	n:=flag.Int("n",100000,"number of measured operations per server")
	concurrency:=flag.Int("c",32,"number of concurrent workers")
	valueSize:=flag.Int("value-size",128,"value size in bytes")
	warmup:=flag.Int("warmup",10000,"number of warm-up operations per server")
	pipelineSize:=flag.Int("pipeline",1,"commands per round trip (1 = non-pipelined)")
	clientName:=flag.String("client","go-redis","client: go-redis or rueidis")
	redisAddr:=flag.String("redis-addr","localhost:6379","Redis address")
	dragonflyAddr:=flag.String("dragonfly-addr","localhost:6380","Dragonfly address")
	timeout:=flag.Duration("timeout",5*time.Minute,"timeout per server")
	flag.Parse()
	if *op!="set" && *op!="get" { log.Fatal("-op must be set or get") }
	if *n<=0 || *concurrency<=0 || *valueSize<0 || *warmup<0 || *pipelineSize<=0 { log.Fatal("n, concurrency and pipeline must be positive; value-size and warmup cannot be negative") }
	if *clientName!="go-redis" && *clientName!="rueidis" { log.Fatal("-client must be go-redis or rueidis") }

	targets:=[]target{{name:"redis",addr:*redisAddr},{name:"dragonfly",addr:*dragonflyAddr}}
	encoder:=json.NewEncoder(os.Stdout)
	for _,server:=range targets {
		ctx,cancel:=context.WithTimeout(context.Background(),*timeout)
		result,err:=run(ctx,server,*clientName,*op,*n,*concurrency,*valueSize,*warmup,*pipelineSize)
		cancel()
		if err!=nil { log.Fatal(err) }
		if err:=encoder.Encode(result);err!=nil { log.Fatal(err) }
	}
}
