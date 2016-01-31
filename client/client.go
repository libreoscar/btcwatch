package main

import (
	"fmt"
	"time"

	zmq "github.com/pebbe/zmq4"
)

func main() {
	receiver, _ := zmq.NewSocket(zmq.SUB)
	defer receiver.Close()
	receiver.Connect("tcp://localhost:8001")
	receiver.SetSubscribe("")

	for {
		for {
			msg, err := receiver.Recv(0)
			if err != nil {
				// 'resource is temporarily unavailable' error because the
				// underlying libzmq reports EAGAIN when in NOBLOCK mode
				break
			}
			//  process msg
			fmt.Printf("Got msg: '%s'\n", msg)
		}
		//  No activity, so sleep for 1 millisecond before checking again
		time.Sleep(time.Millisecond)
	}
}
