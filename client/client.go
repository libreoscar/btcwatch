package main

import (
	"fmt"
	"time"

	"github.com/dyzz/gobtclib/message"
	"github.com/golang/protobuf/proto"
	"github.com/libreoscar/dbg/spew"
	zmq "github.com/pebbe/zmq4"
)

func main() {
	receiver, _ := zmq.NewSocket(zmq.SUB)
	defer receiver.Close()
	receiver.Connect("tcp://localhost:8001")
	receiver.SetSubscribe("")

	for {
		for {
			data, err := receiver.RecvBytes(0)
			if err != nil {
				fmt.Println(err)
				// 'resource is temporarily unavailable' error because the
				// underlying libzmq reports EAGAIN when in NOBLOCK mode
				break
			}
			//  process msg
			fmt.Println("Got message!")
			processedBlock := &message.ProcessedBlock{}
			proto.Unmarshal(data, processedBlock)
			spew.Dump(processedBlock)
		}
		//  No activity, so sleep for 1 millisecond before checking again
		time.Sleep(time.Millisecond)
	}
}
