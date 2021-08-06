package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"

	ffi "github.com/filecoin-project/filecoin-ffi"
	"github.com/filecoin-project/go-address"
	"github.com/filecoin-project/go-state-types/abi"
	"github.com/filecoin-project/go-state-types/crypto"
	"github.com/filecoin-project/lotus/chain/gen"
	"github.com/filecoin-project/lotus/chain/store"
	"github.com/filecoin-project/lotus/chain/types"
	lcli "github.com/filecoin-project/lotus/cli"
	proof5 "github.com/filecoin-project/specs-actors/v5/actors/runtime/proof"
	"github.com/urfave/cli/v2"
	"golang.org/x/xerrors"
)

func main() {

	local := []*cli.Command{
		runCmd,
		verifyCmd,
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
		&cli.StringFlag{
			Name: "path",
		},
		&cli.StringFlag{
			Name:  "out",
			Value: "./winningProofs",
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
				fmt.Printf("actor %v heigth %d wincount %d sectors %v\n", addr, i, winner.WinCount, mbi.Sectors)
				if cctx.IsSet("path") {
					actorID, err := address.IDFromAddress(addr)
					if err != nil {
						return err
					}
					minerID := abi.ActorID(actorID)
					privsectors, err := pubSectorToPriv(minerID, mbi.Sectors, cctx.String("path"))
					if err != nil {
						return err
					}

					buf := new(bytes.Buffer)
					if err := addr.MarshalCBOR(buf); err != nil {
						err = xerrors.Errorf("failed to marshal miner address: %w", err)
						return err
					}
					rand, err := store.DrawRandomness(rbase.Data, crypto.DomainSeparationTag_WinningPoStChallengeSeed, round, buf.Bytes())
					if err != nil {
						err = xerrors.Errorf("failed to get randomness for winning post: %w", err)
						return err
					}
					randomness := abi.PoStRandomness(rand)
					randomness[31] &= 0x3f
					proofs, err := ffi.GenerateWinningPoSt(minerID, privsectors, randomness)
					if err != nil {
						return err
					}
					if cctx.String("out") != "" {
						out := WinningOut{
							MinerID:    minerID,
							Sectors:    mbi.Sectors,
							Randomness: randomness,
							Proofs:     proofs,
						}

						b, err := json.Marshal(out)
						if err != nil {
							return err
						}
						f, err := os.OpenFile(cctx.String("out"), os.O_CREATE|os.O_RDWR, 0644)
						if err != nil {
							return err
						}
						defer f.Close()
						_, err = f.Write(b)
						if err != nil {
							return err
						}
					}
					ok, err := verifyWinningPoSt(minerID, mbi.Sectors, proofs, randomness)
					fmt.Printf("verify winningPoSt ok %v error %v\n", ok, err)
				}
			}
		}

		return nil
	},
}

var verifyCmd = &cli.Command{
	Name: "verify",
	Flags: []cli.Flag{
		&cli.StringFlag{
			Name:  "in",
			Value: "./winningProofs",
		},
	},
	Action: func(cctx *cli.Context) error {

		if cctx.String("in") != "" {
			f, err := os.Open(cctx.String("in"))
			if err != nil {
				return err
			}
			defer f.Close()
			b, err := ioutil.ReadAll(f)
			if err != nil {
				return err
			}

			var ret WinningOut
			err = json.Unmarshal(b, &ret)
			if err != nil {
				return err
			}

			ok, err := verifyWinningPoSt(ret.MinerID, ret.Sectors, ret.Proofs, ret.Randomness)
			fmt.Printf("verify winningPoSt ok %v error %v\n", ok, err)
		}

		return nil
	},
}

type WinningOut struct {
	MinerID    abi.ActorID
	Sectors    []proof5.SectorInfo
	Proofs     []proof5.PoStProof
	Randomness abi.PoStRandomness
}

func pubSectorToPriv(mid abi.ActorID, sectorInfo []proof5.SectorInfo, path string) (ffi.SortedPrivateSectorInfo, error) {

	var out []ffi.PrivateSectorInfo
	for _, s := range sectorInfo {

		postProofType, err := abi.RegisteredSealProof.RegisteredWinningPoStProof(s.SealProof)
		if err != nil {
			return ffi.SortedPrivateSectorInfo{}, err
		}

		out = append(out, ffi.PrivateSectorInfo{
			CacheDirPath:     fmt.Sprintf("%s/cache/s-t0%d-%d", path, mid, s.SectorNumber),
			PoStProofType:    postProofType,
			SealedSectorPath: fmt.Sprintf("%s/sealed/s-t0%d-%d", path, mid, s.SectorNumber),
			SectorInfo:       s,
		})
	}

	return ffi.NewSortedPrivateSectorInfo(out...), nil
}

func verifyWinningPoSt(minerID abi.ActorID, sectorInfo []proof5.SectorInfo, proofs []proof5.PoStProof, randomness abi.PoStRandomness) (bool, error) {

	var provingSet []proof5.SectorInfo
	for _, s := range sectorInfo {
		p := proof5.SectorInfo{
			SealProof:    s.SealProof,
			SectorNumber: s.SectorNumber,
			SealedCID:    s.SealedCID,
		}
		provingSet = append(provingSet, p)
	}
	winningPostProofType, err := abi.RegisteredSealProof.RegisteredWinningPoStProof(sectorInfo[0].SealProof)
	if err != nil {
		return false, err
	}

	// figure out which sectors have been challenged
	indicesInProvingSet, err := ffi.GenerateWinningPoStSectorChallenge(winningPostProofType, minerID, randomness[:], uint64(len(provingSet)))
	if err != nil {
		return false, err
	}

	var challengedSectors []proof5.SectorInfo
	for idx := range indicesInProvingSet {
		challengedSectors = append(challengedSectors, provingSet[indicesInProvingSet[idx]])
	}
	return ffi.VerifyWinningPoSt(proof5.WinningPoStVerifyInfo{
		Randomness:        randomness[:],
		Proofs:            proofs,
		ChallengedSectors: challengedSectors,
		Prover:            minerID,
	})
}
