package bls

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/cometbft/cometbft/crypto"
	"github.com/cometbft/cometbft/crypto/tmhash"
	blst "github.com/supranational/blst/bindings/go"
)

type PublicKey = blst.P1Affine
type Signature = blst.P2Affine

// KeyGenMode determines the key generation method
type KeyGenMode int

const (
	KeyGenRandom KeyGenMode = iota
	KeyGenDeterministic
)

// Domain separation tag
var dst = []byte("BLS_SIG_BLS12381G2_XMD:SHA-256_SSWU_RO_NUL_")

// PrivateKey represents a BLS private key
type PrivateKey struct {
	sk *blst.SecretKey
}

// GeneratePrivateKey creates a new private key
func GeneratePrivateKey(mode KeyGenMode) *PrivateKey {
	var ikm [32]byte
	switch mode {
	case KeyGenRandom:
		rand.Read(ikm[:])
	case KeyGenDeterministic:
		// Use zero bytes for deterministic key generation
	default:
		panic("invalid key generation mode")
	}

	sk := blst.KeyGen(ikm[:])
	return &PrivateKey{sk: sk}
}

// GeneratePrivateKeyFromSeed creates a deterministic private key
func GeneratePrivateKeyFromSeed(seed []byte) *PrivateKey {
	var ikm [32]byte
	copy(ikm[:], seed)
	sk := blst.KeyGen(ikm[:])
	return &PrivateKey{sk: sk}
}

// PublicKey returns the public key for the private key
func (sk *PrivateKey) PublicKey() *PublicKey {
	return new(PublicKey).From(sk.sk)
}

// Sign creates a signature for a message
func (sk *PrivateKey) Sign(msg []byte) *Signature {
	return new(Signature).Sign(sk.sk, msg, dst)
}

// Bytes serializes the private key
func (sk *PrivateKey) Bytes() []byte {
	return sk.sk.Serialize()
}

// Hex returns the hex-encoded private key
func (sk *PrivateKey) Hex() string {
	return hex.EncodeToString(sk.Bytes())
}

// Helper functions for PublicKey

// PublicKeyFromBytes reconstructs a public key from bytes
func PublicKeyFromBytes(data []byte) (*PublicKey, error) {
	pk := new(PublicKey).Uncompress(data)
	if pk == nil {
		return nil, fmt.Errorf("invalid public key bytes")
	}
	return pk, nil
}

// PublicKeyToBytes converts a public key to bytes
func PublicKeyToBytes(pk *PublicKey) []byte {
	return pk.Compress()
}

// PublicKeyToHex converts a public key to a hex string
func PublicKeyToHex(pk *PublicKey) string {
	return hex.EncodeToString(PublicKeyToBytes(pk))
}

// PublicKeyAddress generates a Tendermint-compatible address from the public key
func PublicKeyAddress(pk *PublicKey) crypto.Address {
	return crypto.Address(tmhash.SumTruncated(PublicKeyToBytes(pk)))
}

// IsValidPublicKey checks if a public key is valid
func IsValidPublicKey(pk *PublicKey) bool {
	return pk != nil && pk.InG1() && pk.KeyValidate()
}

// PublicKeyEqual checks if two public keys are equal
func PublicKeyEqual(pk1, pk2 *PublicKey) bool {
	return pk1.Equals(pk2)
}

// Verification functions

// Verify checks a signature against a message
func Verify(pk *PublicKey, msg []byte, sig *Signature) bool {
	return sig.Verify(true, pk, true, msg, dst)
}

// AggregateVerify allows verifying multiple signatures against multiple messages
func AggregateVerify(pubKeys []*PublicKey, msgs [][]byte, sig *Signature) bool {
	if len(pubKeys) != len(msgs) {
		return false
	}

	return sig.AggregateVerify(true, pubKeys, true, msgs, dst)
}

// Aggregation functions

// AggregatePublicKeys combines multiple public keys
func AggregatePublicKeys(pubKeys []*PublicKey) *PublicKey {
	if len(pubKeys) == 0 {
		return nil
	}

	aggregatedPk := new(blst.P1Aggregate)
	for _, pk := range pubKeys {
		aggregatedPk.Add(pk, true)
	}

	return aggregatedPk.ToAffine()
}

// AggregateSignatures combines multiple signatures
func AggregateSignatures(signatures []*Signature) (*Signature, error) {
	if len(signatures) == 0 {
		return nil, errors.New("no signatures to aggregate")
	}

	// Check for nil signatures
	for _, sig := range signatures {
		if sig == nil {
			return nil, errors.New("nil signature in aggregation list")
		}
	}

	aggregatedSig := new(blst.P2Aggregate)
	for _, sig := range signatures {
		// Add with minimal serialization/deserialization overhead
		aggregatedSig.Add(sig, true)
	}

	// Convert to affine representation
	finalSig := aggregatedSig.ToAffine()

	// Additional validation
	if finalSig == nil {
		return nil, errors.New("failed to aggregate signatures")
	}

	return finalSig, nil
}

// VerifyAggregateSignature verifies an aggregate signature against multiple public keys
func VerifyAggregateSignature(message []byte, aggregatedSignature *Signature, pubKeys []*PublicKey) bool {
	if aggregatedSignature == nil || len(pubKeys) == 0 {
		return false
	}
	// Create messages array (same message for all validators)
	messages := make([][]byte, len(pubKeys))
	for i := range pubKeys {
		messages[i] = message
	}
	// Use AggregateVerify to verify against multiple public keys
	return aggregatedSignature.AggregateVerify(
		true,     // sigGroupcheck - validate the signature
		pubKeys,  // all validators' public keys
		true,     // pkValidate - validate public keys
		messages, // same message for each validator
		nil,      // dst (domain separation tag)
		false,    // useHash (optional)
		nil,      // augmentation (optional)
	)
}

// Helper functions for Signature

// SignatureFromBytes reconstructs a signature from bytes
func SignatureFromBytes(data []byte) (*Signature, error) {
	sig := new(Signature).Uncompress(data)
	if sig == nil {
		return nil, fmt.Errorf("invalid signature bytes")
	}
	return sig, nil
}

// SignatureToBytes converts a signature to bytes
func SignatureToBytes(sig *Signature) []byte {
	return sig.Compress()
}

// SignatureToHex converts a signature to a hex string
func SignatureToHex(sig *Signature) string {
	return hex.EncodeToString(SignatureToBytes(sig))
}

// IsValidSignature checks if a signature is valid
func IsValidSignature(sig *Signature) bool {
	return sig != nil && sig.SigValidate(true)
}

// SignatureEqual checks if two signatures are equal
func SignatureEqual(sig1, sig2 *Signature) bool {
	return sig1.Equals(sig2)
}

// CreateSignatureAggregator creates a new signature aggregator
func CreateSignatureAggregator(signatures []*Signature) (*Signature, error) {
	return AggregateSignatures(signatures)
}

// AggregateAndVerify aggregates signatures and verifies the aggregate
func AggregateAndVerify(
	message []byte,
	signatures []*Signature,
	publicKeys []*PublicKey,
) (*Signature, bool) {
	// Aggregate signatures
	aggregatedSig, err := AggregateSignatures(signatures)
	if err != nil {
		return nil, false
	}

	// Verify the aggregated signature
	verified := VerifyAggregateSignature(message, aggregatedSig, publicKeys)

	return aggregatedSig, verified
}
