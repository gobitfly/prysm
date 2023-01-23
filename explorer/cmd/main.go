package main

import (
	"flag"
	"os"

	"github.com/ethereum/go-ethereum/rpc"

	runtime "github.com/banzaicloud/logrus-runtime-formatter"
	"github.com/prysmaticlabs/prysm/v3/api/client/beacon"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/state"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/explorer/stateprocessor"
	"github.com/prysmaticlabs/prysm/v3/explorer/tracer"
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

	var s state.BeaconState
	for i := uint64(0); i < 48000; i++ {
		var d map[types.ValidatorIndex]*tracer.ValidatorEpochData
		var err error

		s, d, err = stateprocessor.GetEpochData(s, *network, *epoch+i, clClient)

		if err != nil {
			logrus.Fatal(err)
		}

		// logrus.Infof("retrieved %v data items", len(d))

		// for _, validator := range d {
		// 	for index, epochData := range validator {
		// 		for slot, proposed := range epochData.Proposals {
		// 			if proposed {
		// 				logrus.Infof("validator %v proposed slot %v", index, slot)
		// 			} else {
		// 				logrus.Infof("validator %v missed slot %v", index, slot)
		// 			}
		// 		}
		// 	}
		// }

		// for _, ed := range d[57] {
		// 	logrus.Info(ed)
		// }

		logrus.Info(d[757])

		logrus.Infof("done, state is at slot %v", s.Slot())
	}
}
