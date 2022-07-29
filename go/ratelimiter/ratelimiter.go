// Copyright 2016 Google, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ratelimiter

import (
	"math"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"github.com/notfresh/doorman2/go/client/doorman"
)

// RateLimiter is a rate limiter that works with Doorman resources.
type RateLimiter interface {
	// Wait blocks until the appropriate operation runs or an error occurs.
	Wait(ctx context.Context) error

	// Close closes the rate limiter.
	Close()
}

// qpsRateLimiter is the implementation of rate limiter interface
// for QPS as the resource.
type qpsRateLimiter struct {
	// resource that this rate limiter is limiting.
	resource doorman.Resource

	// quit is used to notify that rate limiter is to be closed.
	quit chan bool

	// events is used by Wait to request a channel it should be waiting on from run.
	// This will be a channel on which receive will succeed immediately, if the rate
	// limiter is unlimited, or the main unfreeze channel otherwise.
	events chan chan chan bool

	// interval indicates period of time once per which the
	// rate limiter releases goroutines waiting for the access to the resource.
	interval time.Duration

	// rate is a limit at which rate limiter releases waiting goroutines
	// that want to access the resource.
	rate int // zx 按理说，这个是放行频率

	// subintervals is the number of subintervals.
	subintervals int
}

// NewQPS creates a rate limiter connected to the resourse.
func NewQPS(res doorman.Resource) RateLimiter {
	rl := &qpsRateLimiter{
		resource: res, // zx the most important thing
		quit:     make(chan bool),
		events:   make(chan chan chan bool),
	}

	go rl.run()
	return rl
}

// Close closes the rate limiter. It panics if called more than once.
func (rl *qpsRateLimiter) Close() {
	rl.quit <- true
}

// zx interval is a time range , the unit is millsecond
// zx the interval default value is 1000 milliseconds, 1 second
// recalculate calculates new values for rate limit and interval.
func (rl *qpsRateLimiter) recalculate(rate int, interval int) (newRate int, leftoverRate int, newInterval time.Duration) {
	newRate = rate
	// zx interval takes millisecond as unit
	newInterval = time.Duration(interval) * time.Millisecond

	// If the rate limit is more than 2 Hz we are going to divide the given
	// interval to some number of subintervals and distribute the given rate
	// among these subintervals to avoid burstiness which could take place otherwise.
	// zx rate > 1 means more than 2 Hz, 2 times per second, split the rate
	if rate > 1 && interval >= 20 {
		// Try to have one event per subinterval, but don't let subintervals go
		// below 20ms, that pounds on things too hard.

		// zx dont abuse the original coder, it's a convenience
		// zx take 100 rate 1 second as an example, the rate is 100, and the interval is 1000
		// zx the subinterval is 50, every interval got 2
		rl.subintervals = int(math.Min(float64(rate), float64(interval/20))) // zx at least this number

		// zx the new rate is 2 , the leftover is 100 % 50 = 0

		newRate = rate / rl.subintervals
		leftoverRate = rate % rl.subintervals
		// zx 2 * 1000 / 100 = 20, means what? every 20 millisecond, 2 request can be approved
		interval = int(float64(newRate*interval) / float64(rate))
		// zx 200 milliseconds
		newInterval = time.Duration(interval) * time.Millisecond // zx this is a smooth algo
	}
	return
}

// zx 根据capacity更新rate，interval？？？
// update sets rate and interval to be appropriate for the capacity received.
func (rl *qpsRateLimiter) update(capacity float64) (leftoverRate int) {
	switch {
	case capacity < 0:
		rl.rate = -1
	case capacity == 0:
		// We block rate limiter, meaning no access to resource at all.
		rl.rate = 0
	case capacity <= 10:
		// zx recalculate? why 1
		// zx if cap is 5, then rate is 1 and interval is 200
		rl.rate, leftoverRate, rl.interval = rl.recalculate(1, int(1000.0/capacity))
	default:
		// zx the rate is updated, 1000 is united by milliseconds
		rl.rate, leftoverRate, rl.interval = rl.recalculate(int(capacity), 1000)
	}
	return
}

// zx 有个rate属性？小于0就是不限制了。
func (rl *qpsRateLimiter) unlimited() bool {
	return rl.rate < 0
}

// zx 等于0就是不给放行
func (rl *qpsRateLimiter) blocked() bool { // zx block > 0 means common case , < 0 means unblocked
	return rl.rate == 0
}

// run is the rate limiter's main loop. It takes care of receiving
// a new capacity for the resource and releasing goroutines waiting
// for the access to the resource at the calculated rate.
func (rl *qpsRateLimiter) run() {
	var (
		// unfreeze is used to notify waiting goroutines that they can access the resource.
		// run will attempt to send values on unfreeze at the available rate.
		// zx massager?
		unfreeze = make(chan bool)

		// released reflects number of times we released waiting goroutines per original
		// interval.
		released = 0

		// leftoverRate is a rate that left after dividing the original rate by number of subintervals.
		// We need to redistribute it among subintervals so we release waiting goroutines at exactly
		// original rate.
		// zx this is a fucking job
		leftoverRate, leftoverRateOriginal = 0, 0
	)

	for {
		var wakeUp <-chan time.Time // zx wake up channel means something

		// We only need to wake up if the rate limiter is neither blocked
		// nor unlimited. If it is blocked, there is nothing to wake up for.
		// When it is unlimited, we will be sending a non-blocking channel
		// to the waiting goroutine anyway.
		if !rl.blocked() && !rl.unlimited() { // zx common case
			wakeUp = time.After(rl.interval) // zx stop for interval
		}

		select {
		case <-rl.quit:
			// Notify closing goroutine that we're done so it
			// could safely close another channels.
			close(unfreeze)
			return

		case capacity := <-rl.resource.Capacity(): // this update the value
			// Updates rate and interval according to received capacity value.
			leftoverRateOriginal = rl.update(capacity)

			// Set released to 0, as a new cycle of goroutines' releasing begins.
			released = 0
			leftoverRate = leftoverRateOriginal // zx for adjust, and will only be adjusted when the cap is updated
		case response := <-rl.events: // zx when will it happen？Wait call
			// If the rate limiter is unlimited, we send back a channel on which
			// we will immediately send something, unblocking the call to Wait
			// that it sent there.
			// zx do once? no
			if rl.unlimited() {
				nonBlocking := make(chan bool)
				response <- nonBlocking
				nonBlocking <- true
				break
			}
			// zx chan chan bool
			response <- unfreeze // zx when wake up, many true bool value is put into freeze channel
		case <-wakeUp: // zx at the interval
			// Release waiting goroutines when timer is expired.
			max := rl.rate                  // zx this is the new rate
			if released < rl.subintervals { // subinterval means the numbers to cut 1 second to small pieces
				if leftoverRate > 0 {
					stepLeftoverRate := leftoverRate/rl.rate + 1
					max += stepLeftoverRate
					leftoverRate -= stepLeftoverRate
				}
				// zx release is use to count the subinterval that has passed, not the quota
				released++ // zx what's the fuck? every subInterval, add x quota, so let the routine go
			} else {
				// Set released to 0, as a new cycle of goroutines releasing begins.
				released = 0
				leftoverRate = leftoverRateOriginal
			}

			for i := 0; i < max; i++ {
				select {
				case unfreeze <- true: // zx what's this for?
					// We managed to release a goroutine
					// waiting on the other side of the channel.
				default:
					// We failed to send value through channel, so nobody
					// is waiting for the resource now, but we keep trying
					// sending values via channel, because a waiting goroutine
					// could eventually appear before rate is reached
					// and we have to release it.
				}
			}
		}
	}
}

// Wait blocks until a time appropriate operation to run or an error occurs.
func (rl *qpsRateLimiter) Wait(ctx context.Context) error {
	response := make(chan chan bool) // zx 2 level
	rl.events <- response            // zx put in 3 level
	unfreeze := <-response           // zx took out 1 level

	select {
	case <-ctx.Done():
		return ctx.Err()
	case _, ok := <-unfreeze: // zx took for this check
		if !ok {
			return grpc.Errorf(codes.ResourceExhausted, "rate limiter was closed")
		}
		return nil
	}
}
