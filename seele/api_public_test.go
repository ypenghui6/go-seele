/**
*  @file
*  @copyright defined in go-seele/LICENSE
 */
package seele

import (
	"bytes"
	"context"
	"math/big"
	"os"
	"path/filepath"
	"testing"

	api2 "github.com/seeleteam/go-seele/api"
	"github.com/seeleteam/go-seele/common"
	"github.com/seeleteam/go-seele/common/hexutil"
	"github.com/seeleteam/go-seele/core/state"
	"github.com/seeleteam/go-seele/core/types"
	"github.com/seeleteam/go-seele/crypto"
	"github.com/seeleteam/go-seele/log"
	"github.com/stretchr/testify/assert"
)

func Test_PublicSeeleAPI(t *testing.T) {
	conf := getTmpConfig()
	serviceContext := ServiceContext{
		DataDir: filepath.Join(common.GetTempFolder(), ".PublicSeeleAPI"),
	}

	var key interface{} = "ServiceContext"
	ctx := context.WithValue(context.Background(), key, serviceContext)
	dataDir := ctx.Value("ServiceContext").(ServiceContext).DataDir
	log := log.GetLogger("seele")
	ss, err := NewSeeleService(ctx, conf, log)
	if err != nil {
		t.Fatal()
	}

	api := NewPublicSeeleAPI(ss)
	defer func() {
		api.s.Stop()
		os.RemoveAll(dataDir)
	}()
	var info api2.GetMinerInfo
	info, err = api.GetInfo()
	assert.Equal(t, err, nil)
	if !bytes.Equal(conf.SeeleConfig.Coinbase[0:], info.Coinbase[0:]) {
		t.Fail()
	}
}

func Test_GetLogs(t *testing.T) {
	dbPath := filepath.Join(common.GetTempFolder(), ".GetLogs")
	api := newTestAPI(t, dbPath)
	defer func() {
		api.s.Stop()
		os.RemoveAll(dbPath)
	}()

	// Create a simple_storage_1 contract
	bytecode, _ := hexutil.HexToBytes("0x6080604052601760005534801561001557600080fd5b5061025f806100256000396000f30060806040526004361061004c576000357c0100000000000000000000000000000000000000000000000000000000900463ffffffff16806360fe47b1146100515780636d4ce63c1461007e575b600080fd5b34801561005d57600080fd5b5061007c600480360381019080803590602001909291905050506100a9565b005b34801561008a57600080fd5b5061009361011b565b6040518082815260200191505060405180910390f35b7fe84bb31d4e9adbff26e80edeecb6cf8f3a95d1ba519cf60a08a6e6f8d62d81006040518080602001828103825260078152602001807f6765744c6f67320000000000000000000000000000000000000000000000000081525060200191505060405180910390a18060008190555050565b60007f978acaf30839c63aff19afed19ff8f3a430103773a67e3890aa1639af9a71bc433604051808273ffffffffffffffffffffffffffffffffffffffff1673ffffffffffffffffffffffffffffffffffffffff16815260200180602001828103825260068152602001807f6765744c6f6700000000000000000000000000000000000000000000000000008152506020019250505060405180910390a17f523b2fb716b59c8e374bb3ea0f14ce672f9ac295b25470c403ad377306abb1026040518080602001828103825260078152602001807f6765744c6f67310000000000000000000000000000000000000000000000000081525060200191505060405180910390a161022b60106100a9565b6000549050905600a165627a7a72305820e12478ad92a5a4181935da97e24de739c4928ac47b2c5c1cd3423513298c62390029")
	statedb, _ := api.s.chain.GetCurrentState()
	from := getFromAddress(statedb)
	createContractTx, _ := types.NewContractTransaction(from, big.NewInt(0), big.NewInt(1), 0, bytecode)
	contractAddressByte := sendTx(t, api, statedb, createContractTx)

	// Get the contract address
	contractAddressHex := hexutil.BytesToHex(contractAddressByte)
	contractAddress, err := common.HexToAddress(contractAddressHex)
	assert.Equal(t, err, nil)

	// The origin statedb
	statedbOri, err := api.s.chain.GetCurrentState()
	assert.Equal(t, err, nil)

	// Call the get function
	msg, err := hexutil.HexToBytes("0x6d4ce63c")
	assert.Equal(t, err, nil)
	getTx, err := types.NewMessageTransaction(from, contractAddress, big.NewInt(0), big.NewInt(10), 1, msg)
	assert.Equal(t, err, nil)
	receipt, err := api.s.chain.ApplyTransaction(getTx, 0, api.s.miner.GetCoinbase(), statedbOri, api.s.chain.CurrentBlock().Header)
	assert.Equal(t, err, nil)

	// Save the statedb and receipts
	batch := api.s.accountStateDB.NewBatch()
	block := api.s.chain.CurrentBlock()
	block.Header.StateHash, _ = statedb.Commit(batch)
	batch.Commit()
	api.s.chain.GetStore().PutReceipts(block.HeaderHash, []*types.Receipt{receipt})

	// Verify the result
	payload := "0xe84bb31d4e9adbff26e80edeecb6cf8f3a95d1ba519cf60a08a6e6f8d62d8100"
	result, err := api.GetLogs(-1, contractAddress.ToHex(), payload)
	assert.Equal(t, err, nil)
	assert.Equal(t, len(result), 1)
	assert.Equal(t, result[0].Txhash, receipt.TxHash)
	addr := result[0].Log.Address
	assert.Equal(t, addr, contractAddress)
	name := result[0].Log.Topics
	assert.Equal(t, name[0].ToHex(), payload)

	// Verify the invalid contractAddress and payload
	result, err = api.GetLogs(-1, "contractAddress.ToHex()", payload)
	assert.Equal(t, err == nil, false)
	result, err = api.GetLogs(-1, contractAddress.ToHex(), "payload")
	assert.Equal(t, err == nil, false)
}

func newTestAPI(t *testing.T, dbPath string) *PublicSeeleAPI {
	conf := getTmpConfig()
	serviceContext := ServiceContext{
		DataDir: dbPath,
	}
	var key interface{} = "ServiceContext"
	ctx := context.WithValue(context.Background(), key, serviceContext)
	log := log.GetLogger("seele")
	ss, err := NewSeeleService(ctx, conf, log)
	assert.Equal(t, err, nil)
	return NewPublicSeeleAPI(ss)
}

func sendTx(t *testing.T, api *PublicSeeleAPI, statedb *state.Statedb, tx *types.Transaction) []byte {
	receipt, err := api.s.chain.ApplyTransaction(tx, 0, api.s.miner.GetCoinbase(), statedb, api.s.chain.CurrentBlock().Header)
	assert.Equal(t, err, nil)
	assert.Equal(t, receipt.Failed, false)

	// Save the statedb
	batch := api.s.accountStateDB.NewBatch()
	block := api.s.chain.CurrentBlock()
	block.Header.StateHash, _ = statedb.Commit(batch)
	block.Header.Height++
	block.Header.PreviousBlockHash = block.HeaderHash
	block.HeaderHash = block.Header.Hash()
	api.s.chain.GetStore().PutBlock(block, big.NewInt(1), true)
	batch.Commit()
	return receipt.ContractAddress
}

func getFromAddress(statedb *state.Statedb) common.Address {
	from := *crypto.MustGenerateRandomAddress()
	statedb.CreateAccount(from)
	statedb.SetBalance(from, common.SeeleToFan)
	statedb.SetNonce(from, 0)
	return from
}
