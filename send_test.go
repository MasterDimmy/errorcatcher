package errorcatcher

import (
	"fmt"
	"testing"
	"time"
)

/*
 	Writes software errors to chat

	usage:

	include "modules/errorcatcher"
	var error_catcher = errorcatcher.System{Name: "some name", CollectorUrl:"url_to_catch_debug_error", Nick: "who"}

	error_catcher.Send("some text of error")
*/

func Test_sender(t *testing.T) {
	c := System{Name: "test errcatcher", CollectorUrl: "http://localhost/catch_debug_error", Nick: "ed"}

	c.Send("first")

	time.Sleep(time.Second)

	for i := 0; i < 5; i++ {
		c.Send("merge this " + fmt.Sprintf("%d", i))
	}
	time.Sleep(time.Second)
}
