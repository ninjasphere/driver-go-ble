// +build !release

package main

import (
	"github.com/bugsnag/bugsnag-go"
	"github.com/juju/loggo"
	"github.com/ninjasphere/go-ninja/logger"
)

var debug = logger.GetLogger("").Warningf

func init() {
	logger.GetLogger("").SetLogLevel(loggo.DEBUG)

	bugsnag.Configure(bugsnag.Configuration{
		APIKey:       "7b66d4013bdcd0541287fd9b00376253",
		ReleaseStage: "development",
	})
}
