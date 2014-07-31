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
		APIKey:       "6e00c08c10fa060ce20db15943ac1063",
		ReleaseStage: "development",
	})
}
