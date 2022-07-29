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

// Command line client for Doorman.
//
// Usage:
// doorman_client --server=localhost:9999 --resource=test_resource --wants=100 --client_id=foo

package main

import (
	"errors"
	"flag"
	"fmt"
	"math/rand"
	"time"

	log "github.com/golang/glog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/notfresh/doorman2/go/client/doorman"
)

var (
	server   = flag.String("server", "", "Address of the doorman server")
	resource = flag.String("resource", "", "Name of the resource to request capacity for")
	wants    = flag.Float64("wants", 0, "Amount of capacity to request")

	clientID = flag.String("client_id", "", "Client id to use")
	caFile   = flag.String("ca_file", "", "The file containning the CA root cert file to connect over TLS (otherwise plain TCP will be used)")
)

func main() {
	flag.Parse()

	if *server == "" || *resource == "" {
		log.Exit("both --server and --resource must be specified")
	}

	if *clientID == "" {
		log.Exit("--client_id must be set")
	}

	var opts []grpc.DialOption
	if len(*caFile) != 0 {
		var creds credentials.TransportAuthenticator
		var err error
		creds, err = credentials.NewClientTLSFromFile(*caFile, "")
		if err != nil {
			log.Exitf("Failed to create TLS credentials %v", err)
		}

		opts = append(opts, grpc.WithTransportCredentials(creds))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	client, err := doorman.NewWithID(*server, *clientID, doorman.DialOpts(opts...))

	if err != nil {
		log.Exitf("could not create client: %v", err)
	}

	defer client.Close()
	resource_name := *resource
	resource, err := client.Resource(*resource, *wants)

	if err != nil {
		log.Exitf("could not acquire resource: %v", err)
	}

	fmt.Println(<-resource.Capacity())
	wantValue := *wants
	for {
		randValue, _ := rand_float(0, 0.2)
		wantValue += wantValue * float64(randValue) * float64(rand_fector()) //
		fmt.Printf("new want value, %v %v\n", resource_name, wantValue)
		resource.Ask(wantValue)
		println("tick...")
		time.Sleep(5 * time.Second)
	}

}

// 0到1之间的范围
func rand_float(begin, end float32) (float64, error) {
	if begin < 0 && end < 0 {
		return 0, errors.New("范围有误")
	}
	rand.Seed(time.Now().UnixNano())
	begin_new, end_new := int(begin*100), int(end*100)
	num := rand.Intn(end_new-begin_new) + begin_new
	return float64(num) / 100, nil
}

// 返回负一或者正一
func rand_fector() int {
	rand.Seed(time.Now().UnixNano())
	x := rand.Intn(2) - 1
	if x == 0 {
		return 1
	} else {
		return -1
	}
}
