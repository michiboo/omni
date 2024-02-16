package provider

import (
	"context"

	atypes "github.com/omni-network/omni/halo2/attest/types"
	"github.com/omni-network/omni/lib/errors"
	"github.com/omni-network/omni/lib/expbackoff"
	"github.com/omni-network/omni/lib/xchain"

	rpcclient "github.com/cometbft/cometbft/rpc/client"

	grpc1 "github.com/cosmos/gogoproto/grpc"
	"github.com/cosmos/gogoproto/proto"
	"google.golang.org/grpc"
)

func NewABCIProvider2(abci rpcclient.ABCIClient) Provider {
	backoffFunc := func(ctx context.Context) (func(), func()) {
		return expbackoff.NewWithReset(ctx, expbackoff.WithFastConfig())
	}

	cl := atypes.NewQueryClient(rpcAdaptor{abci: abci})

	return Provider{
		fetch:       newABCIFetchFunc(cl),
		backoffFunc: backoffFunc,
	}
}

func newABCIFetchFunc(cl atypes.QueryClient) func(ctx context.Context, chainID uint64, fromHeight uint64,
) ([]xchain.AggAttestation, error) {
	return func(ctx context.Context, chainID uint64, fromHeight uint64) ([]xchain.AggAttestation, error) {
		resp, err := cl.ApprovedFrom(ctx, &atypes.ApprovedFromRequest{
			ChainId:    chainID,
			FromHeight: fromHeight,
		})
		if err != nil {
			return nil, errors.Wrap(err, "abci query approved-from")
		}

		aggs, err := atypes.AggregatesFromProto(resp.Aggregates)
		if err != nil {
			return nil, errors.Wrap(err, "unmarshal approved-from aggregates")
		}

		return aggs, nil
	}
}

// rpcAdaptor adapts the cometBFT query client to the gRPC client interface.
// Note it only supports the Invoke method, not the NewStream method.
type rpcAdaptor struct {
	grpc1.ClientConn
	abci rpcclient.ABCIClient
}

func (a rpcAdaptor) Invoke(ctx context.Context, method string, req, resp any, _ ...grpc.CallOption) error {
	reqpb, ok := req.(proto.Message)
	if !ok {
		return errors.New("args not proto.Message")
	}
	resppb, ok := resp.(proto.Message)
	if !ok {
		return errors.New("args not proto.Message")
	}

	bz, err := proto.Marshal(reqpb)
	if err != nil {
		return errors.Wrap(err, "marshal approved-from request")
	}

	r, err := a.abci.ABCIQuery(ctx, method, bz)
	if err != nil {
		return errors.Wrap(err, "abci query")
	} else if !r.Response.IsOK() {
		return errors.New("abci query failed", "code", r.Response.Code)
	}

	err = proto.Unmarshal(r.Response.Value, resppb)
	if err != nil {
		return errors.Wrap(err, "unmarshal approved-from response")
	}

	return nil
}
