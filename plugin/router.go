package plugin

import (
	"bytes"
	"errors"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"
	"voteapi/pkg/util"
)

const ServiceStatusReady = 0
const ServiceStatusLoading = 1

const BalanceByOrder uint8 = 0  // 负载均衡：轮训
const BalanceByWeight uint8 = 1 // 负载均衡：权重

type Service struct {
	Host   string `json:"host"`
	Port   string `json:"port"`
	Weight int    `json:"weight"`
}

type ServiceMapType map[string][]Service

type Router struct {
	mu                    sync.Mutex
	LastUpdate            time.Time
	Expire                time.Duration
	ServiceStatus         uint8
	ServiceMap            ServiceMapType
	ServiceMapCache       ServiceMapType
	GetServiceNameFun     func(string) string
	TargetPathFunc        func(Service, string) (*url.URL, error)
	BalanceType           uint8
	BalanceTypeData       map[string]int // 负载均衡相关数据
	EnableServiceDiscover bool           // 启动服务发现
}

func NewRouter() *Router {
	router := &Router{
		GetServiceNameFun: DefaultGetServiceName,
		TargetPathFunc:    DefaultTargetPathFunc,
		ServiceMap:        make(ServiceMapType),
		ServiceMapCache:   make(ServiceMapType),
		BalanceTypeData:   make(map[string]int),
		Expire:            10 * time.Minute,
	}
	if err := router.RefreshServices(); err != nil {
		log.Println(err)
	}
	return router
}

func DefaultGetServiceName(path string) (serviceName string) {
	if path == "" {
		return
	}
	pathSplit := strings.Split(path, "/")
	return pathSplit[1]
}

func DefaultTargetPathFunc(service Service, path string) (*url.URL, error) {
	return url.Parse("http://" + service.Host + ":" + service.Port + path)
}

func (r *Router) SetServices() error {
	r.ServiceStatus = ServiceStatusLoading
	defer func() {
		r.ServiceStatus = ServiceStatusReady
	}()

	if sm, err := RetrieveServices(); err != nil {
		return err
	} else {
		r.ServiceMap = sm
	}

	r.ServiceMapCache = r.ServiceMap

	r.LastUpdate = time.Now()

	return nil
}

// RefreshServices Refresh cache goroutine
func (r *Router) RefreshServices() error {
	err := r.SetServices()
	if err != nil {
		return err
	}

	if r.EnableServiceDiscover {
		go func() {
			for {
				time.Sleep(r.Expire)

				if err := r.SetServices(); err != nil {
					log.Println(err)
				}
			}
		}()
	}

	return nil
}

func (r *Router) ServiceBalance(serviceName string) (Service, error) {
	if serviceName == "" {
		return Service{}, errors.New("serviceName can not be empty.")
	}

	var services []Service
	var ok bool

	if r.ServiceStatus == ServiceStatusReady {
		services, ok = r.ServiceMap[serviceName]
	} else {
		services, ok = r.ServiceMapCache[serviceName]
	}

	if !ok {
		return Service{}, errors.New("can not match the specified service: " + serviceName)
	}
	serlen := len(services)
	if serlen == 0 {
		return Service{}, errors.New("can not match the specified service: " + serviceName)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if r.BalanceType == BalanceByOrder {
		ind, _ := r.BalanceTypeData[serviceName]
		if ind >= serlen {
			ind = 0
		}
		r.BalanceTypeData[serviceName] = ind + 1

		return services[ind], nil
	} else if r.BalanceType == BalanceByWeight {
		da := util.RandInt(1, 100)
		sum := 0
		for k, v := range services {
			if da >= sum && da <= sum+v.Weight {
				return services[k], nil
			} else {
				sum = sum + v.Weight
			}
		}
	}

	return Service{}, nil
}

func (r *Router) ReverseProxy(w http.ResponseWriter, req *http.Request) {
	for k, v := range LocalPath {
		if strings.HasPrefix(req.URL.Path, k) {
			v(r, w, req)
			return
		}
	}

	service, err := r.ServiceBalance(r.GetServiceNameFun(req.URL.Path))
	if err != nil {
		JsonResponseError(w, err.Error())
		return
	}

	target, err := r.TargetPathFunc(service, req.URL.Path)
	if err != nil {
		JsonResponseError(w, "404 Not Found")
		return
	}

	proxy := httputil.NewSingleHostReverseProxy(target)

	// RawQuery 为地址栏上的参数，去除了问号
	// req.URL 中 Schema, Host都是空的，Path 有值
	originalDerector := proxy.Director

	proxy.Director = func(req *http.Request) {
		originalDerector(req)

		// 默认的 Path 规则不合适，此处需要修改
		req.URL.Path = target.Path
	}

	proxy.ModifyResponse = func(response *http.Response) error {
		response.Header.Set("X-Proxy", "Gateway")

		cont, _ := io.ReadAll(response.Body)
		log.Println(string(cont))

		response.Body = io.NopCloser(bytes.NewReader(cont))
		return nil
	}

	// ErrorHandler is an optional function that handles errors
	// reaching the backend or errors from ModifyResponse.
	//
	// If nil, the default is to log the provided error and return
	// a 502 Status Bad Gateway response.
	proxy.ErrorHandler = func(resp http.ResponseWriter, req *http.Request, err error) {
		return
	}

	proxy.ServeHTTP(w, req)
}

func RetrieveServices() (ServiceMapType, error) {
	ServiceMap := make(ServiceMapType)

	// tmp data
	ServiceMap["show"] = []Service{
		{Host: "127.0.0.1", Port: "8007"},
		{Host: "127.0.0.1", Port: "8007"},
		{Host: "127.0.0.1", Port: "8007"},
		{Host: "127.0.0.1", Port: "8007"},
	}
	ServiceMap["read6"] = []Service{
		{Host: "127.0.0.1", Port: "8011", Weight: 0},
		{Host: "127.0.0.1", Port: "8011", Weight: 40},
		{Host: "127.0.0.1", Port: "8011", Weight: 40},
		{Host: "127.0.0.1", Port: "8011", Weight: 20},
	}

	return ServiceMap, nil
}

var LocalPath = map[string]func(*Router, http.ResponseWriter, *http.Request){
	"/gateway/getServices":     LocalPathGetServices,
	"/gateway/refreshServices": LocalPathRefreshServices,
}

func LocalPathGetServices(r *Router, w http.ResponseWriter, req *http.Request) {
	JsonResponseSuccess(w, r.ServiceMap)
}

func LocalPathRefreshServices(r *Router, w http.ResponseWriter, req *http.Request) {
	err := r.SetServices()
	if err != nil {
		JsonResponseError(w, err.Error())
	} else {
		JsonResponseSuccess(w, "success")
	}
}
