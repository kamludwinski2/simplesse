package main

import (
	"fmt"
	sse "github.com/kamludwinski2/simplesse"
	"net/http"
	"os"
	"time"
)

type TestStruct struct {
	Name string
	Age  int
}

var (
	cache map[string]TestStruct
)

func main() {
	fmt.Println("Hello World")

	cache = make(map[string]TestStruct)

	cache["a"] = TestStruct{"Alice", 30}
	cache["b"] = TestStruct{"Bob", 40}

	cacheTransform := func() []TestStruct {
		res := make([]TestStruct, 0)

		for _, v := range cache {
			res = append(res, v)
		}

		return res
	}

	s := sse.NewServer[TestStruct](cacheTransform).
		OnError(func(err error) {
			fmt.Println("error")
			fmt.Println(err)
		}).
		OnWarn(func(s string) {
			fmt.Println("warning", s)
		}).
		OnConnect(func(s string) {
			fmt.Println("connect", s)
		}).
		OnDisconnect(func(s string, duration time.Duration) {
			fmt.Println("disconnect", s, duration)
		}).
		OnFlush(func(s string, e sse.Event[TestStruct]) {
			fmt.Println("flush", s, e)
		}).
		OnSnapshot(func(s string, e sse.Event[TestStruct]) {
			fmt.Println("snapshot", s, e)
		}).
		OnSend(func(strings []string) {
			fmt.Println("send", strings)
		}).
		OnShutdown(func(strings []string) {
			fmt.Println("shutdown", strings)
		})
	defer s.Shutdown()

	http.HandleFunc("/sse/test", func(w http.ResponseWriter, r *http.Request) {
		s.AddConnection(w, r)
	})

	http.HandleFunc("/sse/close", func(w http.ResponseWriter, r *http.Request) {
		s.Shutdown()

		go func() {
			time.Sleep(2 * time.Second)
			os.Exit(0)
		}()
	})

	http.HandleFunc("/test-connection-count", func(w http.ResponseWriter, r *http.Request) {
		fmt.Println("conn count")
		fmt.Println(s.GetConnectionCount())
	})

	http.HandleFunc("/test-update", func(w http.ResponseWriter, r *http.Request) {
		newObjects := []TestStruct{{Name: "Alice2", Age: 50}, {Name: "Bob2", Age: 420}}

		for _, obj := range newObjects {
			cache[obj.Name] = obj
		}

		e := sse.Event[TestStruct]{
			Type:    "CREATE",
			Payload: newObjects,
		}

		_ = s.SendEvent(e)
	})

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		os.Exit(1)
	}

	os.Exit(0)
}
