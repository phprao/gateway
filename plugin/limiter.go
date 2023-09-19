package plugin

import (
	"errors"
	"hash/crc32"
	"net/http"
	"sync"
	"time"
)

// 滑动窗口
type WindowLeapArray struct {
	Arr          []int // 窗口数据
	Front        int   // 游标
	FrontTime    int64 // 游标最新时间
	WindowStatus bool  // 窗口状态，true 为 拒绝访问
}

func NewWindowLeapArray(windowNum int) *WindowLeapArray {
	return &WindowLeapArray{
		Arr: make([]int, windowNum),
	}
}

// Check 限流计算
func (w *WindowLeapArray) Check(threshold int) bool {
	timenow := time.Now()

	// start := w.FrontTime

	frontTimeLeft := timenow.UnixMilli() - timenow.UnixMilli()%100
	index := (timenow.UnixMilli() - 1000*timenow.Unix()) / 100

	if w.FrontTime == 0 {
		// 记为小窗口的左侧时间 1694678187869 -> 1694678187800
		w.FrontTime = frontTimeLeft
		w.Front = int(index)
		w.Arr[w.Front]++

		// log.Println(timenow.UnixMilli(), start, (timenow.UnixMilli()-start)/100, w.Arr)
		return true
	}

	// 时间差
	gaptime := (timenow.UnixMilli() - w.FrontTime)

	if gaptime < 100 {
		// 同一小窗口

		if w.WindowStatus {
			// log.Println(timenow.UnixMilli(), start, (timenow.UnixMilli()-start)/100, w.Arr)
			return false
		}

		// 统计
		var sum int
		for _, v := range w.Arr {
			sum = sum + v
		}
		if sum >= threshold {
			w.WindowStatus = true
			// log.Println(timenow.UnixMilli(), start, (timenow.UnixMilli()-start)/100, w.Arr)
			return false
		} else {
			w.Arr[w.Front]++
		}
	} else {
		// 滑动，采用环形数组
		// 可能存在跳跃

		w.WindowStatus = false
		w.FrontTime = frontTimeLeft

		gap := gaptime / 100
		if gap >= 10 {
			for i := 0; i < 10; i++ {
				w.Arr[i] = 0
			}
		} else {
			for i := 1; i <= int(gap); i++ {
				tmp := w.Front + i
				if tmp >= 10 {
					tmp = tmp - 10
				}
				w.Arr[tmp] = 0
			}
		}

		w.Front = int(index)

		// 统计
		var sum int
		for _, v := range w.Arr {
			sum = sum + v
		}
		if sum >= threshold {
			w.WindowStatus = true
			// log.Println(timenow.UnixMilli(), start, (timenow.UnixMilli()-start)/100, w.Arr)
			return false
		} else {
			w.Arr[w.Front] = 1
		}
	}
	// log.Println(timenow.UnixMilli(), start, (timenow.UnixMilli()-start)/100, w.Arr)

	return true
}

type RejectList struct {
	data map[string]int64
	mu   sync.RWMutex
}

func NewRejectList() *RejectList {
	return &RejectList{
		data: make(map[string]int64),
	}
}

func (rl *RejectList) Get(key string) (int64, bool) {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	v, ok := rl.data[key]
	return v, ok
}

func (rl *RejectList) GetAll(key string) map[string]int64 {
	rl.mu.RLock()
	defer rl.mu.RUnlock()

	return rl.data
}

func (rl *RejectList) Add(key string, val int64) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	rl.data[key] = val
}

func (rl *RejectList) Remove(key string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	delete(rl.data, key)
}

type Limiter struct {
	WhiteList                *Set[string]                // 白名单
	BlackList                *Set[string]                // 黑名单
	RejectList               *RejectList                 // 封禁名单
	AddToRejectList          bool                        // 超过阈值的IP加入到限制列表
	RejectTTL                int64                       // 限制访问的时间
	BlockSeconds             int64                       // 封禁时长，秒，0-不封禁
	RequestNumPerSecond      int                         // 限制请求数
	RequestNumPerSecondPerIp int                         // 限制请求数
	WindowsNum               int                         // 样本窗口个数
	Counter                  *WindowLeapArray            // 滑动时间窗口计数
	CounterMu                sync.Mutex                  // 全局锁
	IpCounter                map[string]*WindowLeapArray // 滑动时间窗口计数，基于IP
	IpCounterMu              []*sync.Mutex
}

var windowsNum = 10 // 样本窗口个数，设置为10
var bucketNum = 256

// NewLimiter 实例化
//
// requestNumPerSecond 每秒允许的请求数
func NewLimiter(requestNumPerSecond int, requestNumPerSecondPerIp int) *Limiter {
	lm := &Limiter{
		RequestNumPerSecond:      requestNumPerSecond,
		RequestNumPerSecondPerIp: requestNumPerSecondPerIp,
		WindowsNum:               windowsNum,
		IpCounter:                make(map[string]*WindowLeapArray),
		IpCounterMu:              make([]*sync.Mutex, bucketNum),
		Counter:                  NewWindowLeapArray(windowsNum),
		WhiteList:                NewSet[string](),
		BlackList:                NewSet[string](),
		RejectList:               NewRejectList(),
		RejectTTL:                60,
		AddToRejectList:          true,
	}

	go func() {
		for {
			time.Sleep(1 * time.Minute)

			lm.clear()
		}
	}()

	return lm
}

func (lm *Limiter) Limit(w http.ResponseWriter, req *http.Request) error {
	nowtime := time.Now().Unix()

	if lm.RequestNumPerSecond == 0 {
		return errors.New("服务器繁忙，请稍后再试.")
	}

	// 全局校验
	if !lm.counterCheck() {
		return errors.New("服务器繁忙，请稍后再试.")
	}

	clientIp := ClientIp(req)
	if clientIp == "" {
		return errors.New("未获取到客户端IP.")
	}

	if lm.WhiteList.Exists(clientIp) {
		return nil
	}

	if lm.BlackList.Exists(clientIp) {
		return errors.New("当前IP：" + clientIp + "已被加入到黑名单.")
	}

	// 按IP校验
	tim, exists := lm.RejectList.Get(clientIp)
	if exists {
		if tim > nowtime {
			return errors.New("当前IP：" + clientIp + "已被限制访问.")
		} else {
			lm.RejectList.Remove(clientIp)
		}
	} else {
		if !lm.ipCounterCheck(clientIp) {
			if lm.AddToRejectList && lm.RejectTTL > 0 {
				lm.RejectList.Add(clientIp, nowtime+lm.RejectTTL)
			}
			return errors.New("当前IP：" + clientIp + "访问过于频繁，请稍后再试.")
		}
	}

	return nil
}

func (lm *Limiter) counterCheck() bool {
	lm.CounterMu.Lock()
	defer lm.CounterMu.Unlock()

	return !lm.Counter.Check(lm.RequestNumPerSecond)
}

func (lm *Limiter) getIpCounterMu(clientIp string) *sync.Mutex {
	return lm.IpCounterMu[crc32.ChecksumIEEE([]byte(clientIp))%uint32(bucketNum)]
}

func (lm *Limiter) ipCounterCheck(clientIp string) bool {
	if clientIp == "" {
		return false
	}
	if lm.RequestNumPerSecondPerIp > 0 {
		mu := lm.getIpCounterMu(clientIp)
		mu.Lock()
		defer mu.Unlock()

		ipchecker, ok := lm.IpCounter[clientIp]
		if !ok {
			ipchecker = NewWindowLeapArray(lm.WindowsNum)
			lm.IpCounter[clientIp] = ipchecker
		}
		if !ipchecker.Check(lm.RequestNumPerSecondPerIp) {
			return false
		}
	}
	return true
}

func (lm *Limiter) AddWhiteList(wl []string) {
	lm.WhiteList.AddMore(wl)
}

func (lm *Limiter) RemoveWhiteList(wl []string) {
	lm.WhiteList.RemoveMore(wl)
}

func (lm *Limiter) AddBlackList(bl []string) {
	lm.BlackList.AddMore(bl)
}

func (lm *Limiter) RemoveBlackList(bl []string) {
	lm.BlackList.RemoveMore(bl)
}

func (lm *Limiter) clear() {
	if len(lm.IpCounter) == 0 {
		return
	}

	for clientIp, counter := range lm.IpCounter {
		// 60s
		if time.Now().UnixMilli()-counter.FrontTime >= 60000 {
			lm.deleteIpCounterItem(clientIp)
		}
	}
}

func (lm *Limiter) deleteIpCounterItem(clientIp string) {
	lm.CounterMu.Lock()
	defer lm.CounterMu.Unlock()

	delete(lm.IpCounter, clientIp)
}
