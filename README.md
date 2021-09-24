"# errorcatcher" 

Write software errors to Rocket Chat via middleware (not provided)

Usage:

```
	package errorcatcher

import (
	"testing"
)

	func Test_SendFile(t *testing.T) {
		c := System{Name: "test errcatcher", CollectorUrl: "http://localhost/catch_debug_error", 		Nick: []string{"whom to send"}}

	c.Send("first")
	c.Wait()

	c.SendWithFile("test file", []string{"./send.go", "./go.mod"})
	c.Wait()
}

```
