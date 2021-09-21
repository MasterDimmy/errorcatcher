package errorcatcher

import (
	"testing"
	"time"
)

func Test_SendFile(t *testing.T) {
	c := System{Name: "test errcatcher", CollectorUrl: "http://localhost/catch_debug_error", Nick: []string{"ed"}}

	c.Send("first")

	time.Sleep(time.Second)

	c.SendWithFile("test file", []string{"./send.go", "./go.mod"})

	time.Sleep(time.Second)
}
