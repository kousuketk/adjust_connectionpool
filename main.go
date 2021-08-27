package main

import (
	"bytes"
	"fmt"
	"net/http"
	"sync/atomic"
	"time"
)

const (
	maxConnsInitial = 5
	maxConnsLimit   = 1000
)

var client = http.Client{Transport: &http.Transport{MaxIdleConnsPerHost: maxConnsLimit}}
var sem *ElasticSemaphore = NewElasticSemaphore(maxConnsInitial, maxConnsLimit)

type ElasticSemaphore struct {
	sem chan struct{}
	max int32
}

func NewElasticSemaphore(initial, limit int) *ElasticSemaphore {
	if initial > limit {
		panic("initial should be <= limit")
	}

	s := make(chan struct{}, limit)
	for i := 0; i < initial; i++ {
		s <- struct{}{}
	}
	return &ElasticSemaphore{
		sem: s,
		max: int32(initial),
	}
}

func (t *ElasticSemaphore) Acquire() {
	select {
	case <-t.sem:
	case <-time.After(time.Second):
		atomic.AddInt32(&t.max, +1)
	}
}

func (t *ElasticSemaphore) Release() {
	select {
	case t.sem <- struct{}{}:
	default:
		atomic.AddInt32(&t.max, -1)
	}
}

func (t *ElasticSemaphore) MaxConns() int32 {
	return atomic.LoadInt32(&t.max)
}

func worker() {
	buff := &bytes.Buffer{}
	for {
		sem.Acquire()
		res, err := client.Get("http://127.0.0.1:8001/")
		if err != nil {
			sem.Release()
			panic(err)
		}
		buff.ReadFrom(res.Body)
		res.Body.Close()
		sem.Release()
		buff.Reset()
		time.Sleep(time.Second)
	}
}

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte("hello, world\n"))
	})
	go func() { fmt.Println(http.ListenAndServe("127.0.0.1:8001", nil)) }()

	for i := 0; i < 1000; i++ {
		go worker() // ひたすらリクエストを送る
		time.Sleep(time.Millisecond)
	}

	for i := 0; i < 100; i++ {
		fmt.Println(sem.MaxConns())
		time.Sleep(time.Second / 5)
	}
}
