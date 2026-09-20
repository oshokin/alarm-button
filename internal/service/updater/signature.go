package updater

import (
	"crypto/ed25519"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"go.yaml.in/yaml/v3"
)

// trustedUpdatePublicKeys are embedded roots of trust for update metadata.
var trustedUpdatePublicKeys = map[string]ed25519.PublicKey{
	"2026-02": mustDecodePublicKey("nkVBARz8knB69w0FkFVAqhkVVBnzLpxEUhMvN53IMa4="),
}

// errManifestKeyIDUnknown indicates manifest references key not present in trust store.
var errManifestKeyIDUnknown = errors.New("manifest key id is not trusted")

// mustDecodePublicKey decodes base64 key or panics during initialization.
func mustDecodePublicKey(text string) ed25519.PublicKey {
	decoded, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		panic(err)
	}

	return ed25519.PublicKey(decoded)
}

// verifyManifest verifies detached Ed25519 signature against trusted key set.
func (r *runner) verifyManifest(manifest, signatureText []byte) error {
	trustedKeys := r.trustedManifestKeys
	if len(trustedKeys) == 0 {
		return errNoTrustedManifestKey
	}

	signature, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signatureText)))
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	candidateID := r.candidateManifestKeyID(manifest)
	if candidateID != "" {
		key := trustedKeys[candidateID]
		if len(key) == 0 {
			return fmt.Errorf("%w: %s", errManifestKeyIDUnknown, candidateID)
		}

		if ed25519.Verify(key, manifest, signature) {
			return nil
		}

		return errInvalidManifestSignature
	}

	for _, key := range trustedKeys {
		if ed25519.Verify(key, manifest, signature) {
			return nil
		}
	}

	return errInvalidManifestSignature
}

// candidateManifestKeyID extracts explicit signing key id from manifest bytes.
func (r *runner) candidateManifestKeyID(manifest []byte) string {
	var parsed Manifest
	if err := yaml.Unmarshal(manifest, &parsed); err != nil {
		return ""
	}

	if parsed.Signing.KeyID == "" {
		return ""
	}

	return parsed.Signing.KeyID
}
