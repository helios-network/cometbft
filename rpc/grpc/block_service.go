package coregrpc

import (
	context "context"
	"errors"
	"fmt"
	"sync"

	abci "github.com/tendermint/tendermint/abci/types"
	"github.com/tendermint/tendermint/libs/log"
	v1 "github.com/tendermint/tendermint/proto/tendermint/services/block/v1"
	"github.com/tendermint/tendermint/state"
	"github.com/tendermint/tendermint/types"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type blockServiceServer struct {
	stateStore state.Store
	blockStore state.BlockStore
	eventBus   *types.EventBus
	log        log.Logger

	mtx     sync.Mutex
	nextID  int
	subsIDs map[int]interface{}
}

type BlockServiceServerConfig struct {
	StateStore state.Store
	BlockStore state.BlockStore
	EventBus   *types.EventBus
	Log        log.Logger
}

var _ v1.BlockServiceServer = (*blockServiceServer)(nil)

// NewBlockServiceServer creates a new gRPC BlockServiceServer instance.
func NewBlockServiceServer(cfg *BlockServiceServerConfig) (v1.BlockServiceServer, error) {
	if cfg.StateStore == nil {
		return nil, errors.New("block service requires access to the state store")
	}
	if cfg.BlockStore == nil {
		return nil, errors.New("block service requires access to the block store")
	}
	if cfg.EventBus == nil {
		return nil, errors.New("block service requires access to the event bus")
	}
	if cfg.Log == nil {
		return nil, errors.New("block service requires a logger")
	}
	return &blockServiceServer{
		stateStore: cfg.StateStore,
		blockStore: cfg.BlockStore,
		eventBus:   cfg.EventBus,
		log:        cfg.Log,
		subsIDs:    make(map[int]interface{}),
	}, nil
}

// GetBlock implements v1.BlockServiceServer
func (s *blockServiceServer) GetBlock(ctx context.Context, req *v1.GetBlockRequest) (*v1.GetBlockResponse, error) {
	height := req.Height
	if height <= 1 {
		height = s.blockStore.Height()
	}
	block := s.blockStore.LoadBlock(height)
	if block == nil {
		return nil, status.Errorf(codes.NotFound, "no such block in store for height %d", height)
	}
	bp, err := block.ToProto()
	if err != nil {
		traceID := newTraceID()
		s.log.Error("Failed to serialize block to Protobuf representation", "err", err, "traceID", traceID)
		return nil, status.Errorf(codes.Internal, "failed to serialize block to Protobuf representation (%s)", traceID)
	}
	blockMeta := s.blockStore.LoadBlockMeta(height)
	if blockMeta == nil {
		return nil, status.Errorf(codes.NotFound, "no such block metadata in store for height %d", height)
	}
	blockID := blockMeta.BlockID
	bip := blockID.ToProto()
	return &v1.GetBlockResponse{
		Height:  height,
		BlockId: &bip,
		Block:   bp,
	}, nil
}

// GetBlockResults implements v1.BlockServiceServer
func (s *blockServiceServer) GetBlockResults(ctx context.Context, req *v1.GetBlockResultsRequest) (*v1.GetBlockResultsResponse, error) {
	height := req.Height
	if height <= 1 {
		height = s.blockStore.Height()
	}
	results, err := s.stateStore.LoadABCIResponses(height)
	if err != nil {
		// Assumes that the only possible reason for failure is that we cannot
		// find the results for that height
		return nil, status.Errorf(codes.NotFound, "no ABCI responses stored for height %d", height)
	}

	// We have to convert these to pointers to deal with legacy non-pointer
	// representations of these structs.
	beginBlockEvents := make([]*abci.Event, len(results.BeginBlock.Events))
	for _, ev := range results.BeginBlock.Events {
		beginBlockEvents = append(beginBlockEvents, &ev)
	}
	validatorUpdates := make([]*abci.ValidatorUpdate, len(results.EndBlock.ValidatorUpdates))
	for _, vu := range results.EndBlock.ValidatorUpdates {
		validatorUpdates = append(validatorUpdates, &vu)
	}
	endBlockEvents := make([]*abci.Event, len(results.EndBlock.Events))
	for _, ev := range results.EndBlock.Events {
		endBlockEvents = append(endBlockEvents, &ev)
	}
	return &v1.GetBlockResultsResponse{
		Height:                height,
		BeginBlockEvents:      beginBlockEvents,
		TxResults:             results.DeliverTxs,
		ValidatorUpdates:      validatorUpdates,
		ConsensusParamUpdates: results.EndBlock.ConsensusParamUpdates,
		EndBlockEvents:        endBlockEvents,
	}, nil
}

func (s *blockServiceServer) nextSubsID() int {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	nextID := s.nextID
	for {
		if _, exists := s.subsIDs[nextID]; exists {
			nextID++
		} else {
			break
		}
	}
	s.nextID = nextID + 1
	s.subsIDs[nextID] = nil
	return nextID
}

func (s *blockServiceServer) removeSubsID(id int) {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	delete(s.subsIDs, id)
}

// GetLatestHeight implements v1.BlockServiceServer
func (s *blockServiceServer) GetLatestHeight(req *v1.GetLatestHeightRequest, stream v1.BlockService_GetLatestHeightServer) error {
	subsID := s.nextSubsID()
	defer s.removeSubsID(subsID)

	subsIDStr := fmt.Sprintf("BlockServiceServer%d", s.nextSubsID())
	subs, err := s.eventBus.Subscribe(context.Background(), subsIDStr, types.EventQueryNewBlock)
	if err != nil {
		traceID := newTraceID()
		s.log.Error("Failed to initiate NewBlock event subscription in BlockServiceServer", "err", err, "traceID", traceID, "subsID", subsIDStr)
		return status.Errorf(codes.Internal, "internal subscription error (%s)", traceID)
	}
	defer func() {
		err := s.eventBus.Unsubscribe(context.Background(), subsIDStr, types.EventQueryNewBlock)
		if err != nil {
			s.log.Debug("Failed to unsubscribe from NewBlock event query in BlockServiceServer", "err", err, "subsID", subsIDStr)
		}
	}()

	for {
		select {
		case <-subs.Cancelled():
			if err := subs.Err(); err != nil {
				traceID := newTraceID()
				s.log.Error("BlockServiceServer subscription cancelled", "err", err, "traceID", traceID, "subsID", subsIDStr)
				return status.Errorf(codes.Canceled, "subscription cancelled by server (%s)", traceID)
			}

		case msg := <-subs.Out():
			eventData, ok := msg.Data().(types.EventDataNewBlock)
			if !ok {
				traceID := newTraceID()
				s.log.Error("Unexpected message type received from internal subscription", "data", msg.Data(), "traceID", traceID, "subsID", subsIDStr)
				return status.Errorf(codes.Internal, "internal subscription error (%s)", traceID)
			}
			res := &v1.GetLatestHeightResponse{
				Height: eventData.Block.Height,
			}
			if err := stream.Send(res); err != nil {
				traceID := newTraceID()
				// Set to debug level because this will most likely happen when
				// a client disconnects.
				s.log.Debug("Failed to send latest height to BlockServiceServer client", "err", err, "traceID", traceID, "subsID", subsIDStr)
				return status.Errorf(codes.Internal, "failed to write response to stream (%s)", traceID)
			}
		}
	}
}
