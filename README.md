"# errorcatcher" 

Writes software errors to chat

Usage:

```
	include "modules/errorcatcher"

	var error_catcher = errorcatcher.System{Name: "some name", CollectorUrl:"url_to_catch_debug_error", Nick: "whom"}

	error_catcher.Send("some text of error")
```
