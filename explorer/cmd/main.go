package main

import (
	"flag"
	"os"

	"github.com/davecgh/go-spew/spew"
	"github.com/ethereum/go-ethereum/rpc"

	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	"github.com/prysmaticlabs/prysm/v3/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v3/explorer/stateprocessor"
	"github.com/sirupsen/logrus"
)

func main() {

	formatter := runtime.Formatter{ChildFormatter: &logrus.TextFormatter{
		FullTimestamp: true,
	}}
	formatter.Line = true
	logrus.SetFormatter(&formatter)
	logrus.SetOutput(os.Stdout)
	logrus.SetLevel(logrus.InfoLevel)

	clNode := flag.String("cl-node", "http://localhost:4000", "CL Node API Endpoint")
	elNode := flag.String("el-node", "http://localhost:8545", "EL Node API Endpoint")
	network := flag.String("network", "", "Config to use (can be mainnet, prater or sepolia")
	epoch := flag.Uint64("epoch", 1, "Epoch to calculate rewards for")
	flag.Parse()

	clClient, err := beacon.NewClient(*clNode)
	if err != nil {
		logrus.Fatal(err)
	}

	_, err = rpc.Dial(*elNode)
	if err != nil {
		logrus.Fatal(err)
	}

	logrus.Infof("network: %v", *network)
	logrus.Infof("epoch: %v", *epoch)

	d := &stateprocessor.EpochData{}

	for i := uint64(0); i < 200; i++ {
		var err error

		d, err = stateprocessor.GetEpochData(d.State, *network, *epoch+i, clClient)

		if err != nil {
			logrus.Fatal(err)
		}

		spew.Dump(d.Validators[2227])

		logrus.Infof("epoch %v processed, state is at slot %v, previous epoch active is %v, previous epoch participated is %v", *epoch+i, d.State.Slot(), d.PreviousEpochActiveGWei, d.PreviousEpochVotedGWei)
	}
}
