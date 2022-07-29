package flagenv

import (
	"flag"
	"os"
	"testing"
)

func TestPopulate(t *testing.T) {

	var ( // zx create a flagSet and add three key value pairs
		fs  = flag.NewFlagSet("testing", flag.ExitOnError)
		foo = fs.String("foo", "", "")
		bar = fs.String("bar", "", "")
		baz = fs.String("baz", "", "")
	)
	fs.Parse([]string{"-foo=foo", "-baz=baz"}) // zx what's this for?

	os.Clearenv() // zx OS create some value to check
	os.Setenv("DOORMAN_BAR", "bar")
	os.Setenv("DOORMAN_BAZ", "baz from env")

	if err := Populate(fs, "DOORMAN"); err != nil {
		t.Fatal(err)
	}

	if got, want := *foo, "foo"; got != want {
		t.Errorf("foo=%q; want %q", got, want)
	}
	if got, want := *bar, "bar"; got != want {
		t.Errorf("bar=%q; want %q", got, want)
	}
	if got, want := *baz, "baz"; got != want {
		t.Errorf("baz=%q; want %q", got, want)
	}
}
