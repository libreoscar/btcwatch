package main

import (
	"github.com/btcsuite/btcrpcclient"
	"github.com/davecgh/go-spew/spew"
	"log"
)

func main() {
	connCfg := &btcrpcclient.ConnConfig{
		Host:         "localhost:18332",
		User:         "braft",
		Pass:         "braft",
		HTTPPostMode: true,
		DisableTLS:   true,
	}

	client, err := btcrpcclient.New(connCfg, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer client.Shutdown()

	// Get the current block count.
	info, err := client.GetInfo()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Bitcoind Info: %v", spew.Sdump(info))
}
