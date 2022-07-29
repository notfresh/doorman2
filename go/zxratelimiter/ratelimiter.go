package zxratelimiter

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"
)

type RateLimiter interface {
	TakeAvailable(count int64) int64
	Wait(count int64)
}

type Bucket struct {
	startTime time.Time
	capacity  int64

	// quantum holds how many tokens are added on
	// each tick.
	quantum int64 // zx this is to control the add rate

	// fillInterval holds the interval between each tick.
	fillInterval time.Duration

	// mu guards the fields below it.
	mu sync.Mutex

	availableTokens int64

	// latestTick holds the latest tick for which
	// we know the number of tokens in the bucket.
	latestTick     int64
	buckeTimeRange time.Duration // zx block time unit
}

// rate from specified rate. 1% seems reasonable. // zx just intuition
const rateMargin = 0.01

func NewBucketWithRate(rate float64, capacity int64, timeRange time.Duration) RateLimiter {
	if capacity <= 0 { // at least 1
		panic("token bucket capacity must > 0")
	}
	if rate < 0 {
		panic("token buket rate must >= 0 ")
	}
	tb := &Bucket{
		startTime:       time.Now(),
		latestTick:      0,
		fillInterval:    1, // zx this will be updated
		capacity:        capacity,
		quantum:         1,
		availableTokens: 0,
		buckeTimeRange:  timeRange,
	}
	// zx try to make each fillInterval 1 quantum, but if the interval is too small that near 0,
	// zx make quantum larger, this happens only if the rate is too large.
	// zx this is to update the fillInterval and quantum.
	for quantum := int64(1); quantum < 1<<50; quantum = nextQuantum(quantum) {
		fillInterval := time.Duration(float64(timeRange) / rate * float64(quantum))
		if fillInterval <= 0 {
			continue
		}
		tb.fillInterval = fillInterval
		tb.quantum = quantum
		if diff := math.Abs(tb.Rate() - rate); diff/rate <= rateMargin {
			return tb
		}
	}
	panic("cannot find suitable quantum for " + strconv.FormatFloat(rate, 'g', -1, 64))
}

const infinityDuration time.Duration = 0x7fffffffffffffff

func (tb *Bucket) Take(count int64) time.Duration {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	d, _ := tb.take(time.Now(), count, time.Hour*9999)
	return d
}

func (tb *Bucket) take(now time.Time, count int64, maxWait time.Duration) (time.Duration, bool) {
	if count <= 0 {
		return 0, true
	}
	tick := tb.currentTick(now)
	tb.adjustavailableTokens(tick)
	avail := tb.availableTokens - count
	if avail > 0 {
		tb.availableTokens = avail
		return 0, true
	}
	endTick := tick + (-avail-1)/tb.quantum + 1
	endTime := tb.startTime.Add(time.Duration(endTick) * tb.fillInterval)
	waitTime := endTime.Sub(now)
	if waitTime > maxWait {
		return 0, false
	}
	tb.availableTokens = avail // zx
	return waitTime, true
}

func (tb *Bucket) Wait(count int64) {
	if d := tb.Take(count); d > 0 {
		time.Sleep(d)
	}
}

func nextQuantum(q int64) int64 {
	q1 := q * 11 / 10
	if q1 == q {
		q1++
	}
	return q1
}

func (tb *Bucket) TakeAvailable(count int64) int64 { // zx this is useful most
	tb.mu.Lock()
	defer tb.mu.Unlock()
	return tb.takeAvailable(time.Now(), count)
}

func (tb *Bucket) takeAvailable(now time.Time, count int64) int64 { // zx take time and count
	if count < 0 {
		return 0
	}
	tb.adjustavailableTokens(tb.currentTick(now))
	if tb.availableTokens <= 0 {
		return 0
	}
	if count > tb.availableTokens {
		count = tb.availableTokens
	}
	tb.availableTokens -= count
	return count
}

func (tb *Bucket) adjustavailableTokens(tick int64) {
	// zx, tick diff can only multiply the quantum. this is very important.
	tb.availableTokens += (tick - tb.latestTick) * tb.quantum
	if tb.availableTokens >= tb.capacity {
		tb.availableTokens = tb.capacity
	}
	tb.latestTick = tick
	return
}

// Capacity returns the capacity that the bucket was created with.
func (tb *Bucket) Capacity() int64 {
	return tb.capacity
}

// Rate returns the fill rate of the bucket, in tokens per second.
func (tb *Bucket) Rate() float64 {
	return float64(tb.buckeTimeRange) * float64(tb.quantum) / float64(tb.fillInterval)
	// zx
}

func (tb *Bucket) currentTick(now time.Time) int64 { // zx this take the current time and return the tick number
	return int64(now.Sub(tb.startTime) / tb.fillInterval) // zx, this is core, the start time should not change
	// zx if we use now.sub(LastRequest), then we will never add the current tick
}

// entries is used to record entry times to rate limiter's Wait method.
type entries struct {
	times     []time.Time
	timeRange time.Duration // statistics period
	winSize   int
}

// Record records a new entry to rate limiter's Wait method.
// zx : append time, but for what?
func (e *entries) Record(entry time.Time) {
	e.times = append(e.times, entry)
}

// Clear removes old events: ones which happened more than specified
// window ago.
// zx remove the old ones if that small than window
// zx window is a duration, not an exact time.
func (e *entries) Clear(window time.Duration) {
	fmt.Println("Call clear")
	//for i := 0; i < len(e.times); i++ {
	//	fmt.Println("e.times[]", i, e.times[i])
	//}

	//j := 0
	i := sort.Search(len(e.times), func(i int) bool {
		//j = i
		return time.Since(e.times[i]) < window
	})
	//fmt.Println("Clear j is ", j)
	e.times = e.times[i:]
}

func max(a, b int) int {
	if a >= b {
		return a
	}
	return b
}

// GetWants calculates wants capacity based on number of entries recorded
// during "window" duration.
func (e *entries) GetWants() float64 {
	// Get rid of old events.
	e.Clear(time.Duration(e.winSize) * e.timeRange)

	// frequency keeps information about how many events happened
	// in a particular second (within "window" last seconds).
	frequency := make(map[int]int)

	// Calculate number of events per every second.
	// zx remember, every call of Wait will put a Time in e.times
	for _, entry := range e.times {
		frequency[int(time.Since(entry)/e.timeRange)]++ // zx in a reverse time
	}
	for k, v := range frequency {
		fmt.Println("frequency k,v", k, v)
	}

	// Calculate the following sum: for every second within window
	// we multiply the number of events that occured in this particular
	// second by the second's weight. The weight given to a second is
	// proportional to its recency (with interval of 10 seconds, the most
	// recent second will have a weight of 10, while the 10th second will
	// have the weigth of 1).

	var want int
	for i, n := 0, e.winSize; i < n; i++ { // zx windows' second
		//sum += frequency[i] * (n - i) // zx n is the total seconds, sum take the weight into accounting.
		want = max(want, frequency[i])
	} // zx in a reverse time
	// zx len's the square, and take the split.
	return float64(want)
}

type AdaptiveQPS struct {
	ratelimiter RateLimiter // zx the interface
	//resource    doorman.Resource
	entry chan time.Time // zx ? a channel
	quit  chan bool
	//opts        *adaptiveOptions
	window     time.Duration
	timeRange  time.Duration
	windowSize int
	LastWants  float64
	stat       entries
}

func NewAdaptiveBucket(rate float64, capacity int64, timeRange time.Duration, winSize int) *AdaptiveQPS {
	arl := &AdaptiveQPS{
		ratelimiter: NewBucketWithRate(rate, capacity, timeRange), // zx this include the QPSRateLimiter
		entry:       make(chan time.Time),
		quit:        make(chan bool),
		window:      timeRange * time.Duration(winSize),
		timeRange:   timeRange,
		windowSize:  winSize,
		LastWants:   -1,
	}
	arl.stat = entries{
		winSize:   winSize,
		timeRange: timeRange,
	}
	fmt.Println("NewAdaptiveBucket  window is ", arl.window)
	go arl.run() // zx this is a very important method
	return arl   // zx adaptive
}

func (arl *AdaptiveQPS) Wait(count int64) {
	now := time.Now()
	for i := int64(0); i < count; i++ {
		arl.stat.Record(now)
	}
	arl.ratelimiter.Wait(count)
}

func (arl *AdaptiveQPS) TakeAvailable(count int64) int64 {
	now := time.Now()
	for i := int64(0); i < count; i++ {
		arl.stat.Record(now)
	}
	take := arl.ratelimiter.TakeAvailable(count)
	return take
}

func (arl *AdaptiveQPS) run() {
	// entries is used to record entry times to Wait method.

	for {
		//fmt.Println("Go run adaptive ")
		select {
		case <-time.After(arl.timeRange): // zx this is understandable
			wants := arl.stat.GetWants() // zx this is the core method
			arl.LastWants = wants
			fmt.Println("####################")
			fmt.Println("Info get wants ", wants)
			fmt.Println("####################")
		case <-arl.quit: // zx close
			// Stop receiving entry time records and exit.
			close(arl.entry)
			return
		}
	}
}

// the first edition
//func main() {
//	limiter := NewBucketWithRate(60, 1, time.Minute)
//	i := 0
//	for {
//		if i >= 10 {
//			break
//		}
//		limiter.Wait(1)
//
//		fmt.Println("now time is ", time.Now(), ", i is ", i)
//		if i == 0 {
//			time.Sleep(time.Second * 3) // j
//		}
//		i++
//		//n := limiter.TakeAvailable(1)
//		//if n > 0 {
//		//	fmt.Println("now time is ", time.Now())
//		//	i++
//		//}
//	}
//}

func timeNearEqual(time1 time.Time, time2 time.Time) bool {
	if time1.After(time2) {
		return time1.Sub(time2) <= time.Millisecond
	}
	return time2.Sub(time1) <= time.Millisecond
}
