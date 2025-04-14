package types

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/cometbft/cometbft/crypto"
	ce "github.com/cometbft/cometbft/crypto/encoding"
	cmtrand "github.com/cometbft/cometbft/libs/rand"
	cmtproto "github.com/cometbft/cometbft/proto/tendermint/types"

	blscrypto "github.com/cometbft/cometbft/crypto/bls"
)

// Validator now includes optional BLS public key
type Validator struct {
	Address     Address       `json:"address"`
	PubKey      crypto.PubKey `json:"pub_key"`
	VotingPower int64         `json:"voting_power"`

	ProposerPriority int64                `json:"proposer_priority"`
	BlsPubKey        *blscrypto.PublicKey `json:"bls_pub_key,omitempty"`
}

// NewValidator returns a new validator with the given pubkey and voting power.
// Optionally allows adding a BLS public key
func NewValidator(pubKey crypto.PubKey, votingPower int64, blsPubKey ...*blscrypto.PublicKey) *Validator {
	val := &Validator{
		Address:          pubKey.Address(),
		PubKey:           pubKey,
		VotingPower:      votingPower,
		ProposerPriority: 0,
	}

	// Set BLS public key if provided
	if len(blsPubKey) > 0 && blsPubKey[0] != nil {
		val.BlsPubKey = blsPubKey[0]
	}

	return val
}

// ValidateBasic performs basic validation.
func (v *Validator) ValidateBasic() error {
	if v == nil {
		return errors.New("nil validator")
	}
	if v.PubKey == nil {
		return errors.New("validator does not have a public key")
	}

	if v.VotingPower < 0 {
		return errors.New("validator has negative voting power")
	}

	addr := v.PubKey.Address()
	if !bytes.Equal(v.Address, addr) {
		return fmt.Errorf("validator address is incorrectly derived from pubkey. Exp: %v, got %v", addr, v.Address)
	}

	// Optional BLS public key validation
	if v.BlsPubKey != nil && !blscrypto.IsValidPublicKey(v.BlsPubKey) {
		return errors.New("invalid BLS public key")
	}

	return nil
}

// Creates a new copy of the validator so we can mutate ProposerPriority.
// Panics if the validator is nil.
func (v *Validator) Copy() *Validator {
	if v == nil {
		return nil
	}
	vCopy := *v
	// Deep copy BLS public key if exists
	if v.BlsPubKey != nil {
		blsPubKeyCopy, _ := blscrypto.PublicKeyFromBytes(blscrypto.PublicKeyToBytes(v.BlsPubKey))
		vCopy.BlsPubKey = blsPubKeyCopy
	}
	return &vCopy
}

// Returns the one with higher ProposerPriority.
func (v *Validator) CompareProposerPriority(other *Validator) *Validator {
	if v == nil {
		return other
	}
	switch {
	case v.ProposerPriority > other.ProposerPriority:
		return v
	case v.ProposerPriority < other.ProposerPriority:
		return other
	default:
		result := bytes.Compare(v.Address, other.Address)
		switch {
		case result < 0:
			return v
		case result > 0:
			return other
		default:
			panic("Cannot compare identical validators")
		}
	}
}

// String returns a string representation of String.
func (v *Validator) String() string {
	if v == nil {
		return "nil-Validator"
	}

	// Include BLS public key in string representation if exists
	if v.BlsPubKey != nil {
		return fmt.Sprintf("Validator{%v %v VP:%v BLS:%v A:%v}",
			v.Address,
			v.PubKey,
			v.VotingPower,
			blscrypto.PublicKeyToHex(v.BlsPubKey),
			v.ProposerPriority)
	}

	return fmt.Sprintf("Validator{%v %v VP:%v A:%v}",
		v.Address,
		v.PubKey,
		v.VotingPower,
		v.ProposerPriority)
}

// ValidatorListString returns a prettified validator list for logging purposes.
func ValidatorListString(vals []*Validator) string {
	chunks := make([]string, len(vals))
	for i, val := range vals {
		chunks[i] = fmt.Sprintf("%s:%d", val.Address, val.VotingPower)
	}

	return strings.Join(chunks, ",")
}

// Bytes computes the unique encoding of a validator with a given voting power.
func (v *Validator) Bytes() []byte {
	pk, err := ce.PubKeyToProto(v.PubKey)
	if err != nil {
		panic(err)
	}

	pbv := cmtproto.SimpleValidator{
		PubKey:      &pk,
		VotingPower: v.VotingPower,
	}

	bz, err := pbv.Marshal()
	if err != nil {
		panic(err)
	}
	return bz
}

// ToProto converts Validator to protobuf
func (v *Validator) ToProto() (*cmtproto.Validator, error) {
	if v == nil {
		return nil, errors.New("nil validator")
	}

	pk, err := ce.PubKeyToProto(v.PubKey)
	if err != nil {
		return nil, err
	}

	vp := cmtproto.Validator{
		Address:          v.Address,
		PubKey:           pk,
		VotingPower:      v.VotingPower,
		ProposerPriority: v.ProposerPriority,
	}

	// Add BLS public key to protobuf if exists
	if v.BlsPubKey != nil {
		vp.BlsPubKey = blscrypto.PublicKeyToBytes(v.BlsPubKey)
	}

	return &vp, nil
}

// ValidatorFromProto sets a protobuf Validator to the given pointer.
func ValidatorFromProto(vp *cmtproto.Validator) (*Validator, error) {
	if vp == nil {
		return nil, errors.New("nil validator")
	}

	pk, err := ce.PubKeyFromProto(vp.PubKey)
	if err != nil {
		return nil, err
	}

	v := new(Validator)
	v.Address = vp.GetAddress()
	v.PubKey = pk
	v.VotingPower = vp.GetVotingPower()
	v.ProposerPriority = vp.GetProposerPriority()

	// Parse BLS public key if exists
	if len(vp.BlsPubKey) > 0 {
		blsPubKey, err := blscrypto.PublicKeyFromBytes(vp.BlsPubKey)
		if err != nil {
			return nil, fmt.Errorf("error parsing BLS public key: %w", err)
		}
		v.BlsPubKey = blsPubKey
	}

	return v, nil
}

func RandValidator(randPower bool, minPower int64) (*Validator, PrivValidator) {
	privVal := NewMockPV()
	votePower := minPower
	if randPower {
		votePower += int64(cmtrand.Uint32())
	}
	pubKey, err := privVal.GetPubKey()
	if err != nil {
		panic(fmt.Errorf("could not retrieve pubkey %w", err))
	}

	// Get BLS private key
	blsPrivKey, err := privVal.GetBlsPrivKey()
	if err != nil {
		panic(fmt.Errorf("could not retrieve BLS private key %w", err))
	}

	// Create validator with both ED25519 and BLS public keys
	val := NewValidator(
		pubKey,
		votePower,
		blsPrivKey.PublicKey(), // Include BLS public key
	)
	return val, privVal
}
