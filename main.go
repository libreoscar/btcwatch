//go:generate protoc -I $GOPATH/src --go_out=$GOPATH/src $GOPATH/src/github.com/libreoscar/btcwatch/message/schema.proto

package main

import (
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"
	"github.com/golang/protobuf/proto"
	"github.com/libreoscar/btcwatch/addr"
	"github.com/libreoscar/btcwatch/message"
	"github.com/davecgh/go-spew/spew"
	"github.com/libreoscar/utils/log"
	zmq "github.com/pebbe/zmq4"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
)

var client *btcrpcclient.Client
var sender *zmq.Socket
var logger = log.New(log.DEBUG)
var isTestnet = false

func loadConf() *btcrpcclient.ConnConfig {
	file, err := os.Open("conf.json")
	if err != nil {
		logger.Crit("failed to open \"conf.json\"")
		os.Exit(-1)
	}
	decoder := json.NewDecoder(file)
	rpcConf := &btcrpcclient.ConnConfig{}
	err = decoder.Decode(rpcConf)
	if err != nil {
		logger.Crit(fmt.Sprintf("decode error:%s", err.Error()))
		os.Exit(-1)
	}
	logger.Info(fmt.Sprintf("is testnet:%v", isTestnet))
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
		make([]*message.ProcessedTx, 0),
	}

	logger.Info("Processing txs...")
	start := time.Now()
	var wg sync.WaitGroup
	for txIndex, tx := range txs {
		wg.Add(1)
		go func(txIndex int, tx *btcutil.Tx) {
			defer wg.Done()
			vouts := tx.MsgTx().TxOut
			result := make([]*message.TxResult, len(vouts))
			hasReturn := false
			for i, vout := range vouts {
				btcAddr := addr.NewAddrFromPkScript(vout.PkScript, isTestnet)
				if btcAddr != nil {
					result[i] = &message.TxResult{
						&message.TxResult_Transfer{
							&message.ValueTransfer{
								btcAddr.String(),
								uint64(vout.Value),
							},
						},
					}
				} else {
					msg := decodePkScript(vout.PkScript)
					if msg != nil {
						result[i] = &message.TxResult{
							&message.TxResult_Msg{
								&message.OpReturnMsg{string(msg)},
							},
						}
						hasReturn = true
					}
				}
			}
			if hasReturn {
				processedBlock.Txs = append(processedBlock.Txs,
					&message.ProcessedTx{
						tx.Sha().String(),
						result,
					})
			}
		}(txIndex, tx)
	}
	wg.Wait()
	spew.Dump(processedBlock)
	data, err := proto.Marshal(processedBlock)
	if err != nil {
		logger.Crit(err.Error())
	} else {
		logger.Info("Publish to ZMQ...")
		spew.Dump(data)
		sender.SendBytes(data, 0)
	}
	elapsed := time.Since(start)
	logger.Info(fmt.Sprintf("Process done in %s", elapsed))
	logger.Info(fmt.Sprintf("Block %d has %d OP_Return Txs", blockNum, len(processedBlock.Txs)))
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
