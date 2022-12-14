// Package flagenv allows setting flags from the command line.
package flagenv

import (
	"flag"
	"fmt"
	"os"
	"strings"

	log "github.com/golang/glog"
)

// NOTE(ryszard): This code is heavily inspired by
// https://github.com/coreos/etcd/blob/master/pkg/flags/flag.go.

// Populate sets the flags in set from the environment. The
// environment value used will be the name of the flag in upper case,
// with '-' changed to '_', and (if prefixis not the empty string)
// prepended with prefix and an underscore. So, if prefix is
// "DOORMAN", and the flag's name "foo-bar", the environment variable
// DOORMAN_FOO_BAR will be used.
// zx this function will populate the？
func Populate(set *flag.FlagSet, prefix string) error {
	var (
		setThroughFlags = make(map[string]bool) // zx what's this?
		knownEnv        = make(map[string]bool) // zx record the value
		err             error
	)
	// zx this apply a function to every element , only those that have been set.
	set.Visit(func(f *flag.Flag) { // zx set is not a common Set
		setThroughFlags[f.Name] = true
	})

	set.VisitAll(func(f *flag.Flag) { // zx visit all flags
		key := flagToEnv(prefix, f.Name) // zx update the f.name
		knownEnv[key] = true
		val := os.Getenv(key) // zx get the updated value from operating system
		if val == "" {
			return
		}

		if setThroughFlags[f.Name] { // zx this parts is read from  command line
			log.Warningf("Recognized environment variable %v, but shadowed by flag %v: won't be used.", key, f.Name)
			return
		}
		if e := set.Set(f.Name, val); e != nil {
			err = fmt.Errorf("Invalid value %q for %v.", val, key)
			return
		}
	})

	for _, env := range os.Environ() {
		kv := strings.SplitN(env, "=", 2)
		if len(kv) < 1 {
			continue
		}
		if name := kv[0]; strings.HasPrefix(name, prefix) && !knownEnv[name] {
			log.Warningf("Unrecognized environment variable %s", name)
		}
	}

	return err
}

// zx update the name
func flagToEnv(prefix, name string) string {
	rest := strings.ToUpper(strings.Replace(name, "-", "_", -1)) // zx replace all the - with _
	if prefix != "" {
		return prefix + "_" + rest
	}
	return rest
}
