package main

import (
	//	"encoding/binary"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcrpcclient"
	"github.com/davecgh/go-spew/spew"
	"github.com/libreoscar/utils/log"
	zmq "github.com/pebbe/zmq4"
	"io"
	"net/http"
	"os"
	"sync"
)

var client *btcrpcclient.Client
var sender *zmq.Socket
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

func getInfo(client *btcrpcclient.Client) {
	// getinfo demo
	info, err := client.GetInfo()
	if err != nil {
		logger.Crit(err.Error())
	}
	logger.Info(fmt.Sprintf("Bitcoind Info: %v", spew.Sdump(info)))

}

func decodePkScript(script []byte) (message []byte) {
	if len(script) < 3 || len(script) > 82 || script[0] != 0x6a {
		return nil
	} else {
		size := int(script[1])
		if size != len(script)-2 {
			return nil
		}
		return script[2:]
	}
}

func buildZmqMsg(result []interface{}) string {
	return ""
}

func checkBlock(client *btcrpcclient.Client, blockNum int64) {
	blockHash, err := client.GetBlockHash(653895)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	block, err := client.GetBlock(blockHash)
	if err != nil {
		logger.Crit(err.Error())
		return
	}

	txs := block.Transactions()

	logger.Info("Processing txs...")
	for _, tx := range txs {
		var msg = fmt.Sprintf("Tx ID: %v\n", tx.Sha())
		logger.Info(msg)
		vouts := tx.MsgTx().TxOut
		result := make([]interface{}, len(vouts))
		for i, vout := range vouts {
			addr := NewAddrFromPkScript(vout.PkScript, true)
			msg = spew.Sdump(addr)
			logger.Info(msg)
			if addr != nil {
				result[i] = struct {
					addr  *BtcAddr
					value int64
				}{
					addr,
					vout.Value,
				}
			} else {
				result[i] = decodePkScript(vout.PkScript)
			}
		}
		spew.Dump(result)
		sender.Send(buildZmqMsg(result), 0)
	}
	logger.Info("Process done.")
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

	var wg sync.WaitGroup

	wg.Add(2)

	http.HandleFunc("/block", blockNotify)
	logger.Info("Starting server...")

	// Start http server for bitcoind
	go func() {
		defer wg.Done()
		err = http.ListenAndServe("127.0.0.1:8000", nil)
		if err != nil {
			logger.Crit(err.Error())
		}
	}()

	go func() {
		defer wg.Done()
		sender, err = zmq.NewSocket(zmq.PUB)
		defer sender.Close()
		if err != nil {
			logger.Crit(err.Error())
		}
		sender.Bind("tcp://*:8001")
		logger.Info("ZMQ server started...")
	}()

	wg.Wait()

}
