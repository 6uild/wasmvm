package alex

import (
	_ "embed"
	wasmvm "github.com/CosmWasm/wasmvm"
	"github.com/CosmWasm/wasmvm/api"
	"github.com/CosmWasm/wasmvm/types"
	"github.com/stretchr/testify/require"
	"math"
	"path/filepath"
	"testing"
	"time"
)

//go:embed contracts/trusted_token.wasm
var trustedTokensWasm []byte

func TestTrustedTokenExecute(t *testing.T) {
	homeDir := t.TempDir()
	const contractMemoryLimit = 32
	const defaultMemoryCacheSize = 100 // in MiB
	features := "staking,iterator,tgrade"
	wasmer, err := wasmvm.NewVM(filepath.Join(homeDir, "wasm"), features, contractMemoryLimit, true, defaultMemoryCacheSize)
	require.NoError(t, err)
	checksum, err := wasmer.Create(trustedTokensWasm)
	require.NoError(t, err)
	myContractAddr := "tgrade1ae0e7wmt56mqwhre4vs8t0nlv8j2arj5r2a6t9"
	env := types.Env{
		Block: types.BlockInfo{
			Height:  uint64(1),
			Time:    uint64(time.Now().UnixNano()),
			ChainID: "testing",
		},
		Contract: types.ContractInfo{
			Address: myContractAddr,
		},
	}
	info := types.MessageInfo{
		Sender: "my-sender",
		Funds:  types.Coins{},
	}
	initBz := []byte(`
{
            "name": "Test Token",
            "symbol": "TST",
            "decimals": 6,
            "initial_balances": [
              {
                "address": "tgrade1ae0e7wmt56mqwhre4vs8t0nlv8j2arj5r2a6t9",
                "amount": "100000000000000"
              }
            ],
            "whitelist_group": "tgrade1kydlyzdwqkyxw360hu95320p62tu98mz04sk2p33as0j9pa20d6qgvpypv"
          }
`)

	infinteGas := uint64(math.MaxUint64)
	gasMeter := api.NewMockGasMeter(infinteGas)
	kvStore := wasmvm.KVStore(api.NewLookup(gasMeter))
	goApi := api.GoAPI{
		HumanAddress: func(bytes []byte) (string, uint64, error) {
			return string(bytes), 0, nil
		},
		CanonicalAddress: func(s string) ([]byte, uint64, error) {
			return []byte(s), 0, nil
		},
	}
	querier := MyQuerier(myContractAddr, types.Coins{}, func(r types.QueryRequest, limit uint64) ([]byte, error) {
		return []byte(`{"members":[]}`), nil
	})
	deserCost := types.UFraction{Numerator: 140_000_000, Denominator: 1}
	_, _, err = wasmer.Instantiate(checksum, env, info, initBz, kvStore, goApi, querier, gasMeter, infinteGas, deserCost)
	require.NoError(t, err)

	execBz := []byte(`
{
            "increase_allowance": {
              "spender": "tgrade1kydlyzdwqkyxw360hu95320p62tu98mz04sk2p33as0j9pa20d6qgvpypv",
              "amount": "340282366920938463463374607431768211454"
            }
}
`)
	_, _, err = wasmer.Execute(checksum, env, info, execBz, kvStore, goApi, querier, gasMeter, infinteGas, deserCost)
	require.NoError(t, err)
}

var _ api.Querier = myQuerier{}

type myQuerier struct {
	other       api.Querier
	wasmHandler func(request types.QueryRequest, limit uint64) ([]byte, error)
}

func MyQuerier(contractAddr string, coins types.Coins, wasmHandler func(request types.QueryRequest, limit uint64) ([]byte, error)) api.Querier {
	return myQuerier{other: api.DefaultQuerier(contractAddr, coins), wasmHandler: wasmHandler}
}

func (m myQuerier) Query(request types.QueryRequest, gasLimit uint64) ([]byte, error) {
	if request.Wasm == nil {
		return m.other.Query(request, gasLimit)
	}
	return m.wasmHandler(request, gasLimit)
}

func (m myQuerier) GasConsumed() uint64 {
	return m.other.GasConsumed()
}
