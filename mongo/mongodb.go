package mongo

import (
	"gopkg.in/mgo.v2"
)

var SessionGlobal *mgo.Session
var TraceGlobal string
var CurrentTx string
var TxVMErr string

func InitMongoDb() {
	var err error
    if SessionGlobal, err = mgo.Dial(""); err != nil {
        panic(err)
    }
}
