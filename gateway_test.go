package main

import (
	"fmt"
	"log"
	"net/http"
	"sync"
	"testing"
	"time"
	"voteapi/app/gateway/plugin"
	"voteapi/pkg/util"
)

func TestRace(t *testing.T) {
	var num = 100
	var wg sync.WaitGroup
	wg.Add(num)

	doreq := func() {
		defer wg.Done()
		http.Get("http://127.0.0.1:8888/read6/stats")
	}

	for i := 0; i < num; i++ {
		go doreq()
	}

	wg.Wait()
}

func TestBalance(t *testing.T) {
	services := []plugin.Service{
		{Host: "127.0.0.1", Port: "8011", Weight: 0},
		{Host: "127.0.0.1", Port: "8011", Weight: 40},
		{Host: "127.0.0.1", Port: "8011", Weight: 40},
		{Host: "127.0.0.1", Port: "8011", Weight: 20},
	}
	res := make(map[int]int)
	for i := 0; i < 1000; i++ {
		da := util.RandInt(1, 100)
		sum := 0
		for k, v := range services {
			if da >= sum && da <= sum+v.Weight {
				res[k]++
				break
			} else {
				sum = sum + v.Weight
			}
		}
	}
	fmt.Println(res)
}

func TestFun(t *testing.T) {
	w := plugin.NewWindowLeapArray(10)
	for i := 0; i < 30; i++ {
		re := w.Check(10)
		log.Println(re)
		n := util.RandInt(30, 100)
		time.Sleep(time.Duration(n) * time.Millisecond)
	}
}
