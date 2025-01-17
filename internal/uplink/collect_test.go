package uplink

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/brocaar/chirpstack-network-server/api/gw"
	"github.com/brocaar/chirpstack-network-server/internal/band"
	"github.com/brocaar/chirpstack-network-server/internal/helpers"
	"github.com/brocaar/chirpstack-network-server/internal/models"
	"github.com/brocaar/chirpstack-network-server/internal/storage"
	"github.com/brocaar/chirpstack-network-server/internal/test"
	"github.com/brocaar/lorawan"
)

type CollectTestSuite struct {
	suite.Suite
}

func (ts *CollectTestSuite) SetupSuite() {
	assert := require.New(ts.T())
	conf := test.GetConfig()
	conf.NetworkServer.DeduplicationDelay = time.Millisecond * 500

	assert.NoError(storage.Setup(conf))
	assert.NoError(Setup(conf))
}

func (ts *CollectTestSuite) TestDeduplication() {
	testTable := []struct {
		Name       string
		PHYPayload lorawan.PHYPayload
		Gateways   []lorawan.EUI64
		Count      int
	}{
		{
			"single item expected",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{1, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{1, 1, 1, 1, 1, 1, 1, 1},
			},
			1,
		}, {
			"two items expected",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{2, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{2, 1, 1, 1, 1, 1, 1, 1},
				{2, 2, 2, 2, 2, 2, 2, 2},
			},
			2,
		}, {
			"two items expected (three collected)",
			lorawan.PHYPayload{
				MHDR: lorawan.MHDR{
					MType: lorawan.UnconfirmedDataUp,
					Major: lorawan.LoRaWANR1,
				},
				MIC:        [4]byte{3, 2, 3, 4},
				MACPayload: &lorawan.MACPayload{},
			},
			[]lorawan.EUI64{
				{3, 1, 1, 1, 1, 1, 1, 1},
				{3, 2, 2, 2, 2, 2, 2, 2},
				{3, 2, 2, 2, 2, 2, 2, 2},
			},
			2,
		},
	}

	for _, tst := range testTable {
		ts.T().Run(tst.Name, func(t *testing.T) {
			assert := require.New(t)
			test.MustFlushRedis(storage.RedisPool())

			var received int
			var called int

			cb := func(packet models.RXPacket) error {
				called = called + 1
				received = len(packet.RXInfoSet)
				return nil
			}

			var wg sync.WaitGroup
			for i := range tst.Gateways {
				g := tst.Gateways[i]
				phyB, err := tst.PHYPayload.MarshalBinary()
				assert.NoError(err)

				wg.Add(1)
				packet := gw.UplinkFrame{
					RxInfo: &gw.UplinkRXInfo{
						GatewayId: g[:],
					},
					TxInfo:     &gw.UplinkTXInfo{},
					PhyPayload: phyB,
				}
				assert.NoError(helpers.SetUplinkTXInfoDataRate(packet.TxInfo, 0, band.Band()))

				go func(packet gw.UplinkFrame) {
					assert.NoError(collectAndCallOnce(storage.RedisPool(), packet, cb))
					wg.Done()
				}(packet)
			}
			wg.Wait()

			assert.Equal(1, called)
			assert.Equal(tst.Count, received)
		})
	}
}

func TestCollect(t *testing.T) {
	suite.Run(t, new(CollectTestSuite))
}
