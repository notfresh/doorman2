package main

import (
	"context"
	"fmt"
	dm "github.com/notfresh/doorman2/go/client/doorman"
	dmrl "github.com/notfresh/doorman2/go/ratelimiter"
	//"github.com/notfresh/doorman2/ratelimiter"
	rpc "google.golang.org/grpc"
	"time"
)

func main() {
	//limiter := ratelimiter.NewBucketWithRate(1, 10)
	//count := 0
	//for {
	//	if count > 20 {
	//		break
	//	}
	//	take := limiter.TakeAvailable(1) // 不停的去拿，如果没有拿到，就无限尝试
	//	if take > 0 {
	//		now := time.Now()
	//		fmt.Println("take 1 token", now.Format("2006-01-02 15:04:05"), "count is", count)
	//		count++
	//	}
	//}
	rpcTimeout := 2 * time.Second
	// zx: 先连接一个非master，再次试试能否跳转到master
	client, err := dm.NewWithID("127.0.0.1:5100", "test_client",
		dm.DialOpts(rpc.WithInsecure(), rpc.WithTimeout(rpcTimeout)))
	if err != nil {
		fmt.Println("Fail to new Client ", err)
	}

	res, err := client.Resource("proportional", 20)
	if err != nil {
		fmt.Println("Fail to get resource ", err)
	}
	time.Sleep(time.Second * 3)
	rl := dmrl.NewQPS(res)
	//time.Sleep(time.Millisecond * 800)
	count := 0
	ctx := context.Background()
	for count < 40 {
		now := time.Now() // now.Format("2006-01-02 15:04:05")
		fmt.Println("take 1 token", now, "count is", count)
		count++
		rl.Wait(ctx)
	}

}
