//go:generate protoc -I $GOPATH/src --go_out=$GOPATH/src $GOPATH/src/github.com/libreoscar/btcwatch/message/schema.proto

package main

import (
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcrpcclient"
	"github.com/golang/protobuf/proto"
	"github.com/libreoscar/btcwatch/message"
	"github.com/libreoscar/dbg/spew"
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

	txs := block.Transactions()

	var processedBlock = &message.ProcessedBlock{
		int32(blockNum),
		make([]*message.ProcessedTx, len(txs)),
	}

	logger.Info("Processing txs...")
	for txIndex, tx := range txs {
		vouts := tx.MsgTx().TxOut
		result := make([]*message.TxResult, len(vouts))
		for i, vout := range vouts {
			addr := NewAddrFromPkScript(vout.PkScript, true)
			if addr != nil {
				result[i] = &message.TxResult{
					&message.TxResult_Transfer{
						&message.ValueTransfer{
							addr.String(),
							int32(vout.Value),
						},
					},
				}
			} else {
				result[i] = &message.TxResult{
					&message.TxResult_Msg{
						&message.OpReturnMsg{string(decodePkScript(vout.PkScript))},
					},
				}
			}
		}
		processedBlock.Txs[txIndex] = &message.ProcessedTx{
			tx.Sha().String(),
			result,
		}
	}
	spew.Dump(processedBlock)
	data, err := proto.Marshal(processedBlock)
	if err != nil {
		logger.Crit(err.Error())
	} else {
		logger.Info("Publish to ZMQ...")
		spew.Dump(data)
		sender.SendBytes(data, 0)
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

	wg.Add(1)

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

	// Start ZMQ server for braft
	sender, err = zmq.NewSocket(zmq.PUB)
	defer sender.Close()
	if err != nil {
		logger.Crit(err.Error())
	}
	sender.Bind("tcp://*:8001")
	logger.Info("ZMQ server started...")

	wg.Wait()
}
