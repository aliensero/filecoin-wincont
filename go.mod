module wincount

go 1.16

require (
	github.com/filecoin-project/filecoin-ffi v0.30.4-0.20200910194244-f640612a1a1f
	github.com/filecoin-project/go-address v0.0.5
	github.com/filecoin-project/go-state-types v0.1.1-0.20210506134452-99b279731c48
	github.com/filecoin-project/lotus v1.11.0
	github.com/filecoin-project/specs-actors/v5 v5.0.1
	github.com/urfave/cli/v2 v2.2.0
	golang.org/x/xerrors v0.0.0-20200804184101-5ec99f83aff1
)

replace (
	github.com/filecoin-project/filecoin-ffi => ../lotus/extern/filecoin-ffi
	github.com/filecoin-project/lotus => ../lotus
)
