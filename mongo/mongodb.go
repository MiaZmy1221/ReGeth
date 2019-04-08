package mongo

import (
	"gopkg.in/mgo.v2"
)

var SessionGlobal *mgo.Session

func InitMongoDb() {
	var err error
    if SessionGlobal, err = mgo.Dial(""); err != nil {
        panic(err)
    }
}