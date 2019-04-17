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
	// "gopkg.in/mgo.v2/bson"
	"github.com/ethereum/go-ethereum/mongo"
	"time"
	"encoding/json"
)



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
	// print("transaction hash is ", tx.Hash().Hex(), "\n")
	mongo.CurrentTx = tx.Hash().Hex()
	mongo.TraceGlobal = ""
	mongo.TxVMErr = ""

	msg, err := tx.AsMessage(types.MakeSigner(config, header.Number))
	if err != nil {
		return nil, 0, err
	}

	// Create a new context to be used in the EVM environment
	context := NewEVMContext(msg, header, bc, author)

	toaddr := ""
	if msg.To() == nil {
		toaddr = "0x0"
	} else {
		tempt := *msg.To()
		toaddr = tempt.String()
	}

	// write transaction to the array
	mongo.BashTxs[mongo.CurrentNum] = mongo.Transac{statedb.BlockHash().Hex(), header.Number.String(), 
					msg.From().String(), fmt.Sprintf("%d", tx.Gas()), tx.GasPrice().String(), 
					tx.Hash().Hex(), hexutil.Encode(tx.Data()), fmt.Sprintf("0x%x", tx.Nonce()), 
					fmt.Sprintf("0x%x", tx.R()), fmt.Sprintf("0x%x", tx.S()), toaddr, 
					fmt.Sprintf("0x%x", statedb.TxIndex()), fmt.Sprintf("0x%x", tx.V()), msg.Value().String()}

	// Create a new environment which holds all relevant information
	// about the transaction and calling mechanisms.
	// vmenv := vm.NewEVM(context, statedb, config, cfg)
	vmenv := vm.NewEVMWithFlag(context, statedb, config, cfg, false)

	// Apply the transaction to the current state (included in the env)
	// Double clean the trace to prevent duplications
	mongo.TraceGlobal = ""
	_, gas, failed, err := ApplyMessage(vmenv, msg, gp)

	// write trace to the array
	mongo.BashTrs[mongo.CurrentNum] = mongo.Trace{tx.Hash().Hex(), mongo.TraceGlobal}

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

	// write receipt to the array
	re_final_log := ""
	for i := 0; i < len(receipt.Logs); i++ {
	        res, _ := json.Marshal(receipt.Logs[i])
	        re_final_log = fmt.Sprintf("%s\n%s", re_final_log, string(res))
	}
	mongo.BashRes[mongo.CurrentNum] = mongo.Rece{receipt.ContractAddress.String(), fmt.Sprintf("%d", receipt.CumulativeGasUsed),
			fmt.Sprintf("%d", receipt.GasUsed), re_final_log, fmt.Sprintf("0x%x", receipt.Bloom.Big()), fmt.Sprintf("0x%d", receipt.Status), 
			receipt.TxHash.Hex(), mongo.TxVMErr}

	// bash write bash number of transactions, receipts and traces into the db
	if mongo.CurrentNum != mongo.BashNum - 1 {
		mongo.CurrentNum = mongo.CurrentNum + 1
	} else {
		start := time.Now()
		session  := mongo.SessionGlobal.Clone()
		defer func() { session.Close() }()
		db_tx := session.DB("geth").C("transaction")
		db_tr := session.DB("geth").C("trace")
		db_re := session.DB("geth").C("receipt")		

		session_err := db_tx.Insert(mongo.BashTxs...)
		if session_err != nil {
			// panic(session_err)
			// WriteTxsInLoop(mongo.BashTxs)
			for i := 0; i < mongo.BashNum; i++ {
				 // Write the transaction into db
				 session_err = db_tx.Insert(&mongo.BashTxs[i]) 
				 if session_err != nil {
					mongo.ErrorFile.WriteString(fmt.Sprintf("Transaction %s\n", session_err))
			         }
			 }
			// mongo.ErrorFile.WriteString(fmt.Sprintf("%s\n", session_err))
		}
		session_err = db_tr.Insert(mongo.BashTrs...)
		if session_err != nil {
			// panic(session_err)
			// WriteTrsLoop(mongo.BashTrs)
			for i := 0; i < mongo.BashNum; i++ {
				// Write the trace into db
				session_err = db_tr.Insert(&mongo.BashTrs[i])
				if session_err != nil {
	                                   mongo.ErrorFile.WriteString(fmt.Sprintf("Trace %s\n", session_err))
				 }																			}
			// mongo.ErrorFile.WriteString(fmt.Sprintf("%s\n", session_err))
		}
		session_err = db_re.Insert(mongo.BashRes...)
		if session_err != nil {
			// panic(session_err)
			// WriteResInLoop(mongo.BashRes)
			for i := 0; i < mongo.BashNum; i++ {
				// Write the receipt into db
				session_err = db_re.Insert(&mongo.BashRes[i])
				if session_err != nil {
					 mongo.ErrorFile.WriteString(fmt.Sprintf("Receipt %s\n", session_err))
				}
			}
			// mongo.ErrorFile.WriteString(fmt.Sprintf("%s\n", session_err))
		}

		mongo.CurrentNum = 0
		mongo.BashTxs = make([]interface{}, mongo.BashNum)
		mongo.BashTrs = make([]interface{}, mongo.BashNum)
		mongo.BashRes = make([]interface{}, mongo.BashNum)
		print("state process db time is ", fmt.Sprintf("%s", time.Since(start)) , "\n")
	}

	return receipt, gas, err
}
