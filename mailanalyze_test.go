package mailanalyze

import (
	"fmt"
	"os"
	"testing"
	"github.com/davecgh/go-spew/spew"
)


func TestDumpTs(t *testing.T) {
	tf,e := os.Open("T")
	if e!=nil { return }
	ns,e := tf.Readdirnames(-1)
	tf.Close()
	if e!=nil {
		return
	}
	for _,n := range ns {
		fmt.Println("Processing ",n)
		f,e := os.Open("T/"+n)
		if e!=nil { continue }
		res,e := Analyze(f)
		if e!=nil {
			fmt.Println("ERROR: ", e)
		} else {
			spew.Dump(res)
//			fmt.Println(res.Subject)
			_ = res
		}
		f.Close()
	}
}

var D = spew.Dump
