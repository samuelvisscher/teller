package scanner

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/boltdb/bolt"
	"github.com/btcsuite/btcd/btcjson"
	"github.com/btcsuite/btcd/chaincfg/chainhash"
	"github.com/stretchr/testify/require"

	"github.com/skycoin/teller/src/util/dbutil"
	"github.com/skycoin/teller/src/util/testutil"
)

var dummyBlocksBktName = []byte("blocks")

type blockHashHeight struct {
	Hash   string
	Height int64
}

type dummyBtcrpcclient struct {
	db        *bolt.DB
	bestBlock blockHashHeight
	lastBlock blockHashHeight
}

func newDummyBtcrpcclient(t *testing.T) *dummyBtcrpcclient {
	db, err := bolt.Open("./test.db", 0600, nil)
	require.NoError(t, err)

	return &dummyBtcrpcclient{db: db}
}

func (dbc *dummyBtcrpcclient) Shutdown() {
}

func (dbc *dummyBtcrpcclient) GetBestBlock() (*chainhash.Hash, int32, error) {
	hash, err := chainhash.NewHashFromStr(dbc.bestBlock.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, int32(dbc.bestBlock.Height), nil
}

func (dbc *dummyBtcrpcclient) GetLastScanBlock() (*chainhash.Hash, int32, error) {
	hash, err := chainhash.NewHashFromStr(dbc.lastBlock.Hash)
	if err != nil {
		return nil, 0, err
	}

	return hash, int32(dbc.lastBlock.Height), nil
}

func (dbc *dummyBtcrpcclient) GetBlockVerboseTx(hash *chainhash.Hash) (*btcjson.GetBlockVerboseResult, error) {
	var block *btcjson.GetBlockVerboseResult
	if err := dbc.db.View(func(tx *bolt.Tx) error {
		var b btcjson.GetBlockVerboseResult
		v := tx.Bucket(dummyBlocksBktName).Get([]byte(hash.String()))
		if v == nil {
			return nil
		}

		if err := json.Unmarshal(v, &b); err != nil {
			return err
		}

		block = &b
		return nil
	}); err != nil {
		return nil, err
	}

	return block, nil
}

func TestScannerRun(t *testing.T) {
	db, shutdown := testutil.PrepareDB(t)
	defer shutdown()

	log, _ := testutil.NewLogger(t)

	rpcclient := newDummyBtcrpcclient(t)
	rpcclient.lastBlock = blockHashHeight{
		Hash:   "00000000000001749cf1a15c5af397a04a18d09e9bc902b6ce70f64bc19acc98",
		Height: 235203,
	}

	rpcclient.bestBlock = blockHashHeight{
		Hash:   "000000000000018d8ece83a004c5a919210d67798d13aa901c4d07f8bf87b719",
		Height: 235205,
	}

	scr, err := NewBTCScanner(log, db, rpcclient, Config{
		ScanPeriod: time.Second * 1,
	})

	require.NoError(t, err)

	scr.AddScanAddress("1ATjE4kwZ5R1ww9SEi4eseYTCenVgaxPWu")
	scr.AddScanAddress("1EYQ7Fnct6qu1f3WpTSib1UhDhxkrww1WH")
	scr.AddScanAddress("1LEkderht5M5yWj82M87bEd4XDBsczLkp9")

	time.AfterFunc(time.Second, func() {
		var dvs []DepositNote
		for dv := range scr.GetDeposit() {
			dvs = append(dvs, dv)
			time.Sleep(100 * time.Millisecond)
			dv.ErrC <- nil
		}
		require.Equal(t, 127, len(dvs))

		// check all deposit value's
		db.View(func(tx *bolt.Tx) error {
			for _, dv := range dvs {
				var d Deposit
				require.Nil(t, dbutil.GetBucketObject(tx, depositBkt, dv.TxN(), &d))
				require.True(t, d.Processed)
			}

			return nil
		})
	})

	time.AfterFunc(15*time.Second, func() {
		scr.Shutdown()
	})

	err = scr.Run()
	require.NoError(t, err)
}