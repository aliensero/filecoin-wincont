package main

import (
	"fmt"
	"log"
	"os"

	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	"github.com/urfave/cli/v2"
)

func main() {

	local := []*cli.Command{
		runCmd,
	}
	app := &cli.App{
		Name:     "lotus-wincount",
		Commands: append(local, lcli.CommonCommands...),
	}

	err := app.Run(os.Args)
	if err != nil {
		log.Fatal(err)
	}

}

var runCmd = &cli.Command{
	Name:  "run",
	Usage: "must set environment FULLNODE_API_INFO",
	Flags: []cli.Flag{
		&cli.Uint64Flag{
			Name:  "begin",
			Value: 0,
		},
		&cli.Uint64Flag{
			Name:  "end",
			Value: 0,
		},
		&cli.StringFlag{
			Name: "actor",
		},
	},
	Action: func(cctx *cli.Context) error {
		api, closer, err := lcli.GetFullNodeAPI(cctx)
		if err != nil {
			return err
		}
		defer closer()

		addr, err := address.NewFromString(cctx.String("actor"))
		if err != nil {
			return err
		}

		bEpoch := cctx.Uint64("begin")
		eEpoch := cctx.Uint64("end")
		for i := bEpoch; i <= eEpoch; i++ {
			round := abi.ChainEpoch(i)
			ts, err := api.ChainGetTipSetByHeight(cctx.Context, abi.ChainEpoch(i-1), types.EmptyTSK)
			if err != nil {
				log.Println(err)
				continue
			}

			mbi, err := api.MinerGetBaseInfo(cctx.Context, addr, round, ts.Key())
			if err != nil {
				log.Println(err)
				continue
			}
			beaconPrev := mbi.PrevBeaconEntry
			bvals := mbi.BeaconEntries
			rbase := beaconPrev
			if len(bvals) > 0 {
				rbase = bvals[len(bvals)-1]
			}
			winner, err := gen.IsRoundWinner(cctx.Context, ts, round, addr, rbase, mbi, api)
			if err != nil {
				log.Println(err)
				continue
			}
			if winner != nil {
				fmt.Printf("actor %v heigth %d wincount %d\n", addr, i, winner.WinCount)
			}
		}

		return nil
	},
}
