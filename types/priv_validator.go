package types

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/cometbft/cometbft/crypto"
	blscrypto "github.com/cometbft/cometbft/crypto/bls"
	"github.com/cometbft/cometbft/crypto/ed25519"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"
)

// Keep the original PrivValidator interface
type PrivValidator interface {
	GetPubKey() (crypto.PubKey, error)

	SignVote(chainID string, vote *cmtproto.Vote) error
	SignProposal(chainID string, proposal *cmtproto.Proposal) error

	// New BLS-specific methods (optional)
	GetBlsPrivKey() (*blscrypto.PrivateKey, error)
}

type PrivValidatorsByAddress []PrivValidator

func (pvs PrivValidatorsByAddress) Len() int {
	return len(pvs)
}

func (pvs PrivValidatorsByAddress) Less(i, j int) bool {
	pvi, err := pvs[i].GetPubKey()
	if err != nil {
		panic(err)
	}
	pvj, err := pvs[j].GetPubKey()
	if err != nil {
		panic(err)
	}

	return bytes.Compare(pvi.Address(), pvj.Address()) == -1
}

func (pvs PrivValidatorsByAddress) Swap(i, j int) {
	pvs[i], pvs[j] = pvs[j], pvs[i]
}

// MockPV implementation
type MockPV struct {
	PrivKey              crypto.PrivKey
	BlsPrivKey           *blscrypto.PrivateKey
	breakProposalSigning bool
	breakVoteSigning     bool
}

func NewMockPV() MockPV {
	privKey := ed25519.GenPrivKey()
	blsPrivKey := blscrypto.GeneratePrivateKey(blscrypto.KeyGenRandom)
	return MockPV{
		PrivKey:              privKey,
		BlsPrivKey:           blsPrivKey,
		breakProposalSigning: false,
		breakVoteSigning:     false,
	}
}

// Restore original methods
func (pv MockPV) GetPubKey() (crypto.PubKey, error) {
	return pv.PrivKey.PubKey(), nil
}

func (pv MockPV) SignVote(chainID string, vote *cmtproto.Vote) error {
	useChainID := chainID
	if pv.breakVoteSigning {
		useChainID = "incorrect-chain-id"
	}

	signBytes := VoteSignBytes(useChainID, vote)
	sig, err := pv.PrivKey.Sign(signBytes)
	if err != nil {
		return err
	}
	vote.Signature = sig

	var extSig []byte
	// We only sign vote extensions for non-nil precommits
	if vote.Type == cmtproto.PrecommitType && !ProtoBlockIDIsNil(&vote.BlockID) {
		extSignBytes := VoteExtensionSignBytes(useChainID, vote)
		extSig, err = pv.PrivKey.Sign(extSignBytes)
		if err != nil {
			return err
		}
	} else if len(vote.Extension) > 0 {
		return errors.New("unexpected vote extension - vote extensions are only allowed in non-nil precommits")
	}
	vote.ExtensionSignature = extSig
	return nil
}

func (pv MockPV) SignProposal(chainID string, proposal *cmtproto.Proposal) error {
	useChainID := chainID
	if pv.breakProposalSigning {
		useChainID = "incorrect-chain-id"
	}

	signBytes := ProposalSignBytes(useChainID, proposal)
	sig, err := pv.PrivKey.Sign(signBytes)
	if err != nil {
		return err
	}
	proposal.Signature = sig
	return nil
}

// New BLS-specific method
func (pv MockPV) GetBlsPrivKey() (*blscrypto.PrivateKey, error) {
	if pv.BlsPrivKey == nil {
		// Generate a new BLS key if not exists
		pv.BlsPrivKey = blscrypto.GeneratePrivateKey(blscrypto.KeyGenRandom)
	}
	return pv.BlsPrivKey, nil
}

func (pv MockPV) ExtractIntoValidator(votingPower int64) *Validator {
	pubKey, _ := pv.GetPubKey()
	blsPrivKey, _ := pv.GetBlsPrivKey()

	return NewValidator(
		pubKey,
		votingPower,
		blsPrivKey.PublicKey(),
	)
}

func (pv MockPV) String() string {
	mpv, _ := pv.GetPubKey() // mockPV will never return an error, ignored here
	return fmt.Sprintf("MockPV{%v}", mpv.Address())
}

func (pv MockPV) DisableChecks() {
	// Currently this does nothing,
	// as MockPV has no safety checks at all.
}

// NewMockPVWithParams allows one to create a MockPV instance, but with finer
// grained control over the operation of the mock validator.
func NewMockPVWithParams(privKey crypto.PrivKey, breakProposalSigning, breakVoteSigning bool) MockPV {
	return MockPV{
		PrivKey:              privKey,
		BlsPrivKey:           blscrypto.GeneratePrivateKey(blscrypto.KeyGenRandom),
		breakProposalSigning: breakProposalSigning,
		breakVoteSigning:     breakVoteSigning,
	}
}

// Error MockPV
type ErroringMockPV struct {
	MockPV
}

var ErroringMockPVErr = errors.New("erroringMockPV always returns an error")

func (pv *ErroringMockPV) SignVote(string, *cmtproto.Vote) error {
	return ErroringMockPVErr
}

func (pv *ErroringMockPV) SignProposal(string, *cmtproto.Proposal) error {
	return ErroringMockPVErr
}

func (pv *ErroringMockPV) GetBlsPrivKey() (*blscrypto.PrivateKey, error) {
	return nil, ErroringMockPVErr
}

func NewErroringMockPV() *ErroringMockPV {
	return &ErroringMockPV{
		MockPV{
			PrivKey:              ed25519.GenPrivKey(),
			BlsPrivKey:           nil,
			breakProposalSigning: false,
			breakVoteSigning:     false,
		},
	}
}
