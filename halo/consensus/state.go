package consensus

import (
	"encoding/json"
	"sync"

	"github.com/omni-network/omni/lib/errors"
	"github.com/omni-network/omni/lib/xchain"

	abci "github.com/cometbft/cometbft/api/cometbft/abci/v1"

	"github.com/ethereum/go-ethereum/crypto"
)

type state struct {
	mu                 sync.Mutex
	ExecutionStateRoot [32]byte
	valSetID           uint64
	validators         []abci.ValidatorUpdate
	pendingAggs        []xchain.AggAttestation
	approvedAggs       []xchain.AggAttestation
}

// InitChainState sets consensus validators.
// It returns an error if the validators are already set.
// This implies validators can only be set once and never change.
func (s *state) InitChainState(req *abci.InitChainRequest) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.valSetID > 0 {
		return errors.New("chain already initialized")
	}

	if req.InitialHeight > 0 {
		return errors.New("initial height must be 0")
	}

	if len(req.AppStateBytes) > 0 {
		return errors.New("app state bytes must be empty")
	}

	s.valSetID++ // Update from 0 to 1.
	s.validators = req.Validators

	return nil
}

// AppHash returns the application hash.
func (s *state) AppHash() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	buf, err := json.Marshal(appHashJSON{
		ValSetID:     s.valSetID,
		Validators:   s.validators,
		PendingAggs:  s.pendingAggs,
		ApprovedAggs: s.approvedAggs,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal app hash json")
	}

	return crypto.Keccak256(buf), nil
}

// AddAttestations adds the provided aggregates to the state, moving pending to approved if applicable.
func (s *state) AddAttestations(aggregates []xchain.AggAttestation) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Create a lookup
	aggsByHeader := make(map[xchain.BlockHeader]xchain.AggAttestation)
	for _, agg := range aggregates {
		aggsByHeader[agg.BlockHeader] = agg
	}

	// Add to pending, moving approved.
	pendingCopy := s.pendingAggs
	s.pendingAggs = nil
	for _, agg := range pendingCopy {
		toAdd, ok := aggsByHeader[agg.BlockHeader]
		if !ok {
			s.pendingAggs = append(s.pendingAggs, agg)
			continue
		}

		agg.Signatures = append(agg.Signatures, toAdd.Signatures...)
		if isApproved(agg, s.validators) {
			s.approvedAggs = append(s.approvedAggs, agg)
		} else {
			s.pendingAggs = append(s.pendingAggs, agg)
		}
	}

	// TODO(corver): Update approved aggregates, also trim approved aggregates after some blocks.
}

// appHashJSON is the JSON representation of the state used to calculate app hash.
type appHashJSON struct {
	ExecutionStateRoot [32]byte                `json:"execution_state_root"`
	ValSetID           uint64                  `json:"validator_set_id"`
	Validators         []abci.ValidatorUpdate  `json:"validators"`
	PendingAggs        []xchain.AggAttestation `json:"pending_aggregate_attestations"`
	ApprovedAggs       []xchain.AggAttestation `json:"approved_aggregate_attestations"`
}