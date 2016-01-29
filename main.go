package main

import (
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcrpcclient"
	"github.com/davecgh/go-spew/spew"
	"github.com/libreoscar/utils/log"
	"io"
	"net/http"
	"os"
)

var client *btcrpcclient.Client
var logger = log.New(log.DEBUG)

func loadConf() *btcrpcclient.ConnConfig {
	file, _ := os.Open("conf.json")
	decoder := json.NewDecoder(file)
	rpcConf := &btcrpcclient.ConnConfig{}
	err := decoder.Decode(rpcConf)
	if err != nil {
		fmt.Println("error:", err)
	}
	rpcConf.HTTPPostMode = true
	rpcConf.DisableTLS = true
	return rpcConf
}

func checkBlock(client *btcrpcclient.Client, blockNum int64) {
	blockHash, err := client.GetBlockHash(blockNum)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	block, err := client.GetBlock(blockHash)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	for _, tx := range block.Transactions() {
		fmt.Printf("Tx ID: %v\n", tx.Sha())
		for _, vout := range tx.MsgTx().TxOut {
			addr := NewAddrFromPkScript(vout.PkScript, true)
			spew.Dump(addr)
		}
		fmt.Println("===========")
	}
}

func blockNotify(w http.ResponseWriter, r *http.Request) {
	logger.Info("Received new block!")
	blockNum, err := client.GetBlockCount()
	if err != nil {
		io.WriteString(w, "bitcoind rpc failed\n")
	}
	checkBlock(client, blockNum)
}

func main() {
	connCfg := loadConf()
	var err error

	client, err = btcrpcclient.New(connCfg, nil)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	defer client.Shutdown()

	// // Get the current block count.
	// info, err := client.GetInfo()
	// if err != nil {
	// 	log.Fatal(err)
	// }
	// log.Printf("Bitcoind Info: %v", spew.Sdump(info))

	http.HandleFunc("/block", blockNotify)
	logger.Info("Starting server...")
	err = http.ListenAndServe(":8000", nil)
	if err != nil {
		logger.Crit(err.Error())
	}
}
