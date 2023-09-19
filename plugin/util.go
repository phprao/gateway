package plugin

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const RESPONSE_CODE_ERROR = 5000

type GatewayResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data any    `json:"data"`
}

func JsonResponseError(w http.ResponseWriter, msg string, code ...int) {
	WriteContentType(w)
	errcode := RESPONSE_CODE_ERROR
	if len(code) > 0 {
		errcode = code[0]
	}
	s, _ := json.Marshal(GatewayResponse{
		Code: errcode,
		Msg:  msg,
		Data: "",
	})

	fmt.Fprintf(w, string(s))
}

func JsonResponseSuccess(w http.ResponseWriter, data any) {
	WriteContentType(w)
	s, _ := json.Marshal(GatewayResponse{
		Code: 0,
		Msg:  "",
		Data: data,
	})

	fmt.Fprintf(w, string(s))
}

func WriteContentType(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
}

// 考虑代理
// 考虑ipv6
func ClientIp(req *http.Request) string {
	ip, _, err := net.SplitHostPort(strings.TrimSpace(req.RemoteAddr))
	if err != nil {
		return ""
	}
	remoteIP := net.ParseIP(ip)
	if remoteIP == nil {
		return ""
	}

	return remoteIP.String()
}

type Set[T comparable] struct {
	data map[T]struct{}
	mu   sync.RWMutex
}

func NewSet[T comparable]() *Set[T] {
	return &Set[T]{
		data: make(map[T]struct{}),
	}
}

func NewSetWithData[T comparable](origin []T) *Set[T] {
	return &Set[T]{
		data: make(map[T]struct{}),
	}
}

func (s *Set[T]) Exists(element T) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	_, ok := s.data[element]
	return ok
}

func (s *Set[T]) Add(element T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.data[element] = struct{}{}
}

func (s *Set[T]) AddMore(elements []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, v := range elements {
		s.data[v] = struct{}{}
	}
}

func (s *Set[T]) Remove(element T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.data, element)
}

func (s *Set[T]) RemoveMore(elements []T) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, v := range elements {
		delete(s.data, v)
	}
}

func (s *Set[T]) IsEmpty() bool {
	if s.Len() == 0 {
		return true
	} else {
		return false
	}
}

func (s *Set[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.data)
}

func (s *Set[T]) All() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()

	res := make([]T, s.Len())
	var index int
	for k, _ := range s.data {
		res[index] = k
		index++
	}

	return res
}

var src = rand.NewSource(time.Now().UnixNano())
var Rander = rand.New(src)

// 随机数生成
// @Param	min 	int	最小值
// @Param 	max		int	最大值
// @return  int		[min, max]
func RandInt(min int, max int) int {
	if min == max {
		return min
	}
	// Rander.Intn  --> [0, n)
	num := Rander.Intn(max-min+1) + min
	return num
}
