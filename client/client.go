package main

import (
	"fmt"
	"time"

	"github.com/golang/protobuf/proto"
	"github.com/libreoscar/btcwatch/message"
	"github.com/davecgh/go-spew/spew"
	zmq "github.com/pebbe/zmq4"
)

func main() {
	receiver, _ := zmq.NewSocket(zmq.SUB)
	defer receiver.Close()
	err := receiver.Connect("tcp://188.166.253.98:8001")
	if err != nil {
		fmt.Println("failed to connect server")
		return
	}
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
