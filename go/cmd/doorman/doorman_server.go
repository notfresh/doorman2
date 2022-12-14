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

package main

import (
	"flag"
	"fmt"
	"github.com/ghodss/yaml"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/net/context"

	log "github.com/golang/glog"
	rpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"

	"github.com/notfresh/doorman2/go/configuration"
	"github.com/notfresh/doorman2/go/connection"
	"github.com/notfresh/doorman2/go/flagenv"
	"github.com/notfresh/doorman2/go/server/doorman"
	"github.com/notfresh/doorman2/go/server/election"
	"github.com/notfresh/doorman2/go/status"

	pb "github.com/notfresh/doorman2/proto/doorman"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	_ "expvar"
	_ "net/http/pprof"
)

var (
	port = flag.Int("port", 6000, "port to bind to")
	// FIXME(ryszard): As of Jan 21, 2016 it's impossible to serve
	// both RPC and HTTP traffic on the same port. This should be
	// fixed by grpc/grpc-go#75. When that happens, remove
	// debugPort.
	debugPort  = flag.Int("debug_port", 6050, "port to bind for HTTP debug info")
	serverRole = flag.String("server_role", "root", "Role of this server in the server tree")
	parent     = flag.String("parent", "", "Address of the parent server which this server connects to")
	hostname   = flag.String("hostname", "", "Use this as the hostname (if empty, use whatever the kernel reports")
	config     = flag.String("config", "", "source to load the config from (text protobufs)")

	rpcDialTimeout = flag.Duration("doorman_rpc_dial_timeout", 5*time.Second, "timeout to use for connecting to the doorman server")

	minimumRefreshInterval = flag.Duration("doorman_minimum_refresh_interval", 5*time.Second, "minimum refresh interval")

	tls      = flag.Bool("tls", false, "Connection uses TLS if true, else plain TCP")
	certFile = flag.String("cert_file", "", "The TLS cert file")
	keyFile  = flag.String("key_file", "", "The TLS key file")

	etcdEndpoints      = flag.String("etcd_endpoints", "", "comma separated list of etcd endpoints")
	masterDelay        = flag.Duration("master_delay", 10*time.Second, "delay in master elections")
	masterElectionLock = flag.String("master_election_lock", "", "etcd path for the master election or empty for no master election")
)

var (
	statusz = `
<h2>Mastership</h2>
<p>
{{if .IsMaster}}
  This <strong>is</strong> the master.
{{else}}
This is <strong>not</strong> the master.
  {{with .CurrentMaster}}
    The current master is <a href="http://{{.}}">{{.}}</a>
  {{else}}
    The current master is unknown.
  {{end}}
{{end}}
</p>
{{with .Election}}{{.}}{{end}}

<h2>Resources</h2>
{{ with .Resources }}
<table border="1">
  <thead>
    <tr>
      <td>ID</td>
      <td>Capacity</td>
      <td>SumHas</td>
      <td>SumWants</td>
      <td>Clients</td>
      <td>Learning</td>
      <td>Algorithm</td>
    </tr>
  </thead>
  {{range .}}
  <tr>
    <td><a href="/debug/resources?resource={{.ID}}">{{.ID}}</a></td>
    <td>{{.Capacity}}</td>
    <td>{{.SumHas}}</td>
    <td>{{.SumWants}}</td>
    <td>{{.Count}}</td>
    <td>{{.InLearningMode}}
    <td><code>{{.Algorithm}}</code></td>
  </tr>
  {{end}}
</table>
{{else}}
No resources in the store.
{{end}}

<h2>Configuration</h2>
<pre>{{.Config}}</pre>
`
)

// getServerID returns a unique server id, consisting of a host:port id.
func getServerID(port int) string {
	if *hostname != "" {
		return fmt.Sprintf("%s:%d", *hostname, port)
	}
	hn, err := os.Hostname() // zx if there is no host name , take the os name

	if err != nil {
		hn = "unknown.localhost"
	}

	return fmt.Sprintf("%s:%d", hn, port)
}

func main() {
	log_dir_path := "./doorman_log_dir"
	if _, err := os.Stat(log_dir_path); os.IsNotExist(err) {
		log.Infoln("??????????????????" + log_dir_path)
		if err := os.Mkdir(log_dir_path, os.ModePerm); err != nil {
			log.Exit(err)
		}
	}
	flag.Lookup("log_dir").Value.Set(log_dir_path) // zx ? @hard how to set log dir
	// zx log_dir is the directory that will keep the log,but parsed by flags
	if err := flagenv.Populate(flag.CommandLine, "DOORMAN"); err != nil {
		log.Exit(err)
	}
	flag.Parse()

	if *config == "" {
		log.Exit("--config cannot be empty")
	}
	// zx go get can create the binary
	// zx: doorman: $GOPATH/bin/doorman -config=./config.yml -port=$PORT
	// -debug_port=$(expr $PORT + 50) -etcd_endpoints=http://localhost:2379
	// -master_election_lock=/doorman.master -hostname=localhost
	// -log_dir="./doorman_log_dir"
	var (
		etcdEndpointsSlice = strings.Split(*etcdEndpoints, ",") // zx strings is a good tool.
		masterElection     election.Election
	)
	if *masterElectionLock != "" {

		if len(etcdEndpointsSlice) == 1 && etcdEndpointsSlice[0] == "" { // zx log.Exit == return, but log
			log.Exit("-etcd_endpoints cannot be empty if -master_election_lock is provided")
		}

		masterElection = election.Etcd(etcdEndpointsSlice, *masterElectionLock, *masterDelay)
	} else {
		masterElection = election.Trivial() // zx this is ?
	}

	// zx:???????????????????????????
	dm, err := doorman.New(context.Background(), getServerID(*port), *parent, masterElection,
		connection.MinimumRefreshInterval(*minimumRefreshInterval),
		connection.DialOpts(
			rpc.WithTimeout(*rpcDialTimeout)))
	if err != nil {
		log.Exitf("doorman.NewIntermediate: %v", err)
	}

	var opts []rpc.ServerOption
	if *tls {
		log.Infof("Loading credentials from %v and %v.", *certFile, *keyFile)
		creds, err := credentials.NewServerTLSFromFile(*certFile, *keyFile)
		if err != nil {
			log.Exitf("Failed to generate credentials %v", err)
		}
		opts = []rpc.ServerOption{rpc.Creds(creds)}
	}
	server := rpc.NewServer(opts...) // zx what's this? the server is a business unrelated server

	pb.RegisterCapacityServer(server, dm) // zx the register link the business and the bottom server

	if *config == "" {
		log.Exit("-config cannot be empty")
	}

	// zx: ?????????????????????file?????????????????????etcd???????????????
	var cfg configuration.Source
	kind, path := configuration.ParseSource(*config) // zx take the config
	switch {
	case kind == "file":
		cfg = configuration.LocalFile(path)
	case kind == "etcd":
		if len(etcdEndpointsSlice) == 1 && etcdEndpointsSlice[0] == "" {
			log.Exit("-etcd_endpoints cannot be empty if a config source etcd is provided")
		}
		cfg = configuration.Etcd(path, etcdEndpointsSlice)
	default:
		panic("unreachable")
	}

	// Try to load the background. If there's a problem with loading
	// the server for the first time, the server will keep running,
	// but will not serve traffic.
	go func() {
		for { // zx loop, but why?
			data, err := cfg(context.Background())
			if err != nil {
				log.Errorf("cannot load config data: %v", err)
				continue
			}
			cfg := new(pb.ResourceRepository)
			// zx: ???????????????, decode, from json string to struct, cfg is a struct instance
			if err := yaml.Unmarshal(data, cfg); err != nil {
				log.Errorf("cannot unmarshal config data: %q", data)
				continue
			}
			// zx:??????doorman, ????????????????????????
			if err := dm.LoadConfig(context.Background(), cfg, map[string]*time.Time{}); err != nil {
				log.Errorf("cannot load config: %v", err)
			}
		}
	}()

	status.AddStatusPart("Doorman", statusz, func(context.Context) interface{} { return dm.Status() })

	// Redirect form / to /debug/status.
	http.Handle("/", http.RedirectHandler("/debug/status", http.StatusMovedPermanently))
	AddServer(dm)

	http.Handle("/metrics", promhttp.Handler())
	log.Info(fmt.Sprintf("Server listen on port %v", *debugPort))
	go http.ListenAndServe(fmt.Sprintf(":%v", *debugPort), nil)

	// Waits for the server to get its initial configuration. This guarantees that
	// the server will never run without a valid configuration.
	log.Info("Waiting for the server to be configured...")
	dm.WaitUntilConfigured()

	// Runs the server.
	log.Info("Server is configured, ready to go!")

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", *port))
	if err != nil {
		log.Exit(err)
	}

	server.Serve(lis) // zx server is an rpc.server instance

}
