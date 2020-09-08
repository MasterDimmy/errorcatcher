package errorcatcher

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sync"
	"sync/atomic"
	"time"

	"github.com/valyala/fasthttp"
)

type TCatchedError struct {
	Id     int64
	Nick   string
	Source string
	Text   string
	When   int64
}

type System struct {
	Name string //system name
	Nick string //whom to inform in chat?

	CollectorUrl string //chat connector url

	exename string //name of the executable

	once    sync.Once
	tasks   chan string
	working int64
}

//send error message
func (s *System) Send(text string) {
	s.sender()

	if len(s.Name) == 0 {
		panic("errorcatcher: system name cant be empty")
	}

	if len(s.Nick) == 0 {
		panic("errorcatcher: nick to receive message cant be empty")
	}

	if len(s.CollectorUrl) == 0 {
		panic("errorcatcher: CollectorUrl is nil")
	}

	s.tasks <- text
}

//ensure all tasks sent
func (s *System) Wait() {
	for atomic.LoadInt64(&s.working) == 1 {
		time.Sleep(time.Second)
	}
}

func (s *System) sender() {
	s.once.Do(func() {
		s.tasks = make(chan string, 1000)

		host, _ := os.Hostname()
		s.exename, _ = os.Executable()
		s.exename = host + " - " + path.Base(s.exename)

		client := &fasthttp.Client{}
		client.ReadTimeout = 10 * time.Second
		client.WriteTimeout = 10 * time.Second

		go func() {
			for {
				func() {
					msg := <-s.tasks
					mx := len(msg)
					if mx > 200 {
						msg = msg[:200]
					}
					msg += "\n"
					atomic.StoreInt64(&s.working, 1)
					defer atomic.StoreInt64(&s.working, 0)

					ok := true
					for ok {
						select {
						case t := <-s.tasks:
							mx := len(t)
							if mx > 200 {
								t = t[:200]
							}
							msg += t + "\n"
						default:
							ok = false
						}
					}

					buf, _ := json.Marshal(&TCatchedError{
						Source: s.Name + " (" + s.exename + ")",
						Nick:   s.Nick,
						Text:   msg,
						When:   time.Now().Unix(),
					})

					var b []byte
					p := fasthttp.AcquireArgs()
					defer fasthttp.ReleaseArgs(p)
					p.AddBytesV("data", buf)

					c, data, err := client.Post(b, s.CollectorUrl, p)
					if err != nil || c != 200 || len(data) == 0 || string(data) != "OK" {
						if err != nil {
							fmt.Println(err.Error())
							return
						}
						fmt.Printf("%d : %s\n", c, string(data))
					}
				}()
			}
		}()
	})
}
