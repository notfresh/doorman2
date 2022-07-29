package zxratelimiter

import (
	"fmt"
	"testing"
	"time"
)

func TestNewAdaptiveBucket(t *testing.T) {
	rl := NewAdaptiveBucket(10, 1, time.Second, 2)
	for i := 0; i < 20; i++ {
		rl.Wait(1)
		fmt.Println(time.Now(), "i is ", i, "LastWants ", rl.LastWants)
	}
	time.Sleep(time.Second)
}

func TestEntries_GetWants(t *testing.T) {
	e := &entries{
		timeRange: time.Second,
		winSize:   3,
	}
	t1 := time.Now()
	e.Record(t1)
	t2 := t1.Add(time.Second * -1)
	e.Record(t2)
	t3 := t1.Add(time.Second / 2 * -1)
	e.Record(t3)
	t4 := t1.Add(time.Second * -2)
	e.Record(t4)
	fmt.Println("e.wants ", e.GetWants())
}

// we predefine a request count sequence, so every period
// the request came as a fix number, so what will be the expected `Wants` number?
func TestNewAdaptiveBucketWithReqeustSequence(t *testing.T) {
	limiter := NewAdaptiveBucket(10, 1, time.Second, 3)
	requestSeries := []int{10, 0, 8, 6, 5, 7, 10}
	for j := 0; j < len(requestSeries); j++ {
		fmt.Println(" ###################", "The round", j, "request count ", requestSeries[j])
		startTime := time.Now()
		nextRound := startTime.Add(time.Second)
		fmt.Println("startTime, nextRound ", startTime, nextRound)
		for i := 0; i < requestSeries[j] && time.Now().Before(nextRound); i++ {
			limiter.Wait(1)
			fmt.Println("now time is ", time.Now(), ", i is ", i)
		}

		//now2 := now1
		for time.Now().Before(nextRound) {
			//if now1.Sub(now2) >= time.Second/2 {
			//	fmt.Println("Info Waiting for next loop, time is ", now1)
			//	now2 = now1
			//}
		}
	}
	time.Sleep(time.Second)

}
