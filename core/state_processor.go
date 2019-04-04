// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus"
	"github.com/ethereum/go-ethereum/consensus/misc"
	"github.com/ethereum/go-ethereum/core/state"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/core/vm"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/params"

	// Add
	"fmt"
	"github.com/ethereum/go-ethereum/common/hexutil"
 	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// Databse 1, store the basic transaction metadata
type Transac struct {
	Tx_BlockHash string
	Tx_BlockNum string 
	Tx_FromAddr string
	Tx_Gas string
	Tx_GasPrice string
	Tx_Hash string 
	Tx_Input string 
	Tx_Nonce string
	Tx_R string
 	Tx_S string
	Tx_ToAddr string
	Tx_Index string
	Tx_V string
	Tx_Value string
}

// Database 3: receipt
type Rece struct{
	// BlockHash
	// BlockNumber
	Re_contractAddress string
	Re_CumulativeGasUsed string
	// from
	Re_GasUsed string
	Re_Logs string
	Re_LogsBloom string
	Re_Status  string
	// to
	Re_TxHash string
	// TransactionIndex
}


// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus.Engine    // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(config *params.ChainConfig, bc *BlockChain, engine consensus.Engine) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(block *types.Block, statedb *state.StateDB, cfg vm.Config) (types.Receipts, []*types.Log, uint64, error) {
	var (
		receipts types.Receipts
		usedGas  = new(uint64)
		header   = block.Header()
		allLogs  []*types.Log
		gp       = new(GasPool).AddGas(block.GasLimit())
	)
	// Mutate the block and state according to any hard-fork specs
	if p.config.DAOForkSupport && p.config.DAOForkBlock != nil && p.config.DAOForkBlock.Cmp(block.Number()) == 0 {
		misc.ApplyDAOHardFork(statedb)
	}
	// Iterate over and process the individual transactions
	for i, tx := range block.Transactions() {
		statedb.Prepare(tx.Hash(), block.Hash(), i)
		receipt, _, err := ApplyTransaction(p.config, p.bc, nil, gp, statedb, header, tx, usedGas, cfg)
		if err != nil {
			return nil, nil, 0, err
		}
		receipts = append(receipts, receipt)
		allLogs = append(allLogs, receipt.Logs...)
	}
	// Finalize the block, applying any consensus engine specific extras (e.g. block rewards)
	p.engine.Finalize(p.bc, header, statedb, block.Transactions(), block.Uncles(), receipts)

	return receipts, allLogs, *usedGas, nil
}

// ApplyTransaction attempts to apply a transaction to the given state database
// and uses the input parameters for its environment. It returns the receipt
// for the transaction, gas used and an error if the transaction failed,
// indicating the block was invalid.
func ApplyTransaction(config *params.ChainConfig, bc ChainContext, author *common.Address, gp *GasPool, statedb *state.StateDB, header *types.Header, tx *types.Transaction, usedGas *uint64, cfg vm.Config) (*types.Receipt, uint64, error) {
	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}
	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc, author)

	// Record related info
	print("ApplyTransaction Transaction\n")
	print("block hash is ", statedb.BlockHash().Hex(), "\n")
	print("block number is ", header.Number.String(), "\n")
	print("from is ", msg.From().String(), "\n")
	print("gas is ", tx.Gas(), "\n")
	print("gasPrice is ", tx.GasPrice().String(), "\n")
	print("hash is ", tx.Hash().Hex(), "\n")
	print("input is ", hexutil.Encode(tx.Data()), "\n")
	print("Nonce is ", tx.Nonce(), "\n")
	print("r is ", fmt.Sprintf("0x%x", tx.R()), "\n")
	print("s is ", fmt.Sprintf("0x%x", tx.S()), "\n")
	toaddr := ""
	if msg.To() == nil {
		toaddr = "0x0"
	} else {
		tempt := *msg.To()
		toaddr = tempt.String()
	}
	print("to is ", toaddr, "\n")
	print("index is ", statedb.TxIndex(), "\n")
	print("v is ", fmt.Sprintf("0x%x", tx.V()), "\n")
	print("value is, ", msg.Value().String(), "\n")

	// txstring := fmt.Sprintf("%s|%s|%s|%d|%s|%s|%s|%d|%s|%s|%s|%d|%s|%s", statedb.BlockHash().Hex(),
	//		header.Number.String(), msg.From().String(), tx.Gas(), tx.GasPrice().String(), tx.Hash().Hex(),
	//		hexutil.Encode(tx.Data()), tx.Nonce(), fmt.Sprintf("0x%x", tx.R()), fmt.Sprintf("0x%x", tx.S()),
	//		toaddr, statedb.TxIndex(), fmt.Sprintf("0x%x", tx.V()), msg.Value().String())

	session, err := mgo.Dial("")
	if err != nil {
		panic(err)
	}
	defer func() { session.Close() }()

	db_tx := session.DB("geth").C("transaction")
	tx_exist, err := db_tx.Find(bson.M{"tx_hash": tx.Hash().Hex()}).Count()
	if err != nil {
		panic(err)
	}
	if tx_exist == 0 {
		err := db_tx.Insert(&Transac{statedb.BlockHash().Hex(), header.Number.String(), msg.From().String(), fmt.Sprintf("%d", tx.Gas()), tx.GasPrice().String(), tx.Hash().Hex(), hexutil.Encode(tx.Data()), fmt.Sprintf("%d", tx.Nonce()), fmt.Sprintf("0x%x", tx.R()), fmt.Sprintf("0x%x", tx.S()), toaddr, fmt.Sprintf("%d", statedb.TxIndex()), fmt.Sprintf("0x%x", tx.V()), msg.Value().String()})
		if err != nil {
			panic(err)
		}
	}
	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	vmenv := vm.NewEVMWithTx(context, statedb, config, cfg, tx.Hash().Hex())
	// Apply the transaction to the current state (included in the env)
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)
	if err != nil {
		return nil, 0, err
	}
	// Update the state with pending changes
	var root []byte
	if config.IsByzantium(header.Number) {
		statedb.Finalise(true)
	} else {
		root = statedb.IntermediateRoot(config.IsEIP158(header.Number)).Bytes()
	}
	*usedGas += gas

	// Create a new receipt for the transaction, storing the intermediate root and gas used by the tx
	// based on the eip phase, we're passing whether the root touch-delete accounts.
	receipt := types.NewReceipt(root, failed, *usedGas)
	receipt.TxHash = tx.Hash()
	receipt.GasUsed = gas
	// if the transaction created a contract, store the creation address in the receipt.
	if msg.To() == nil {
		receipt.ContractAddress = crypto.CreateAddress(vmenv.Context.Origin, tx.Nonce())
	}
	// Set the receipt logs and create a bloom for filtering
	receipt.Logs = statedb.GetLogs(tx.Hash())
	receipt.Bloom = types.CreateBloom(types.Receipts{receipt})
	receipt.BlockHash = statedb.BlockHash()
	receipt.BlockNumber = header.Number
	receipt.TransactionIndex = uint(statedb.TxIndex())

	print("ApplyTransaction Receipt\n")
	print("contract address is ", receipt.ContractAddress.String(), "\n")
	print("cumulativegasused is ", receipt.CumulativeGasUsed, "\n")
	print("Gasused is ", receipt.GasUsed, "\n")
	print("logs are ", fmt.Sprintf("%s", receipt.Logs), "\n")
	print("logsbloom are ", fmt.Sprintf("0x%x", receipt.Bloom.Big()), "\n")
	print("status is ", receipt.Status, "\n")
	print("txhash is ", receipt.TxHash.Hex(), "\n")

	db_re := session.DB("geth").C("receipt")
	re_exist, err := db_re.Find(bson.M{"tx_hash": receipt.TxHash.Hex()}).Count()
	if err != nil {
        	panic(err)
	}
	if re_exist == 0 {
		err = db_re.Insert(&Rece{receipt.ContractAddress.String(), fmt.Sprintf("%d", receipt.CumulativeGasUsed),
			fmt.Sprintf("%d", receipt.GasUsed), fmt.Sprintf("%s", receipt.Logs),		
			fmt.Sprintf("0x%x", receipt.Bloom.Big()), fmt.Sprintf("0x%d", receipt.Status), 
			receipt.TxHash.Hex()})
	}

	return receipt, gas, err
}
