package mongo

import (
	"gopkg.in/mgo.v2"
)

var SessionGlobal *mgo.Session
var TraceGlobal string
var CurrentTx string
var TxVMErr string

var DbTx *mgo.Collection
var DbTr *mgo.Collection
var DbRe *mgo.Collection

func InitMongoDb() {
	var err error
    if SessionGlobal, err = mgo.Dial(""); err != nil {
        panic(err)
    }
	// Open the transaction collection
	DbTx = SessionGlobal.DB("geth").C("transaction")
	// Open the trace collection
	DbTr = SessionGlobal.DB("geth").C("trace")
	// Open the receupt collection
	DbRe = SessionGlobal.DB("geth").C("receipt")
}