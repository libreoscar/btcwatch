package main

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg"
	"github.com/btcsuite/btcd/wire"
	"github.com/btcsuite/btcrpcclient"
	"github.com/btcsuite/btcutil"
	"github.com/codegangsta/cli"
	"github.com/libreoscar/dbg/spew"
	"github.com/libreoscar/utils/log"
	"os"
	"strconv"
)

var _ = spew.Dump

var (
	FEE       = 0.0002
	MAX_BYTES = 80
	client    *btcrpcclient.Client
	logger    = log.New(log.DEBUG)
	err       error
	isTestnet = false
	sendTx    = false
)

func posString(slice []string, element string) int {
	for index, elem := range slice {
		if elem == element {
			return index
		}
	}
	return -1
}

func containsString(slice []string, element string) bool {
	return !(posString(slice, element) == -1)
}

func askForConfirmation(msg string) bool {
	fmt.Printf("%s Type yes or no:[Y/N]", msg)

	var response string
	_, err := fmt.Scanln(&response)
	if err != nil {
		logger.Crit(err.Error())
	}
	okayResponses := []string{"y", "Y", "yes", "Yes", "YES"}
	nokayResponses := []string{"n", "N", "no", "No", "NO"}
	if containsString(okayResponses, response) {
		return true
	} else if containsString(nokayResponses, response) {
		return false
	} else {
		fmt.Println("Please type yes or no and then press enter:")
		return askForConfirmation(msg)
	}
}

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
	rpcConf.HTTPPostMode = true
	rpcConf.DisableTLS = true
	return rpcConf
}

func messageToHex(msg wire.Message) (string, error) {
	var buf bytes.Buffer
	if err := msg.BtcEncode(&buf, 70002); err != nil {
		return "", fmt.Errorf(fmt.Sprintf("Failed to encode msg of type %T", msg))
	}
	return hex.EncodeToString(buf.Bytes()), nil
}

func toSatoshi(f float64) int64 {
	return int64(f * 1e8)
}

type selectInputsResult struct {
	total  float64
	inputs []btcjson.ListUnspentResult
}

func selectInputs(totalAmount float64) (*selectInputsResult, error) {
	unspents, err := client.ListUnspent()
	if err != nil {
		return nil, err
	}
	inputAmount := 0.0
	var inputs []btcjson.ListUnspentResult
	for _, unspent := range unspents {
		if !unspent.Spendable {
			continue
		}
		inputs = append(inputs, unspent)
		inputAmount += unspent.Amount
		if inputAmount >= totalAmount {
			break
		}
	}
	if inputAmount < totalAmount {
		return nil, fmt.Errorf("not enough funds")
	}
	return &selectInputsResult{
		total:  inputAmount,
		inputs: inputs,
	}, nil
}

func createTx(inputs *selectInputsResult, addr btcutil.Address, amount float64, change float64, msg []byte) *wire.MsgTx {
	tx := wire.NewMsgTx()
	txIns := make([]*wire.TxIn, len(inputs.inputs))
	for i, input := range inputs.inputs {
		hash, err := wire.NewShaHashFromStr(input.TxID)
		if err != nil {
			logger.Crit("invalid txid")
			os.Exit(0)
		}
		prevOut := wire.NewOutPoint(hash, input.Vout)
		txIn := wire.NewTxIn(prevOut, nil)
		txIns[i] = txIn
	}

	txOuts := make([]*wire.TxOut, 3)

	result, _ := client.ValidateAddress(addr)
	if !result.IsValid {
		logger.Crit("invalid address")
		os.Exit(0)
	}
	addrPkScript := result.ScriptPubKey
	addrPkScriptBin, _ := hex.DecodeString(addrPkScript)
	txOut := wire.NewTxOut(toSatoshi(amount), addrPkScriptBin)
	txOuts[0] = txOut

	changePkScript := inputs.inputs[0].ScriptPubKey
	changePKScriptBin, _ := hex.DecodeString(changePkScript)
	txOut = wire.NewTxOut(toSatoshi(change), changePKScriptBin)
	txOuts[1] = txOut

	msgLen := len(msg)

	var payload bytes.Buffer
	payload.WriteByte(0x6a)
	if msgLen <= 75 {
		payload.WriteByte(byte(msgLen))
	} else if msgLen <= 256 {
		payload.WriteByte(0x4c)
		payload.WriteByte(byte(msgLen))
	} else {
		payload.WriteByte(0x4d)
		payload.WriteByte(byte(msgLen % 256))
		payload.WriteByte(byte(msgLen / 256))
	}
	payload.Write(msg)
	opReturnBin := payload.Bytes()

	txOut = wire.NewTxOut(0, opReturnBin)
	txOuts[2] = txOut

	tx.TxIn = txIns
	tx.TxOut = txOuts

	var buf bytes.Buffer
	tx.Serialize(&buf)
	logger.Info("Created Tx:")
	logger.Info(spew.Sdump(hex.EncodeToString(buf.Bytes())))

	return tx
}

func sendOpReturn(addr string, totalAmount float64, msg []byte) {
	if len(msg) > MAX_BYTES {
		logger.Crit("message oversize")
		os.Exit(0)
	}
	btcAddr, err := btcutil.DecodeAddress(addr, &chaincfg.MainNetParams)
	if err != nil {
		logger.Crit("can't decode address")
		os.Exit(0)
	}
	logger.Info("finding avaible inputs")
	inputs, err := selectInputs(totalAmount + FEE)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	change := inputs.total - totalAmount - FEE
	rawtx := createTx(inputs, btcAddr, totalAmount, change, msg)

	if sendTx {
		signedTx, complete, err := client.SignRawTransaction(rawtx)
		if err != nil || !complete {
			logger.Crit(fmt.Sprintf("could not sign the tx: %s", err.Error()))
			os.Exit(0)
		} else {
			var rawtx bytes.Buffer
			signedTx.Serialize(&rawtx)
			decodedTx, _ := client.DecodeRawTransaction(rawtx.Bytes())
			logger.Info(spew.Sdump(decodedTx))

			askForConfirmation("Are you going to send the tx? ")
			txHash, err := client.SendRawTransaction(signedTx, false)
			if err != nil {
				logger.Crit("could not send the tx")
				os.Exit(0)
			} else {
				logger.Info(fmt.Sprintf("tx sent: %s", txHash.String()))
			}
		}
	}
}

func main() {
	conf := loadConf()
	client, err = btcrpcclient.New(conf, nil)
	if err != nil {
		logger.Crit(err.Error())
		return
	}
	defer client.Shutdown()

	app := cli.NewApp()
	app.Name = "Go OP_Return"
	app.Flags = []cli.Flag{
		cli.BoolFlag{
			Name: "testnet",
		},
		cli.BoolFlag{
			Name:  "real",
			Usage: "send tx to btc network",
		},
	}
	app.Commands = []cli.Command{
		{
			Name:  "send",
			Usage: "send op_return tx to address",
			Action: func(c *cli.Context) {
				if len(c.Args()) < 3 {
					fmt.Println("send addr amount msg")
					return
				}
				addr := c.Args().First()
				amount, err := strconv.ParseFloat(c.Args().Get(1), 64)
				if err != nil {
					logger.Crit("error parsing amount")
					return
				}
				msg := []byte(c.Args().Get(2))
				logger.Info(fmt.Sprintf("crafting tx to %s with amount %v, msg \"%s\"", addr, amount, msg))
				sendOpReturn(addr, amount, msg)
			},
		},
	}
	app.Before = func(c *cli.Context) error {
		if c.GlobalBool("testnet") {
			isTestnet = true
		}
		if isTestnet {
			logger.Info("using testnet")
		} else {
			logger.Info("using mainchain")
		}
		if c.GlobalBool("real") {
			sendTx = true
		}
		if sendTx {
			logger.Info("tx will be sent to bitcoind")
		}
		return nil
	}
	app.Run(os.Args)
}
