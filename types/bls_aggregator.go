package types

import (
	"errors"
	"fmt"

	"github.com/cometbft/cometbft/crypto/bls"
	blst "github.com/supranational/blst/bindings/go"
)

type BlsSignatureAggregator struct {
	validatorSet *ValidatorSet
}

func NewBlsSignatureAggregator(valSet *ValidatorSet) *BlsSignatureAggregator {
	return &BlsSignatureAggregator{
		validatorSet: valSet,
	}
}

func (a *BlsSignatureAggregator) AggregateVoteSignatures(votes []*Vote) (*blst.P2Affine, error) {
	if len(votes) == 0 {
		return nil, errors.New("no votes to aggregate")
	}

	signatures := make([]*blst.P2Affine, 0, len(votes))

	for _, vote := range votes {
		// Find corresponding validator
		_, validator := a.validatorSet.GetByAddress(vote.ValidatorAddress)
		if validator == nil || validator.BlsPubKey == nil {
			continue // Skip validators without BLS public key
		}

		if len(vote.Signature) == 0 {
			continue // Skip votes with empty signatures
		}

		// Convert signature to BLS signature
		signature, err := bls.SignatureFromBytes(vote.Signature)
		if err != nil {
			panic("invalid bls signature")
			continue // Skip invalid signatures
		}

		signatures = append(signatures, signature)
	}

	// Aggregate signatures
	aggregatedSignature, err := bls.AggregateSignatures(signatures)
	if err != nil {
		return nil, fmt.Errorf("failed to aggregate BLS signatures: %w", err)
	}

	return aggregatedSignature, nil
}

// Add a method to verify the aggregated signature
func (a *BlsSignatureAggregator) VerifyAggregatedSignature(
	message []byte,
	aggregatedSignature *blst.P2Affine,
	participatingValidators []*Validator,
) bool {
	// Collect BLS public keys for participating validators
	pubKeys := make([]*bls.PublicKey, 0, len(participatingValidators))
	for _, val := range participatingValidators {
		if val.BlsPubKey != nil {
			pubKeys = append(pubKeys, val.BlsPubKey)
		}
	}

	// Verify the aggregate signature
	return bls.VerifyAggregateSignature(message, aggregatedSignature, pubKeys)
}

func (a *BlsSignatureAggregator) VerifyAggregatedSignatureMultiMessage(
	messages [][]byte,
	aggregatedSignature *blst.P2Affine,
	participatingValidators []*Validator,
) bool {
	// Collect BLS public keys for participating validators
	pubKeys := make([]*bls.PublicKey, 0, len(participatingValidators))
	for _, val := range participatingValidators {
		if val.BlsPubKey != nil {
			pubKeys = append(pubKeys, val.BlsPubKey)
		}
	}

	// Use AggregateVerify which can handle multiple messages
	return bls.AggregateVerify(
		pubKeys,             // Public keys
		messages,            // Individual messages
		aggregatedSignature, // Aggregated signature
	)
}
